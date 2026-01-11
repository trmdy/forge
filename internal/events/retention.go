// Package events provides event logging and retention management.
package events

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/tOgg1/forge/internal/config"
	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/logging"
	"github.com/tOgg1/forge/internal/models"
)

// RetentionService manages event retention and cleanup.
type RetentionService struct {
	cfg     *config.EventRetentionConfig
	dataDir string
	repo    *db.EventRepository
	logger  zerolog.Logger
	stopCh  chan struct{}
	wg      sync.WaitGroup
	mu      sync.Mutex
	running bool

	// Stats
	lastCleanup   time.Time
	totalDeleted  int64
	totalArchived int64
}

// RetentionStats contains statistics about retention operations.
type RetentionStats struct {
	LastCleanup   time.Time  `json:"last_cleanup"`
	TotalDeleted  int64      `json:"total_deleted"`
	TotalArchived int64      `json:"total_archived"`
	EventCount    int64      `json:"event_count"`
	OldestEvent   *time.Time `json:"oldest_event,omitempty"`
}

// NewRetentionService creates a new retention service.
func NewRetentionService(cfg *config.Config, repo *db.EventRepository) *RetentionService {
	logger := logging.Component("retention")

	return &RetentionService{
		cfg:     &cfg.EventRetention,
		dataDir: cfg.Global.DataDir,
		repo:    repo,
		logger:  logger,
		stopCh:  make(chan struct{}),
	}
}

// Start begins the background cleanup job.
func (s *RetentionService) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("retention service already running")
	}
	s.running = true
	s.mu.Unlock()

	if !s.cfg.Enabled {
		s.logger.Info().Msg("event retention is disabled")
		return nil
	}

	s.logger.Info().
		Dur("cleanup_interval", s.cfg.CleanupInterval).
		Dur("max_age", s.cfg.MaxAge).
		Int("max_count", s.cfg.MaxCount).
		Bool("archive_before_delete", s.cfg.ArchiveBeforeDelete).
		Msg("starting event retention service")

	// Run initial cleanup
	if err := s.RunCleanup(ctx); err != nil {
		s.logger.Warn().Err(err).Msg("initial cleanup failed")
	}

	// Start background job
	s.wg.Add(1)
	go s.cleanupLoop(ctx)

	return nil
}

// Stop stops the background cleanup job.
func (s *RetentionService) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	s.mu.Unlock()

	close(s.stopCh)
	s.wg.Wait()
	s.logger.Info().Msg("retention service stopped")
}

// RunCleanup runs a single cleanup cycle.
func (s *RetentionService) RunCleanup(ctx context.Context) error {
	s.logger.Debug().Msg("running cleanup cycle")
	startTime := time.Now()

	var deletedByAge, deletedByCount int64
	var archivedByAge, archivedByCount int64
	var err error

	// Clean by age first
	if s.cfg.MaxAge > 0 {
		cutoff := time.Now().Add(-s.cfg.MaxAge)
		deletedByAge, archivedByAge, err = s.cleanupByAge(ctx, cutoff)
		if err != nil {
			return fmt.Errorf("cleanup by age failed: %w", err)
		}
	}

	// Then clean by count
	if s.cfg.MaxCount > 0 {
		deletedByCount, archivedByCount, err = s.cleanupByCount(ctx, s.cfg.MaxCount)
		if err != nil {
			return fmt.Errorf("cleanup by count failed: %w", err)
		}
	}

	totalDeleted := deletedByAge + deletedByCount
	totalArchived := archivedByAge + archivedByCount

	s.mu.Lock()
	s.lastCleanup = startTime
	s.totalDeleted += totalDeleted
	s.totalArchived += totalArchived
	s.mu.Unlock()

	if totalDeleted > 0 || totalArchived > 0 {
		s.logger.Info().
			Int64("deleted_by_age", deletedByAge).
			Int64("deleted_by_count", deletedByCount).
			Int64("archived_by_age", archivedByAge).
			Int64("archived_by_count", archivedByCount).
			Dur("duration", time.Since(startTime)).
			Msg("cleanup completed")
	} else {
		s.logger.Debug().Msg("cleanup completed, no events to remove")
	}

	return nil
}

// Stats returns current retention statistics.
func (s *RetentionService) Stats(ctx context.Context) (*RetentionStats, error) {
	s.mu.Lock()
	stats := &RetentionStats{
		LastCleanup:   s.lastCleanup,
		TotalDeleted:  s.totalDeleted,
		TotalArchived: s.totalArchived,
	}
	s.mu.Unlock()

	count, err := s.repo.Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get event count: %w", err)
	}
	stats.EventCount = count

	oldest, err := s.repo.OldestTimestamp(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get oldest event: %w", err)
	}
	stats.OldestEvent = oldest

	return stats, nil
}

func (s *RetentionService) cleanupLoop(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(s.cfg.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			if err := s.RunCleanup(ctx); err != nil {
				s.logger.Error().Err(err).Msg("cleanup cycle failed")
			}
		}
	}
}

func (s *RetentionService) cleanupByAge(ctx context.Context, cutoff time.Time) (deleted, archived int64, err error) {
	for {
		select {
		case <-ctx.Done():
			return deleted, archived, ctx.Err()
		default:
		}

		if s.cfg.ArchiveBeforeDelete {
			// Fetch events to archive
			events, err := s.repo.ListOlderThan(ctx, cutoff, s.cfg.BatchSize)
			if err != nil {
				return deleted, archived, err
			}
			if len(events) == 0 {
				break
			}

			// Archive events
			if err := s.archiveEvents(events); err != nil {
				return deleted, archived, fmt.Errorf("archive failed: %w", err)
			}
			archived += int64(len(events))

			// Delete archived events
			ids := make([]string, len(events))
			for i, e := range events {
				ids[i] = e.ID
			}
			count, err := s.repo.DeleteByIDs(ctx, ids)
			if err != nil {
				return deleted, archived, err
			}
			deleted += count
		} else {
			// Direct delete
			count, err := s.repo.DeleteOlderThan(ctx, cutoff, s.cfg.BatchSize)
			if err != nil {
				return deleted, archived, err
			}
			if count == 0 {
				break
			}
			deleted += count
		}

		// Check if we've deleted less than batch size, meaning we're done
		if deleted > 0 && deleted%int64(s.cfg.BatchSize) != 0 {
			break
		}
	}

	return deleted, archived, nil
}

func (s *RetentionService) cleanupByCount(ctx context.Context, maxCount int) (deleted, archived int64, err error) {
	// Get current count
	total, err := s.repo.Count(ctx)
	if err != nil {
		return 0, 0, err
	}

	excess := total - int64(maxCount)
	if excess <= 0 {
		return 0, 0, nil
	}

	for excess > 0 {
		select {
		case <-ctx.Done():
			return deleted, archived, ctx.Err()
		default:
		}

		batchSize := s.cfg.BatchSize
		if int64(batchSize) > excess {
			batchSize = int(excess)
		}

		if s.cfg.ArchiveBeforeDelete {
			// Fetch oldest events to archive
			events, err := s.repo.ListOldest(ctx, batchSize)
			if err != nil {
				return deleted, archived, err
			}
			if len(events) == 0 {
				break
			}

			// Archive events
			if err := s.archiveEvents(events); err != nil {
				return deleted, archived, fmt.Errorf("archive failed: %w", err)
			}
			archived += int64(len(events))

			// Delete archived events
			ids := make([]string, len(events))
			for i, e := range events {
				ids[i] = e.ID
			}
			count, err := s.repo.DeleteByIDs(ctx, ids)
			if err != nil {
				return deleted, archived, err
			}
			deleted += count
			excess -= count
		} else {
			count, err := s.repo.DeleteExcess(ctx, maxCount, batchSize)
			if err != nil {
				return deleted, archived, err
			}
			if count == 0 {
				break
			}
			deleted += count
			excess -= count
		}
	}

	return deleted, archived, nil
}

func (s *RetentionService) archiveEvents(events []*models.Event) error {
	if len(events) == 0 {
		return nil
	}

	archiveDir := s.cfg.ArchiveDir
	if archiveDir == "" {
		archiveDir = filepath.Join(s.dataDir, "archives")
	}

	// Ensure archive directory exists
	if err := os.MkdirAll(archiveDir, 0755); err != nil {
		return fmt.Errorf("failed to create archive directory: %w", err)
	}

	// Group events by date for archiving
	eventsByDate := make(map[string][]*models.Event)
	for _, event := range events {
		date := event.Timestamp.Format("2006-01-02")
		eventsByDate[date] = append(eventsByDate[date], event)
	}

	// Write each date's events to a separate file
	for date, dateEvents := range eventsByDate {
		filename := fmt.Sprintf("events_%s.jsonl", date)
		filepath := filepath.Join(archiveDir, filename)

		// Open file for appending
		file, err := os.OpenFile(filepath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("failed to open archive file: %w", err)
		}

		for _, event := range dateEvents {
			data, err := json.Marshal(event)
			if err != nil {
				file.Close()
				return fmt.Errorf("failed to marshal event: %w", err)
			}
			if _, err := file.Write(append(data, '\n')); err != nil {
				file.Close()
				return fmt.Errorf("failed to write event: %w", err)
			}
		}

		if err := file.Close(); err != nil {
			return fmt.Errorf("failed to close archive file: %w", err)
		}
	}

	return nil
}
