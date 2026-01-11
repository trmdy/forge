package forged

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/fmail"
)

type mailProject struct {
	ID   string
	Root string
}

type mailProjectResolver interface {
	Resolve(projectID string) (mailProject, error)
}

var (
	errMissingProjectID = errors.New("missing project id")
	errProjectNotFound  = errors.New("project not found")
	errProjectMismatch  = errors.New("project id mismatch")
)

type staticProjectResolver struct {
	project mailProject
}

func newStaticProjectResolver(root string) (*staticProjectResolver, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("project root required")
	}
	if info, err := os.Stat(root); err != nil {
		return nil, err
	} else if !info.IsDir() {
		return nil, errors.New("project root is not a directory")
	}

	id, err := projectIDFromRoot(root)
	if err != nil {
		return nil, err
	}

	return &staticProjectResolver{
		project: mailProject{ID: id, Root: root},
	}, nil
}

func (r *staticProjectResolver) Resolve(projectID string) (mailProject, error) {
	trimmed := strings.TrimSpace(projectID)
	if trimmed != "" && trimmed != r.project.ID {
		return mailProject{}, errProjectMismatch
	}
	return r.project, nil
}

type workspaceProjectResolver struct {
	repo  *db.WorkspaceRepository
	mu    sync.Mutex
	cache map[string]string
}

func newWorkspaceProjectResolver(repo *db.WorkspaceRepository) *workspaceProjectResolver {
	return &workspaceProjectResolver{
		repo:  repo,
		cache: make(map[string]string),
	}
}

func (r *workspaceProjectResolver) Resolve(projectID string) (mailProject, error) {
	trimmed := strings.TrimSpace(projectID)
	if trimmed == "" {
		return mailProject{}, errMissingProjectID
	}
	if root, ok := r.cachedRoot(trimmed); ok {
		return mailProject{ID: trimmed, Root: root}, nil
	}
	if r.repo == nil {
		return mailProject{}, errProjectNotFound
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	workspaces, err := r.repo.List(ctx)
	if err != nil {
		return mailProject{}, err
	}

	for _, workspace := range workspaces {
		root := strings.TrimSpace(workspace.RepoPath)
		if root == "" {
			continue
		}
		if info, err := os.Stat(root); err != nil || !info.IsDir() {
			continue
		}
		id, err := projectIDFromRoot(root)
		if err != nil {
			continue
		}
		if id == trimmed {
			r.cacheRoot(trimmed, root)
			return mailProject{ID: id, Root: root}, nil
		}
	}
	return mailProject{}, errProjectNotFound
}

func (r *workspaceProjectResolver) cachedRoot(projectID string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	root, ok := r.cache[projectID]
	return root, ok
}

func (r *workspaceProjectResolver) cacheRoot(projectID, root string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cache[projectID] = root
}

func projectIDFromRoot(root string) (string, error) {
	if id, ok := readProjectID(root); ok {
		return id, nil
	}
	return fmail.DeriveProjectID(root)
}

func readProjectID(root string) (string, bool) {
	path := filepath.Join(root, ".fmail", "project.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	var payload struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", false
	}
	id := strings.TrimSpace(payload.ID)
	if id == "" {
		return "", false
	}
	return id, true
}
