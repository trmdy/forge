package loop

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tOgg1/forge/internal/config"
	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/models"
	"github.com/tOgg1/forge/internal/testutil"
)

func TestRunnerRunOnceConsumesQueue(t *testing.T) {
	database, cleanup := testutil.NewTestDB(t)
	defer cleanup()

	repoDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Global.DataDir = t.TempDir()
	cfg.Global.ConfigDir = t.TempDir()

	profileRepo := db.NewProfileRepository(database)
	loopRepo := db.NewLoopRepository(database)
	queueRepo := db.NewLoopQueueRepository(database)
	runRepo := db.NewLoopRunRepository(database)

	profile := &models.Profile{
		Name:            "pi-default",
		Harness:         models.HarnessPi,
		PromptMode:      models.PromptModeEnv,
		CommandTemplate: "pi -p \"$FORGE_PROMPT_CONTENT\"",
		MaxConcurrency:  1,
	}
	if err := profileRepo.Create(context.Background(), profile); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	loopEntry := &models.Loop{
		Name:            "loop-a",
		RepoPath:        repoDir,
		BasePromptMsg:   "base",
		IntervalSeconds: 1,
		ProfileID:       profile.ID,
		State:           models.LoopStateStopped,
	}
	if err := loopRepo.Create(context.Background(), loopEntry); err != nil {
		t.Fatalf("create loop: %v", err)
	}

	overridePayload := mustJSON(models.NextPromptOverridePayload{Prompt: "override", IsPath: false})
	messagePayload := mustJSON(models.MessageAppendPayload{Text: "hello"})
	if err := queueRepo.Enqueue(context.Background(), loopEntry.ID,
		&models.LoopQueueItem{Type: models.LoopQueueItemNextPromptOverride, Payload: overridePayload},
		&models.LoopQueueItem{Type: models.LoopQueueItemMessageAppend, Payload: messagePayload},
	); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	var capturedPrompt string
	runner := NewRunner(database, cfg)
	runner.Exec = func(ctx context.Context, profile models.Profile, promptPath, promptContent, workDir string, output io.Writer) (int, string, error) {
		capturedPrompt = promptContent
		return 0, "ok", nil
	}

	if err := runner.RunOnce(context.Background(), loopEntry.ID); err != nil {
		t.Fatalf("run once: %v", err)
	}

	if !strings.Contains(capturedPrompt, "override") {
		t.Fatalf("expected override prompt in content")
	}
	if !strings.Contains(capturedPrompt, "hello") {
		t.Fatalf("expected operator message in content")
	}

	items, err := queueRepo.List(context.Background(), loopEntry.ID)
	if err != nil {
		t.Fatalf("list queue: %v", err)
	}
	for _, item := range items {
		if item.Status != models.LoopQueueStatusCompleted {
			t.Fatalf("expected queue item completed, got %s", item.Status)
		}
	}

	runs, err := runRepo.ListByLoop(context.Background(), loopEntry.ID)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0].PromptSource != "override" || !runs[0].PromptOverride {
		t.Fatalf("expected override prompt source")
	}
}

func TestRunnerClaudeHarnessRunsLoop(t *testing.T) {
	database, cleanup := testutil.NewTestDB(t)
	defer cleanup()

	repoDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Global.DataDir = t.TempDir()
	cfg.Global.ConfigDir = t.TempDir()

	profileRepo := db.NewProfileRepository(database)
	loopRepo := db.NewLoopRepository(database)
	runRepo := db.NewLoopRunRepository(database)

	profile := &models.Profile{
		Name:            "claude-test",
		Harness:         models.HarnessClaude,
		PromptMode:      models.PromptModeEnv,
		CommandTemplate: "script -q -c 'claude -p \"$FORGE_PROMPT_CONTENT\" --dangerously-skip-permissions' /dev/null",
		MaxConcurrency:  1,
	}
	if err := profileRepo.Create(context.Background(), profile); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	loopEntry := &models.Loop{
		Name:            "claude-loop",
		RepoPath:        repoDir,
		BasePromptMsg:   "test prompt for claude",
		IntervalSeconds: 1,
		ProfileID:       profile.ID,
		State:           models.LoopStateStopped,
	}
	if err := loopRepo.Create(context.Background(), loopEntry); err != nil {
		t.Fatalf("create loop: %v", err)
	}

	var capturedProfile models.Profile
	var capturedPrompt string
	runner := NewRunner(database, cfg)
	runner.Exec = func(ctx context.Context, p models.Profile, promptPath, promptContent, workDir string, output io.Writer) (int, string, error) {
		capturedProfile = p
		capturedPrompt = promptContent
		return 0, "claude output", nil
	}

	if err := runner.RunOnce(context.Background(), loopEntry.ID); err != nil {
		t.Fatalf("run once: %v", err)
	}

	if capturedProfile.Harness != models.HarnessClaude {
		t.Fatalf("expected claude harness, got %s", capturedProfile.Harness)
	}
	if capturedProfile.PromptMode != models.PromptModeEnv {
		t.Fatalf("expected env prompt mode, got %s", capturedProfile.PromptMode)
	}
	if !strings.Contains(capturedPrompt, "test prompt for claude") {
		t.Fatalf("expected prompt content, got %s", capturedPrompt)
	}

	runs, err := runRepo.ListByLoop(context.Background(), loopEntry.ID)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0].ProfileID != profile.ID {
		t.Fatalf("expected profile id %s, got %s", profile.ID, runs[0].ProfileID)
	}
}

func TestRunnerInjectsLoopEnv(t *testing.T) {
	database, cleanup := testutil.NewTestDB(t)
	defer cleanup()

	repoDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Global.DataDir = t.TempDir()
	cfg.Global.ConfigDir = t.TempDir()

	profileRepo := db.NewProfileRepository(database)
	loopRepo := db.NewLoopRepository(database)

	profile := &models.Profile{
		Name:            "env-profile",
		Harness:         models.HarnessPi,
		PromptMode:      models.PromptModeEnv,
		CommandTemplate: "pi -p \"$FORGE_PROMPT_CONTENT\"",
		MaxConcurrency:  1,
		Env: map[string]string{
			"FMAIL_AGENT":     "manual-agent",
			"FORGE_LOOP_NAME": "stale-loop-name",
			"FORGE_LOOP_ID":   "stale-loop-id",
			"CUSTOM_ENV":      "ok",
		},
	}
	if err := profileRepo.Create(context.Background(), profile); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	loopEntry := &models.Loop{
		Name:            "loop-env",
		RepoPath:        repoDir,
		BasePromptMsg:   "base",
		IntervalSeconds: 1,
		ProfileID:       profile.ID,
		State:           models.LoopStateStopped,
	}
	if err := loopRepo.Create(context.Background(), loopEntry); err != nil {
		t.Fatalf("create loop: %v", err)
	}

	var capturedProfile models.Profile
	runner := NewRunner(database, cfg)
	runner.Exec = func(ctx context.Context, p models.Profile, promptPath, promptContent, workDir string, output io.Writer) (int, string, error) {
		capturedProfile = p
		return 0, "ok", nil
	}

	if err := runner.RunOnce(context.Background(), loopEntry.ID); err != nil {
		t.Fatalf("run once: %v", err)
	}

	if capturedProfile.Env["FORGE_LOOP_ID"] != loopEntry.ID {
		t.Fatalf("expected FORGE_LOOP_ID=%q, got %q", loopEntry.ID, capturedProfile.Env["FORGE_LOOP_ID"])
	}
	if capturedProfile.Env["FORGE_LOOP_NAME"] != loopEntry.Name {
		t.Fatalf("expected FORGE_LOOP_NAME=%q, got %q", loopEntry.Name, capturedProfile.Env["FORGE_LOOP_NAME"])
	}
	if capturedProfile.Env["FMAIL_AGENT"] != "manual-agent" {
		t.Fatalf("expected explicit FMAIL_AGENT to be preserved, got %q", capturedProfile.Env["FMAIL_AGENT"])
	}
	if capturedProfile.Env["CUSTOM_ENV"] != "ok" {
		t.Fatalf("expected CUSTOM_ENV to be preserved, got %q", capturedProfile.Env["CUSTOM_ENV"])
	}
}

func TestRunnerDefaultsFmailAgentToLoopName(t *testing.T) {
	database, cleanup := testutil.NewTestDB(t)
	defer cleanup()

	repoDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Global.DataDir = t.TempDir()
	cfg.Global.ConfigDir = t.TempDir()

	profileRepo := db.NewProfileRepository(database)
	loopRepo := db.NewLoopRepository(database)

	profile := &models.Profile{
		Name:            "default-fmail-profile",
		Harness:         models.HarnessPi,
		PromptMode:      models.PromptModeEnv,
		CommandTemplate: "pi -p \"$FORGE_PROMPT_CONTENT\"",
		MaxConcurrency:  1,
	}
	if err := profileRepo.Create(context.Background(), profile); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	loopEntry := &models.Loop{
		Name:            "loop-default-fmail",
		RepoPath:        repoDir,
		BasePromptMsg:   "base",
		IntervalSeconds: 1,
		ProfileID:       profile.ID,
		State:           models.LoopStateStopped,
	}
	if err := loopRepo.Create(context.Background(), loopEntry); err != nil {
		t.Fatalf("create loop: %v", err)
	}

	var capturedProfile models.Profile
	runner := NewRunner(database, cfg)
	runner.Exec = func(ctx context.Context, p models.Profile, promptPath, promptContent, workDir string, output io.Writer) (int, string, error) {
		capturedProfile = p
		return 0, "ok", nil
	}

	if err := runner.RunOnce(context.Background(), loopEntry.ID); err != nil {
		t.Fatalf("run once: %v", err)
	}

	if capturedProfile.Env["FMAIL_AGENT"] != loopEntry.Name {
		t.Fatalf("expected FMAIL_AGENT=%q, got %q", loopEntry.Name, capturedProfile.Env["FMAIL_AGENT"])
	}
}

func TestRunnerStopQueueStopsLoop(t *testing.T) {
	database, cleanup := testutil.NewTestDB(t)
	defer cleanup()

	repoDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Global.DataDir = t.TempDir()
	cfg.Global.ConfigDir = t.TempDir()

	loopRepo := db.NewLoopRepository(database)
	queueRepo := db.NewLoopQueueRepository(database)
	runRepo := db.NewLoopRunRepository(database)

	loopEntry := &models.Loop{
		Name:            "loop-stop",
		RepoPath:        repoDir,
		IntervalSeconds: 1,
		State:           models.LoopStateStopped,
	}
	if err := loopRepo.Create(context.Background(), loopEntry); err != nil {
		t.Fatalf("create loop: %v", err)
	}

	stopPayload := mustJSON(models.StopPayload{Reason: "test"})
	if err := queueRepo.Enqueue(context.Background(), loopEntry.ID,
		&models.LoopQueueItem{Type: models.LoopQueueItemStopGraceful, Payload: stopPayload},
	); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	runner := NewRunner(database, cfg)
	runner.Exec = func(ctx context.Context, profile models.Profile, promptPath, promptContent, workDir string, output io.Writer) (int, string, error) {
		return 0, "", nil
	}

	if err := runner.RunOnce(context.Background(), loopEntry.ID); err != nil {
		t.Fatalf("run once: %v", err)
	}

	items, err := queueRepo.List(context.Background(), loopEntry.ID)
	if err != nil {
		t.Fatalf("list queue: %v", err)
	}
	for _, item := range items {
		if item.Status != models.LoopQueueStatusCompleted {
			t.Fatalf("expected queue item completed, got %s", item.Status)
		}
	}

	runs, err := runRepo.ListByLoop(context.Background(), loopEntry.ID)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("expected no runs, got %d", len(runs))
	}

	updated, err := loopRepo.Get(context.Background(), loopEntry.ID)
	if err != nil {
		t.Fatalf("get loop: %v", err)
	}
	if updated.State != models.LoopStateStopped {
		t.Fatalf("expected loop stopped, got %s", updated.State)
	}
}

func TestRunnerInjectsPersistentMemoryIntoPrompt(t *testing.T) {
	database, cleanup := testutil.NewTestDB(t)
	defer cleanup()

	repoDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Global.DataDir = t.TempDir()
	cfg.Global.ConfigDir = t.TempDir()

	profileRepo := db.NewProfileRepository(database)
	loopRepo := db.NewLoopRepository(database)

	profile := &models.Profile{
		Name:            "mem-profile",
		Harness:         models.HarnessPi,
		PromptMode:      models.PromptModeEnv,
		CommandTemplate: "pi -p \"$FORGE_PROMPT_CONTENT\"",
		MaxConcurrency:  1,
	}
	if err := profileRepo.Create(context.Background(), profile); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	loopEntry := &models.Loop{
		Name:            "loop-mem",
		RepoPath:        repoDir,
		BasePromptMsg:   "base",
		IntervalSeconds: 1,
		ProfileID:       profile.ID,
		State:           models.LoopStateStopped,
	}
	if err := loopRepo.Create(context.Background(), loopEntry); err != nil {
		t.Fatalf("create loop: %v", err)
	}

	workRepo := db.NewLoopWorkStateRepository(database)
	if err := workRepo.SetCurrent(context.Background(), &models.LoopWorkState{
		LoopID:        loopEntry.ID,
		AgentID:       "loop-mem",
		TaskID:        "sv-abc123",
		Status:        "blocked",
		Detail:        "asked about schema mismatch",
		LoopIteration: 7,
	}); err != nil {
		t.Fatalf("set work state: %v", err)
	}

	kvRepo := db.NewLoopKVRepository(database)
	if err := kvRepo.Set(context.Background(), loopEntry.ID, "blocked_on", "agent-b reply"); err != nil {
		t.Fatalf("set kv: %v", err)
	}

	var capturedPrompt string
	runner := NewRunner(database, cfg)
	runner.Exec = func(ctx context.Context, p models.Profile, promptPath, promptContent, workDir string, output io.Writer) (int, string, error) {
		capturedPrompt = promptContent
		return 0, "ok", nil
	}

	if err := runner.RunOnce(context.Background(), loopEntry.ID); err != nil {
		t.Fatalf("run once: %v", err)
	}

	if !strings.Contains(capturedPrompt, "Loop Context (persistent)") {
		t.Fatalf("expected context section injected into prompt")
	}
	if !strings.Contains(capturedPrompt, "sv-abc123") || !strings.Contains(capturedPrompt, "blocked") {
		t.Fatalf("expected current task context injected into prompt")
	}
	if !strings.Contains(capturedPrompt, "blocked_on") || !strings.Contains(capturedPrompt, "agent-b reply") {
		t.Fatalf("expected kv injected into prompt")
	}
}

func TestRunnerMaxIterationsStopsLoop(t *testing.T) {
	database, cleanup := testutil.NewTestDB(t)
	defer cleanup()

	repoDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Global.DataDir = t.TempDir()
	cfg.Global.ConfigDir = t.TempDir()

	profileRepo := db.NewProfileRepository(database)
	loopRepo := db.NewLoopRepository(database)
	runRepo := db.NewLoopRunRepository(database)

	profile := &models.Profile{
		Name:            "iter-profile",
		Harness:         models.HarnessPi,
		PromptMode:      models.PromptModeEnv,
		CommandTemplate: "pi -p \"$FORGE_PROMPT_CONTENT\"",
		MaxConcurrency:  1,
	}
	if err := profileRepo.Create(context.Background(), profile); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	loopEntry := &models.Loop{
		Name:            "loop-max-iter",
		RepoPath:        repoDir,
		BasePromptMsg:   "base",
		IntervalSeconds: 0,
		MaxIterations:   2,
		ProfileID:       profile.ID,
		State:           models.LoopStateStopped,
	}
	if err := loopRepo.Create(context.Background(), loopEntry); err != nil {
		t.Fatalf("create loop: %v", err)
	}

	runner := NewRunner(database, cfg)
	runner.Exec = func(ctx context.Context, profile models.Profile, promptPath, promptContent, workDir string, output io.Writer) (int, string, error) {
		return 0, "ok", nil
	}

	if err := runner.RunLoop(context.Background(), loopEntry.ID); err != nil {
		t.Fatalf("run loop: %v", err)
	}

	runs, err := runRepo.ListByLoop(context.Background(), loopEntry.ID)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(runs))
	}

	updated, err := loopRepo.Get(context.Background(), loopEntry.ID)
	if err != nil {
		t.Fatalf("get loop: %v", err)
	}
	if updated.State != models.LoopStateStopped {
		t.Fatalf("expected loop stopped, got %s", updated.State)
	}
}

func TestRunnerMaxRuntimeStopsLoop(t *testing.T) {
	database, cleanup := testutil.NewTestDB(t)
	defer cleanup()

	repoDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Global.DataDir = t.TempDir()
	cfg.Global.ConfigDir = t.TempDir()

	profileRepo := db.NewProfileRepository(database)
	loopRepo := db.NewLoopRepository(database)
	runRepo := db.NewLoopRunRepository(database)

	profile := &models.Profile{
		Name:            "runtime-profile",
		Harness:         models.HarnessPi,
		PromptMode:      models.PromptModeEnv,
		CommandTemplate: "pi -p \"$FORGE_PROMPT_CONTENT\"",
		MaxConcurrency:  1,
	}
	if err := profileRepo.Create(context.Background(), profile); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	loopEntry := &models.Loop{
		Name:              "loop-max-runtime",
		RepoPath:          repoDir,
		BasePromptMsg:     "base",
		IntervalSeconds:   0,
		MaxRuntimeSeconds: 1,
		ProfileID:         profile.ID,
		State:             models.LoopStateStopped,
	}
	if err := loopRepo.Create(context.Background(), loopEntry); err != nil {
		t.Fatalf("create loop: %v", err)
	}

	runner := NewRunner(database, cfg)
	runner.Exec = func(ctx context.Context, profile models.Profile, promptPath, promptContent, workDir string, output io.Writer) (int, string, error) {
		time.Sleep(1100 * time.Millisecond)
		return 0, "ok", nil
	}

	if err := runner.RunLoop(context.Background(), loopEntry.ID); err != nil {
		t.Fatalf("run loop: %v", err)
	}

	runs, err := runRepo.ListByLoop(context.Background(), loopEntry.ID)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}

	updated, err := loopRepo.Get(context.Background(), loopEntry.ID)
	if err != nil {
		t.Fatalf("get loop: %v", err)
	}
	if updated.State != models.LoopStateStopped {
		t.Fatalf("expected loop stopped, got %s", updated.State)
	}
}

func TestRunnerMaxIterationsStopsImmediatelyWithoutSleepCycle(t *testing.T) {
	database, cleanup := testutil.NewTestDB(t)
	defer cleanup()

	repoDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Global.DataDir = t.TempDir()
	cfg.Global.ConfigDir = t.TempDir()

	profileRepo := db.NewProfileRepository(database)
	loopRepo := db.NewLoopRepository(database)
	runRepo := db.NewLoopRunRepository(database)

	profile := &models.Profile{
		Name:            "iter-immediate-stop-profile",
		Harness:         models.HarnessPi,
		PromptMode:      models.PromptModeEnv,
		CommandTemplate: "pi -p \"$FORGE_PROMPT_CONTENT\"",
		MaxConcurrency:  1,
	}
	if err := profileRepo.Create(context.Background(), profile); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	loopEntry := &models.Loop{
		Name:            "loop-max-iter-immediate-stop",
		RepoPath:        repoDir,
		BasePromptMsg:   "base",
		IntervalSeconds: 3600,
		MaxIterations:   1,
		ProfileID:       profile.ID,
		State:           models.LoopStateStopped,
	}
	if err := loopRepo.Create(context.Background(), loopEntry); err != nil {
		t.Fatalf("create loop: %v", err)
	}

	runner := NewRunner(database, cfg)
	runner.Exec = func(ctx context.Context, profile models.Profile, promptPath, promptContent, workDir string, output io.Writer) (int, string, error) {
		return 0, "ok", nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if err := runner.RunLoop(ctx, loopEntry.ID); err != nil {
		t.Fatalf("run loop: %v", err)
	}

	runs, err := runRepo.ListByLoop(context.Background(), loopEntry.ID)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}

	updated, err := loopRepo.Get(context.Background(), loopEntry.ID)
	if err != nil {
		t.Fatalf("get loop: %v", err)
	}
	if updated.State != models.LoopStateStopped {
		t.Fatalf("expected loop stopped, got %s", updated.State)
	}
	if updated.LastExitCode == nil || *updated.LastExitCode != 0 {
		t.Fatalf("expected last exit code 0, got %v", updated.LastExitCode)
	}
	if got := loopIterationCount(updated.Metadata); got != 1 {
		t.Fatalf("expected iteration_count=1, got %d", got)
	}
}

func TestRunnerMaxRuntimeStopsImmediatelyWithoutSleepCycle(t *testing.T) {
	database, cleanup := testutil.NewTestDB(t)
	defer cleanup()

	repoDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Global.DataDir = t.TempDir()
	cfg.Global.ConfigDir = t.TempDir()

	profileRepo := db.NewProfileRepository(database)
	loopRepo := db.NewLoopRepository(database)
	runRepo := db.NewLoopRunRepository(database)

	profile := &models.Profile{
		Name:            "runtime-immediate-stop-profile",
		Harness:         models.HarnessPi,
		PromptMode:      models.PromptModeEnv,
		CommandTemplate: "pi -p \"$FORGE_PROMPT_CONTENT\"",
		MaxConcurrency:  1,
	}
	if err := profileRepo.Create(context.Background(), profile); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	loopEntry := &models.Loop{
		Name:              "loop-max-runtime-immediate-stop",
		RepoPath:          repoDir,
		BasePromptMsg:     "base",
		IntervalSeconds:   3600,
		MaxRuntimeSeconds: 1,
		ProfileID:         profile.ID,
		State:             models.LoopStateStopped,
	}
	if err := loopRepo.Create(context.Background(), loopEntry); err != nil {
		t.Fatalf("create loop: %v", err)
	}

	runner := NewRunner(database, cfg)
	runner.Exec = func(ctx context.Context, profile models.Profile, promptPath, promptContent, workDir string, output io.Writer) (int, string, error) {
		time.Sleep(1100 * time.Millisecond)
		return 0, "ok", nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := runner.RunLoop(ctx, loopEntry.ID); err != nil {
		t.Fatalf("run loop: %v", err)
	}

	runs, err := runRepo.ListByLoop(context.Background(), loopEntry.ID)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}

	updated, err := loopRepo.Get(context.Background(), loopEntry.ID)
	if err != nil {
		t.Fatalf("get loop: %v", err)
	}
	if updated.State != models.LoopStateStopped {
		t.Fatalf("expected loop stopped, got %s", updated.State)
	}
	if updated.LastExitCode == nil || *updated.LastExitCode != 0 {
		t.Fatalf("expected last exit code 0, got %v", updated.LastExitCode)
	}
	if got := loopIterationCount(updated.Metadata); got != 1 {
		t.Fatalf("expected iteration_count=1, got %d", got)
	}
}

func TestEnsureLoopPathsCreatesLogDirWhenLogPathSet(t *testing.T) {
	database, cleanup := testutil.NewTestDB(t)
	defer cleanup()

	repoDir := t.TempDir()
	dataDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Global.DataDir = dataDir
	cfg.Global.ConfigDir = t.TempDir()

	logPath := filepath.Join(dataDir, "logs", "loops", "custom.log")
	if _, err := os.Stat(filepath.Dir(logPath)); !os.IsNotExist(err) {
		t.Fatalf("expected log directory to not exist")
	}

	loopRepo := db.NewLoopRepository(database)
	loopEntry := &models.Loop{
		Name:            "loop-logdir",
		RepoPath:        repoDir,
		IntervalSeconds: 1,
		State:           models.LoopStateStopped,
		LogPath:         logPath,
	}
	if err := loopRepo.Create(context.Background(), loopEntry); err != nil {
		t.Fatalf("create loop: %v", err)
	}

	stored, err := loopRepo.Get(context.Background(), loopEntry.ID)
	if err != nil {
		t.Fatalf("get loop: %v", err)
	}

	runner := NewRunner(database, cfg)
	if err := runner.ensureLoopPaths(context.Background(), stored, loopRepo); err != nil {
		t.Fatalf("ensure loop paths: %v", err)
	}

	if _, err := os.Stat(filepath.Dir(logPath)); err != nil {
		t.Fatalf("expected log directory to exist: %v", err)
	}
}

func mustJSON(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}
