package cli

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/models"
)

func TestLoopCommandRegressionForTUIAcceptanceCriterion12(t *testing.T) {
	repo := t.TempDir()
	cleanupConfig := withTempConfig(t, repo)
	defer cleanupConfig()

	withWorkingDir(t, repo, func() {
		prevYes := yesFlag
		prevNonInteractive := nonInteractive
		prevJSON := jsonOutput
		prevJSONL := jsonlOutput
		prevQuiet := quiet
		defer func() {
			yesFlag = prevYes
			nonInteractive = prevNonInteractive
			jsonOutput = prevJSON
			jsonlOutput = prevJSONL
			quiet = prevQuiet
		}()

		yesFlag = true
		nonInteractive = false
		jsonOutput = false
		jsonlOutput = false
		quiet = true

		database, err := openDatabase()
		if err != nil {
			t.Fatalf("open database: %v", err)
		}
		defer database.Close()

		loopRepo := db.NewLoopRepository(database)
		queueRepo := db.NewLoopQueueRepository(database)

		running := &models.Loop{
			Name:     "cmd-running",
			RepoPath: repo,
			State:    models.LoopStateRunning,
		}
		if err := loopRepo.Create(context.Background(), running); err != nil {
			t.Fatalf("create running loop: %v", err)
		}

		resumeBlocked := &models.Loop{
			Name:     "cmd-resume-guard",
			RepoPath: repo,
			State:    models.LoopStateRunning,
		}
		if err := loopRepo.Create(context.Background(), resumeBlocked); err != nil {
			t.Fatalf("create resume loop: %v", err)
		}

		rmTarget := &models.Loop{
			Name:     "cmd-rm-target",
			RepoPath: repo,
			State:    models.LoopStateRunning,
		}
		if err := loopRepo.Create(context.Background(), rmTarget); err != nil {
			t.Fatalf("create rm loop: %v", err)
		}

		loopStopAll = false
		loopStopRepo = ""
		loopStopPool = ""
		loopStopProfile = ""
		loopStopState = ""
		loopStopTag = ""
		if err := loopStopCmd.RunE(loopStopCmd, []string{running.Name}); err != nil {
			t.Fatalf("loop stop: %v", err)
		}

		items, err := queueRepo.List(context.Background(), running.ID)
		if err != nil {
			t.Fatalf("list queue after stop: %v", err)
		}
		if len(items) != 1 || items[0].Type != models.LoopQueueItemStopGraceful {
			t.Fatalf("expected one stop-graceful item, got %#v", items)
		}

		loopKillAll = false
		loopKillRepo = ""
		loopKillPool = ""
		loopKillProfile = ""
		loopKillState = ""
		loopKillTag = ""
		if err := loopKillCmd.RunE(loopKillCmd, []string{running.Name}); err != nil {
			t.Fatalf("loop kill: %v", err)
		}

		items, err = queueRepo.List(context.Background(), running.ID)
		if err != nil {
			t.Fatalf("list queue after kill: %v", err)
		}
		if len(items) != 2 || items[1].Type != models.LoopQueueItemKillNow {
			t.Fatalf("expected trailing kill-now item, got %#v", items)
		}

		updated, err := loopRepo.Get(context.Background(), running.ID)
		if err != nil {
			t.Fatalf("get running loop after kill: %v", err)
		}
		if updated.State != models.LoopStateStopped {
			t.Fatalf("expected stopped state after kill, got %s", updated.State)
		}

		err = loopResumeCmd.RunE(loopResumeCmd, []string{resumeBlocked.Name})
		if err == nil || !strings.Contains(err.Error(), "only stopped or errored loops can be resumed") {
			t.Fatalf("expected resume guard error, got %v", err)
		}

		loopRmAll = false
		loopRmRepo = ""
		loopRmPool = ""
		loopRmProfile = ""
		loopRmState = ""
		loopRmTag = ""
		loopRmForce = false
		err = loopRmCmd.RunE(loopRmCmd, []string{rmTarget.Name})
		if err == nil || !strings.Contains(err.Error(), "use --force") {
			t.Fatalf("expected rm force guard error, got %v", err)
		}

		loopRmForce = true
		if err := loopRmCmd.RunE(loopRmCmd, []string{rmTarget.Name}); err != nil {
			t.Fatalf("loop rm --force: %v", err)
		}

		_, err = loopRepo.Get(context.Background(), rmTarget.ID)
		if !errors.Is(err, db.ErrLoopNotFound) {
			t.Fatalf("expected rm target to be deleted, got err=%v", err)
		}
	})
}
