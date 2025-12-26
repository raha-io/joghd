package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/raha-io/joghd/internal/alerter"
	"github.com/raha-io/joghd/internal/checker"
	"github.com/raha-io/joghd/internal/config"
	"github.com/raha-io/joghd/internal/domain"
	"github.com/raha-io/joghd/internal/scheduler"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	configPath := flag.String("config", "config.toml", "Path to configuration file")
	mode := flag.String("mode", "", "Run mode: oneshot or continuous (overrides config)")
	showVersion := flag.Bool("version", false, "Show version information")
	flag.Parse()

	if *showVersion {
		fmt.Printf("joghd %s (commit: %s, built: %s)\n", version, commit, date)
		os.Exit(0)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Command-line mode flag overrides config
	if *mode != "" {
		cfg.App.Mode = *mode
	}

	if len(cfg.Targets) == 0 {
		log.Fatal("No targets configured")
	}

	log.Printf("Joghd starting in %s mode with %d targets", cfg.App.Mode, len(cfg.Targets))

	// Create HTTP client
	httpClient := checker.NewRestyClient(cfg.HTTP)

	// Create checker
	chk := checker.New(
		checker.WithHTTPClient(httpClient),
		checker.WithRetryConfig(cfg.Retry),
		checker.WithConcurrency(cfg.App.Concurrency),
	)

	// Create alerter
	alt := buildAlerter(cfg)

	// Create context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Printf("Received signal %v, shutting down...", sig)
		cancel()
	}()

	// Run based on mode
	switch cfg.App.Mode {
	case "oneshot":
		exitCode := runOneshot(ctx, chk, alt, cfg.Targets)
		os.Exit(exitCode)
	case "continuous":
		runContinuous(ctx, chk, alt, cfg.Targets)
	default:
		log.Fatalf("Unknown mode: %s", cfg.App.Mode)
	}
}

func buildAlerter(cfg *config.Config) alerter.Alerter {
	composite := alerter.NewCompositeAlerter()

	if cfg.Alerters.Telegram.Enabled {
		telegram := alerter.NewTelegramAlerter(cfg.Alerters.Telegram)
		composite.Add(telegram)
		log.Println("Telegram alerter enabled")
	}

	return composite
}

func runOneshot(ctx context.Context, chk checker.Checker, alt alerter.Alerter, targets []domain.Target) int {
	log.Println("Running oneshot health check...")

	results := chk.CheckAll(ctx, targets)

	hasFailures := false
	for _, result := range results {
		if result.Success {
			log.Printf("[OK] %s: status=%d, latency=%s",
				result.Target.Name, result.ActualStatus, result.Latency)
		} else {
			hasFailures = true
			log.Printf("[FAIL] %s: status=%d, expected=%d, error=%v",
				result.Target.Name, result.ActualStatus, result.Target.ExpectedStatus, result.Error)

			// Send failure alert
			alert := domain.NewFailureAlert(result)
			if err := alt.Send(ctx, alert); err != nil {
				log.Printf("Failed to send alert: %v", err)
			}
		}
	}

	if hasFailures {
		log.Println("Health check completed with failures")
		return 1
	}

	log.Println("Health check completed successfully")
	return 0
}

func runContinuous(ctx context.Context, chk checker.Checker, alt alerter.Alerter, targets []domain.Target) {
	log.Println("Starting continuous monitoring...")

	sched := scheduler.New(chk, alt, targets)
	if err := sched.Start(ctx); err != nil {
		log.Printf("Scheduler error: %v", err)
	}
}
