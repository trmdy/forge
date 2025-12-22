package account

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/opencode-ai/swarm/internal/config"
	"github.com/opencode-ai/swarm/internal/models"
)

func TestResolveCredential_EnvPrefix(t *testing.T) {
	t.Setenv("TEST_KEY", "value")

	got, err := ResolveCredential("env:TEST_KEY")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != "value" {
		t.Fatalf("expected value, got %q", got)
	}
}

func TestResolveCredential_DollarVar(t *testing.T) {
	t.Setenv("TEST_KEY", "value")

	got, err := ResolveCredential("$TEST_KEY")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != "value" {
		t.Fatalf("expected value, got %q", got)
	}
}

func TestResolveCredential_DollarVarBraced(t *testing.T) {
	t.Setenv("TEST_KEY", "value")

	got, err := ResolveCredential("${TEST_KEY}")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != "value" {
		t.Fatalf("expected value, got %q", got)
	}
}

func TestResolveCredential_File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token.txt")
	if err := os.WriteFile(path, []byte("secret\n"), 0600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	got, err := ResolveCredential("file:" + path)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != "secret" {
		t.Fatalf("expected secret, got %q", got)
	}
}

func TestResolveCredential_Literal(t *testing.T) {
	got, err := ResolveCredential("literal")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != "literal" {
		t.Fatalf("expected literal, got %q", got)
	}
}

func TestService_AddGetListDelete(t *testing.T) {
	cfg := config.DefaultConfig()
	service := NewService(cfg)
	ctx := context.Background()

	account := &models.Account{
		Provider:      models.ProviderOpenAI,
		ProfileName:   "primary",
		CredentialRef: "env:OPENAI_API_KEY",
		IsActive:      true,
	}

	if err := service.AddAccount(ctx, account); err != nil {
		t.Fatalf("AddAccount failed: %v", err)
	}

	if err := service.AddAccount(ctx, account); !errors.Is(err, ErrAccountAlreadyExists) {
		t.Fatalf("expected ErrAccountAlreadyExists, got %v", err)
	}

	got, err := service.GetAccount(ctx, account.ID)
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if got.ProfileName != "primary" {
		t.Fatalf("unexpected profile name: %s", got.ProfileName)
	}

	all, err := service.ListAccounts(ctx, "")
	if err != nil {
		t.Fatalf("ListAccounts failed: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 account, got %d", len(all))
	}

	if err := service.DeleteAccount(ctx, account.ID); err != nil {
		t.Fatalf("DeleteAccount failed: %v", err)
	}

	if _, err := service.GetAccount(ctx, account.ID); !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("expected ErrAccountNotFound, got %v", err)
	}
}

func TestService_GetNextAvailable(t *testing.T) {
	cfg := config.DefaultConfig()
	service := NewService(cfg)
	ctx := context.Background()

	future := time.Now().UTC().Add(5 * time.Minute)

	account1 := &models.Account{
		Provider:      models.ProviderOpenAI,
		ProfileName:   "a",
		CredentialRef: "env:OPENAI_API_KEY",
		IsActive:      true,
		CooldownUntil: &future,
	}
	account2 := &models.Account{
		Provider:      models.ProviderOpenAI,
		ProfileName:   "b",
		CredentialRef: "env:OPENAI_API_KEY",
		IsActive:      true,
	}
	account3 := &models.Account{
		Provider:      models.ProviderAnthropic,
		ProfileName:   "x",
		CredentialRef: "env:ANTHROPIC_API_KEY",
		IsActive:      true,
	}

	for _, acct := range []*models.Account{account1, account2, account3} {
		if err := service.AddAccount(ctx, acct); err != nil {
			t.Fatalf("AddAccount failed: %v", err)
		}
	}

	got, err := service.GetNextAvailable(ctx, models.ProviderOpenAI)
	if err != nil {
		t.Fatalf("GetNextAvailable failed: %v", err)
	}
	if got.ProfileName != "b" {
		t.Fatalf("expected profile b, got %s", got.ProfileName)
	}

	if _, err := service.GetNextAvailable(ctx, models.Provider("")); !errors.Is(err, models.ErrInvalidProvider) {
		t.Fatalf("expected ErrInvalidProvider, got %v", err)
	}
}
