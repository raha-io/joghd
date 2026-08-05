package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/rahacloud/joghd/internal/alerter"
	"github.com/rahacloud/joghd/internal/checker"
	"github.com/rahacloud/joghd/internal/config"
	"github.com/rahacloud/joghd/internal/domain"
)

// targetState tracks one target between checks. Each instance belongs to the
// single goroutine running that target's loop, so it needs no locking and two
// targets sharing a URL cannot interfere with each other.
type targetState struct {
	status        domain.HealthStatus
	lastAlertTime time.Time
}

// Scheduler manages periodic health checks for multiple targets.
type Scheduler struct {
	checker            checker.Checker
	alerter            alerter.Alerter
	targets            []domain.Target
	reminderMultiplier int
}

// New creates a new scheduler.
func New(chk checker.Checker, alt alerter.Alerter, targets []domain.Target, appCfg config.AppConfig) *Scheduler {
	return &Scheduler{
		checker:            chk,
		alerter:            alt,
		targets:            targets,
		reminderMultiplier: appCfg.ReminderMultiplier,
	}
}

// Start begins the scheduling loop. Blocks until context is cancelled.
func (s *Scheduler) Start(ctx context.Context) error {
	var wg sync.WaitGroup

	for _, target := range s.targets {
		wg.Go(func() {
			s.runTargetLoop(ctx, target)
		})
	}

	slog.Info("Scheduler started", "targets", len(s.targets))

	// Wait for all goroutines to finish
	wg.Wait()

	slog.Info("Scheduler stopped")

	return nil
}

func (s *Scheduler) runTargetLoop(ctx context.Context, target domain.Target) {
	ticker := time.NewTicker(target.Interval)
	defer ticker.Stop()

	state := targetState{status: domain.StatusUnknown}

	// Run initial check immediately
	s.checkAndAlert(ctx, target, &state)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.checkAndAlert(ctx, target, &state)
		}
	}
}

func (s *Scheduler) checkAndAlert(ctx context.Context, target domain.Target, state *targetState) {
	result := s.checker.Check(ctx, target)

	currentStatus := domain.StatusUnhealthy
	if result.Success {
		currentStatus = domain.StatusHealthy
	}

	previousStatus := state.status
	state.status = currentStatus

	switch {
	case !result.Success && previousStatus != domain.StatusUnhealthy:
		// First failure, or first check of an already-broken target.
		state.lastAlertTime = time.Now()
		s.send(ctx, target, domain.NewFailureAlert(result), "failure", slog.LevelWarn)

	case !result.Success && s.reminderDue(state, target):
		state.lastAlertTime = time.Now()
		s.send(ctx, target, domain.NewReminderAlert(result), "reminder", slog.LevelWarn)

	case !result.Success:
		slog.Warn("Target still unhealthy",
			"target", target.Name,
			"status", result.ActualStatus,
			"expected", target.ExpectedStatus,
		)

	case previousStatus == domain.StatusUnhealthy:
		// Recovered. A target that starts healthy has no recovery to report.
		state.lastAlertTime = time.Time{}
		s.send(ctx, target, domain.NewRecoveryAlert(result), "recovery", slog.LevelInfo)

	default:
		slog.Debug("Target healthy",
			"target", target.Name,
			"status", result.ActualStatus,
			"latency", result.Latency.Round(time.Millisecond),
		)
	}
}

// reminderDue reports whether a still-unhealthy target is due another alert.
func (s *Scheduler) reminderDue(state *targetState, target domain.Target) bool {
	if s.reminderMultiplier <= 0 {
		return false
	}

	return time.Since(state.lastAlertTime) >= target.Interval*time.Duration(s.reminderMultiplier)
}

// send fans an alert out to the configured alerters under a bounded context so
// that a slow alerter cannot stop this target from being checked again.
func (s *Scheduler) send(ctx context.Context, target domain.Target, alert domain.Alert, kind string, level slog.Level) {
	ctx, cancel := context.WithTimeout(ctx, config.AlertFanoutTimeout)
	defer cancel()

	if err := s.alerter.Send(ctx, alert); err != nil {
		slog.Error("Failed to send alert", "kind", kind, "target", target.Name, "error", err)

		return
	}

	slog.Log(ctx, level, "Sent alert", "kind", kind, "target", target.Name)
}
