package hooks

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/tOgg1/forge/internal/events"
	"github.com/tOgg1/forge/internal/logging"
	"github.com/tOgg1/forge/internal/models"
)

// Manager wires stored hooks into an event publisher.
type Manager struct {
	store    *Store
	executor *Executor
	logger   zerolog.Logger
}

// NewManager creates a new hook manager.
func NewManager(store *Store, executor *Executor) *Manager {
	if executor == nil {
		executor = NewExecutor()
	}

	return &Manager{
		store:    store,
		executor: executor,
		logger:   logging.Component("hooks"),
	}
}

// Attach registers all stored hooks with the publisher.
func (m *Manager) Attach(publisher events.Publisher) error {
	if publisher == nil || m.store == nil {
		return nil
	}

	hooks, err := m.store.List()
	if err != nil {
		return err
	}

	for _, hook := range hooks {
		if !hook.Enabled {
			continue
		}

		filter := events.Filter{
			EventTypes:  hook.EventTypes,
			EntityTypes: hook.EntityTypes,
			EntityID:    hook.EntityID,
		}

		id := fmt.Sprintf("hook:%s", hook.ID)
		hook := hook
		if err := publisher.Subscribe(id, filter, func(event *models.Event) {
			m.runHook(hook, event)
		}); err != nil {
			m.logger.Warn().Err(err).Str("hook_id", hook.ID).Msg("failed to subscribe hook")
		}
	}

	return nil
}

func (m *Manager) runHook(hook Hook, event *models.Event) {
	ctx := context.Background()
	if timeout, ok := parseTimeout(hook.Timeout); ok {
		ctxTimeout, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		ctx = ctxTimeout
	}

	if err := m.executor.Execute(ctx, hook, event); err != nil {
		m.logger.Warn().Err(err).Str("hook_id", hook.ID).Msg("hook execution failed")
	}
}

func parseTimeout(value string) (time.Duration, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return DefaultTimeout, true
	}
	if trimmed == "0" {
		return 0, false
	}
	parsed, err := time.ParseDuration(trimmed)
	if err != nil {
		return DefaultTimeout, true
	}
	if parsed <= 0 {
		return 0, false
	}
	return parsed, true
}
