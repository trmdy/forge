package loop

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/tOgg1/forge/internal/config"
	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/harness"
	"github.com/tOgg1/forge/internal/logging"
	"github.com/tOgg1/forge/internal/models"
)

const (
	defaultOutputTailLines   = 60
	defaultInterruptInterval = 1 * time.Second
	defaultWaitInterval      = 5 * time.Second
)

// ExecuteFunc runs a harness execution and returns exit code, output tail, and error.
type ExecuteFunc func(ctx context.Context, profile models.Profile, promptPath, promptContent, workDir string, output io.Writer) (int, string, error)

// Runner executes loop iterations for a specific loop.
type Runner struct {
	DB                    *db.DB
	Config                *config.Config
	Logger                zerolog.Logger
	OutputTailLines       int
	InterruptPollInterval time.Duration
	Exec                  ExecuteFunc
}

// NewRunner creates a Runner with default dependencies.
func NewRunner(database *db.DB, cfg *config.Config) *Runner {
	logger := logging.Component("loop")
	return &Runner{
		DB:                    database,
		Config:                cfg,
		Logger:                logger,
		OutputTailLines:       defaultOutputTailLines,
		InterruptPollInterval: defaultInterruptInterval,
		Exec:                  defaultExecute,
	}
}

// RunLoop runs the loop until stopped or context cancellation.
// RunLoop runs the loop until stopped or context cancellation.
func (r *Runner) RunLoop(ctx context.Context, loopID string) error {
	return r.runLoop(ctx, loopID, false)
}

// RunOnce runs a single loop iteration.
func (r *Runner) RunOnce(ctx context.Context, loopID string) error {
	return r.runLoop(ctx, loopID, true)
}

func (r *Runner) runLoop(ctx context.Context, loopID string, singleRun bool) error {
	if r.DB == nil || r.Config == nil {
		return errors.New("runner requires database and config")
	}
	if r.Exec == nil {
		r.Exec = defaultExecute
	}
	if r.OutputTailLines <= 0 {
		r.OutputTailLines = defaultOutputTailLines
	}
	if r.InterruptPollInterval <= 0 {
		r.InterruptPollInterval = defaultInterruptInterval
	}

	loopRepo := db.NewLoopRepository(r.DB)
	queueRepo := db.NewLoopQueueRepository(r.DB)
	runRepo := db.NewLoopRunRepository(r.DB)
	profileRepo := db.NewProfileRepository(r.DB)
	poolRepo := db.NewPoolRepository(r.DB)

	loop, err := loopRepo.Get(ctx, loopID)
	if err != nil {
		return err
	}

	if err := r.ensureLoopPaths(ctx, loop, loopRepo); err != nil {
		return err
	}

	logWriter, err := newLoopLogger(loop.LogPath)
	if err != nil {
		return err
	}
	defer logWriter.Close()

	if err := r.attachLoopPID(ctx, loop, loopRepo); err != nil {
		logWriter.WriteLine(fmt.Sprintf("warning: failed to record pid: %v", err))
	}

	maxIterations := loop.MaxIterations
	maxRuntime := time.Duration(loop.MaxRuntimeSeconds) * time.Second
	iterationCount := loopIterationCount(loop.Metadata)
	startedAt := loopStartedAt(loop.Metadata)
	if maxRuntime > 0 && startedAt.IsZero() {
		startedAt = time.Now().UTC()
		setLoopStartedAt(loop, startedAt)
		_ = loopRepo.Update(ctx, loop)
	}

	loop.State = models.LoopStateRunning
	if err := loopRepo.Update(ctx, loop); err != nil {
		return err
	}

	logWriter.WriteLine("loop started")

	pendingSteer := make([]messageEntry, 0)

	for {
		if ctx.Err() != nil {
			logWriter.WriteLine("loop context cancelled")
			loop.State = models.LoopStateStopped
			_ = loopRepo.Update(ctx, loop)
			return ctx.Err()
		}

		if maxIterations > 0 && iterationCount >= maxIterations {
			reason := fmt.Sprintf("max iterations reached (%d)", maxIterations)
			logWriter.WriteLine(reason)
			loop.State = models.LoopStateStopped
			loop.LastError = reason
			_ = loopRepo.Update(ctx, loop)
			return nil
		}

		if maxRuntime > 0 && time.Since(startedAt) >= maxRuntime {
			reason := fmt.Sprintf("max runtime reached (%s)", maxRuntime)
			logWriter.WriteLine(reason)
			loop.State = models.LoopStateStopped
			loop.LastError = reason
			_ = loopRepo.Update(ctx, loop)
			return nil
		}

		plan, err := buildQueuePlan(ctx, queueRepo, loop.ID, pendingSteer)
		pendingSteer = nil
		if err != nil {
			loop.State = models.LoopStateError
			loop.LastError = err.Error()
			_ = loopRepo.Update(ctx, loop)
			logWriter.WriteLine(fmt.Sprintf("queue planning error: %v", err))
			return err
		}

		if plan.StopRequested {
			logWriter.WriteLine("graceful stop requested")
			_ = markQueueCompleted(ctx, queueRepo, plan.StopItemIDs)
			loop.State = models.LoopStateStopped
			_ = loopRepo.Update(ctx, loop)
			return nil
		}

		if plan.KillRequested {
			logWriter.WriteLine("kill requested")
			_ = markQueueCompleted(ctx, queueRepo, plan.KillItemIDs)
			loop.State = models.LoopStateStopped
			_ = loopRepo.Update(ctx, loop)
			return nil
		}

		if plan.PauseDuration > 0 && plan.PauseBeforeRun {
			logWriter.WriteLine(fmt.Sprintf("pause for %s", plan.PauseDuration))
			loop.State = models.LoopStateSleeping
			_ = loopRepo.Update(ctx, loop)
			r.sleep(ctx, plan.PauseDuration)
			if ctx.Err() == nil {
				_ = markQueueCompleted(ctx, queueRepo, plan.PauseItemIDs)
			}
			continue
		}

		profile, waitUntil, err := r.selectProfile(ctx, loop, profileRepo, poolRepo, runRepo)
		if err != nil {
			loop.State = models.LoopStateError
			loop.LastError = err.Error()
			_ = loopRepo.Update(ctx, loop)
			logWriter.WriteLine(fmt.Sprintf("profile selection error: %v", err))
			return err
		}
		if waitUntil != nil {
			if loop.Metadata == nil {
				loop.Metadata = make(map[string]any)
			}
			loop.Metadata["wait_until"] = waitUntil.UTC().Format(time.RFC3339)
			loop.State = models.LoopStateWaiting
			loop.LastError = fmt.Sprintf("waiting for profile availability until %s", waitUntil.UTC().Format(time.RFC3339))
			_ = loopRepo.Update(ctx, loop)
			logWriter.WriteLine(loop.LastError)
			r.sleepUntil(ctx, *waitUntil)
			continue
		}
		if loop.Metadata != nil {
			delete(loop.Metadata, "wait_until")
		}

		prompt, err := resolveBasePrompt(loop)
		if err != nil {
			loop.State = models.LoopStateError
			loop.LastError = err.Error()
			_ = loopRepo.Update(ctx, loop)
			logWriter.WriteLine(fmt.Sprintf("prompt resolution error: %v", err))
			return err
		}

		if plan.OverridePrompt != nil {
			prompt, err = resolveOverridePrompt(loop.RepoPath, *plan.OverridePrompt)
			if err != nil {
				loop.State = models.LoopStateError
				loop.LastError = err.Error()
				_ = loopRepo.Update(ctx, loop)
				logWriter.WriteLine(fmt.Sprintf("override prompt error: %v", err))
				return err
			}
			prompt.Source = "override"
			prompt.Override = true
		}

		hasMessages := len(plan.Messages) > 0
		prompt.Content = appendOperatorMessages(prompt.Content, plan.Messages)

		run := &models.LoopRun{
			LoopID:         loop.ID,
			ProfileID:      profile.ID,
			PromptSource:   prompt.Source,
			PromptPath:     prompt.Path,
			PromptOverride: prompt.Override,
		}
		if err := runRepo.Create(ctx, run); err != nil {
			return err
		}

		effectivePromptPath, effectivePromptContent, err := r.preparePrompt(loop, run, profile, prompt, hasMessages)
		if err != nil {
			run.Status = models.LoopRunStatusError
			_ = runRepo.Finish(ctx, run)
			loop.State = models.LoopStateError
			loop.LastError = err.Error()
			_ = loopRepo.Update(ctx, loop)
			logWriter.WriteLine(fmt.Sprintf("prompt preparation error: %v", err))
			return err
		}

		loop.State = models.LoopStateRunning
		_ = loopRepo.Update(ctx, loop)

		logWriter.WriteLine(fmt.Sprintf("run %s start (profile=%s)", run.ID, profile.Name))

		runResult, interruptResult := r.runWithInterrupt(ctx, loop, run, profile, effectivePromptPath, effectivePromptContent, logWriter)

		run.Status = runResult.status
		run.ExitCode = &runResult.exitCode
		run.OutputTail = runResult.outputTail
		_ = runRepo.Finish(ctx, run)

		if run.FinishedAt != nil {
			loop.LastRunAt = run.FinishedAt
		} else {
			loop.LastRunAt = &run.StartedAt
		}
		loop.LastExitCode = run.ExitCode
		loop.LastError = runResult.errText
		loop.State = models.LoopStateSleeping
		iterationCount++
		setLoopIterationCount(loop, iterationCount)
		_ = loopRepo.Update(ctx, loop)

		_ = markQueueCompleted(ctx, queueRepo, plan.ConsumeItemIDs)

		if err := appendLedgerEntry(loop, run, profile, runResult.outputTail, r.OutputTailLines); err != nil {
			logWriter.WriteLine(fmt.Sprintf("ledger append failed: %v", err))
		}

		skipSleep := false
		if interruptResult != nil && interruptResult.killOnly {
			logWriter.WriteLine("run interrupted: kill")
			loop.State = models.LoopStateStopped
			_ = loopRepo.Update(ctx, loop)
			return nil
		}
		if interruptResult != nil && interruptResult.steerMessage != "" {
			logWriter.WriteLine("run interrupted: steer")
			contextMessage := buildInterruptContext(loop, r.OutputTailLines, r.OutputTailLines)
			if strings.TrimSpace(contextMessage) != "" {
				pendingSteer = append(pendingSteer, messageEntry{Text: contextMessage, Timestamp: time.Now().UTC(), Source: "context"})
			}
			pendingSteer = append(pendingSteer, messageEntry{Text: interruptResult.steerMessage, Timestamp: time.Now().UTC(), Source: "steer"})
			skipSleep = true
		}

		if killRequested, _ := hasPendingKill(ctx, queueRepo, loop.ID); killRequested {
			logWriter.WriteLine("kill queued")
			_ = consumePendingKill(ctx, queueRepo, loop.ID)
			loop.State = models.LoopStateStopped
			_ = loopRepo.Update(ctx, loop)
			return nil
		}

		if stopRequested, _ := hasPendingStop(ctx, queueRepo, loop.ID); stopRequested {
			logWriter.WriteLine("graceful stop queued")
			_ = consumePendingStop(ctx, queueRepo, loop.ID)
			loop.State = models.LoopStateStopped
			_ = loopRepo.Update(ctx, loop)
			return nil
		}

		if plan.PauseDuration > 0 && !plan.PauseBeforeRun {
			logWriter.WriteLine(fmt.Sprintf("pause for %s", plan.PauseDuration))
			loop.State = models.LoopStateSleeping
			_ = loopRepo.Update(ctx, loop)
			r.sleep(ctx, plan.PauseDuration)
			if ctx.Err() == nil {
				_ = markQueueCompleted(ctx, queueRepo, plan.PauseItemIDs)
			}
			skipSleep = true
		}

		if singleRun {
			loop.State = models.LoopStateStopped
			_ = loopRepo.Update(ctx, loop)
			return nil
		}

		if !skipSleep {
			interval := time.Duration(loop.IntervalSeconds) * time.Second
			r.sleep(ctx, interval)
		}
	}
}

func (r *Runner) preparePrompt(loop *models.Loop, run *models.LoopRun, profile *models.Profile, prompt promptSpec, hasMessages bool) (string, string, error) {
	promptPath := prompt.Path
	promptContent := prompt.Content

	needsRender := !prompt.FromFile || hasMessages

	if profile.PromptMode == models.PromptModePath {
		if promptPath == "" || needsRender {
			path, err := r.writePromptFile(loop.ID, run.ID, promptContent)
			if err != nil {
				return "", "", err
			}
			promptPath = path
		}
		return promptPath, promptContent, nil
	}

	return promptPath, promptContent, nil
}

func (r *Runner) runWithInterrupt(ctx context.Context, loop *models.Loop, run *models.LoopRun, profile *models.Profile, promptPath, promptContent string, logWriter *loopLogger) (runResult, *interruptResult) {
	resultCh := make(chan runResult, 1)
	interruptCh := make(chan interruptResult, 1)

	runCtx, runCancel := context.WithCancel(ctx)
	defer runCancel()
	watchCtx, watchCancel := context.WithCancel(ctx)
	defer watchCancel()

	start := time.Now().UTC()

	go func() {
		outputWriter := newTailWriter(r.OutputTailLines)
		writer := io.MultiWriter(logWriter, outputWriter)
		exitCode, outputTail, err := r.Exec(runCtx, *profile, promptPath, promptContent, loop.RepoPath, writer)
		resultCh <- runResult{
			status:     statusFromResult(err),
			exitCode:   exitCode,
			outputTail: outputTailOrFallback(outputTail, outputWriter.String()),
			errText:    errText(err),
		}
	}()

	go func() {
		interrupt, err := watchInterrupts(watchCtx, r.DB, loop.ID, start, r.InterruptPollInterval)
		if err == nil && interrupt != nil {
			interruptCh <- *interrupt
		}
	}()

	select {
	case res := <-resultCh:
		watchCancel()
		return res, nil
	case interrupt := <-interruptCh:
		runCancel()
		res := <-resultCh
		watchCancel()
		res.status = models.LoopRunStatusKilled
		res.errText = interrupt.reason
		return res, &interrupt
	}
}

func (r *Runner) ensureLoopPaths(ctx context.Context, loop *models.Loop, repo *db.LoopRepository) error {
	updated := false
	if loop.LogPath == "" {
		path := LogPath(r.Config.Global.DataDir, loop.Name, loop.ID)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		loop.LogPath = path
		updated = true
	} else {
		if err := os.MkdirAll(filepath.Dir(loop.LogPath), 0o755); err != nil {
			return err
		}
	}
	if loop.LedgerPath == "" {
		path := LedgerPath(loop.RepoPath, loop.Name, loop.ID)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		loop.LedgerPath = path
		updated = true
	}
	if updated {
		if err := repo.Update(ctx, loop); err != nil {
			return err
		}
	}

	return ensureLedgerFile(loop)
}

func (r *Runner) attachLoopPID(ctx context.Context, loop *models.Loop, repo *db.LoopRepository) error {
	if loop.Metadata == nil {
		loop.Metadata = make(map[string]any)
	}
	loop.Metadata["pid"] = os.Getpid()
	loop.Metadata["started_at"] = time.Now().UTC().Format(time.RFC3339)
	loop.Metadata["iteration_count"] = 0
	return repo.Update(ctx, loop)
}

func loopIterationCount(metadata map[string]any) int {
	if metadata == nil {
		return 0
	}
	value, ok := metadata["iteration_count"]
	if !ok {
		return 0
	}
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		parsed, err := strconv.Atoi(v)
		if err != nil {
			return 0
		}
		return parsed
	default:
		return 0
	}
}

func setLoopIterationCount(loop *models.Loop, count int) {
	if loop.Metadata == nil {
		loop.Metadata = make(map[string]any)
	}
	loop.Metadata["iteration_count"] = count
}

func loopStartedAt(metadata map[string]any) time.Time {
	if metadata == nil {
		return time.Time{}
	}
	value, ok := metadata["started_at"]
	if !ok {
		return time.Time{}
	}
	switch v := value.(type) {
	case time.Time:
		return v
	case string:
		parsed, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return time.Time{}
		}
		return parsed
	default:
		return time.Time{}
	}
}

func setLoopStartedAt(loop *models.Loop, startedAt time.Time) {
	if loop.Metadata == nil {
		loop.Metadata = make(map[string]any)
	}
	loop.Metadata["started_at"] = startedAt.UTC().Format(time.RFC3339)
}

func (r *Runner) sleep(ctx context.Context, duration time.Duration) {
	if duration <= 0 {
		return
	}

	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

func (r *Runner) sleepUntil(ctx context.Context, when time.Time) {
	if when.IsZero() {
		r.sleep(ctx, defaultWaitInterval)
		return
	}
	wait := time.Until(when)
	if wait < 0 {
		wait = defaultWaitInterval
	}
	r.sleep(ctx, wait)
}

func defaultExecute(ctx context.Context, profile models.Profile, promptPath, promptContent, workDir string, output io.Writer) (int, string, error) {
	execPlan, err := harness.BuildExecution(ctx, profile, promptPath, promptContent)
	if err != nil {
		return -1, "", err
	}
	execPlan.Cmd.Dir = workDir
	execPlan.Cmd.Stdout = output
	execPlan.Cmd.Stderr = output

	err = execPlan.Cmd.Run()
	return exitCodeFromError(err), "", err
}

func exitCodeFromError(err error) int {
	if err == nil {
		return 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode()
	}
	return -1
}

func statusFromResult(err error) models.LoopRunStatus {
	if err == nil {
		return models.LoopRunStatusSuccess
	}
	return models.LoopRunStatusError
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func outputTailOrFallback(primary, fallback string) string {
	if primary != "" {
		return primary
	}
	return fallback
}

type runResult struct {
	status     models.LoopRunStatus
	exitCode   int
	outputTail string
	errText    string
}
