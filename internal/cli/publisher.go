package cli

import (
	"strings"

	"github.com/opencode-ai/swarm/internal/db"
	"github.com/opencode-ai/swarm/internal/events"
	"github.com/opencode-ai/swarm/internal/hooks"
)

func newEventPublisher(database *db.DB) events.Publisher {
	if database == nil {
		return nil
	}

	repo := db.NewEventRepository(database)
	publisher := events.NewInMemoryPublisher(events.WithRepository(repo))

	store := hooks.NewStore(hookStorePath())
	manager := hooks.NewManager(store, nil)
	if err := manager.Attach(publisher); err != nil {
		logger.Warn().Err(err).Str("store", strings.TrimSpace(store.Path())).Msg("failed to load hooks")
	}

	return publisher
}
