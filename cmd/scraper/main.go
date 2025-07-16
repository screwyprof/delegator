package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/screwyprof/delegator/cmd/scraper/config"
	"github.com/screwyprof/delegator/pkg/logger"
	"github.com/screwyprof/delegator/pkg/pgxdb"
	"github.com/screwyprof/delegator/pkg/tzkt"
	"github.com/screwyprof/delegator/scraper"
	"github.com/screwyprof/delegator/scraper/store/pgxstore"
)

func main() {
	// Load configuration
	cfg := config.New()

	// Initialize logger and set as default
	log := logger.NewFromConfig(logger.Config{
		LogLevel:         cfg.LogLevel,
		LogHumanFriendly: cfg.LogHumanFriendly,
	})
	slog.SetDefault(log)

	// Prepare context with signal handling
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Database connection
	db, err := pgxdb.NewConnection(ctx, cfg.DatabaseURL)
	if err != nil {
		log.ErrorContext(ctx, "Failed to connect to database", slog.Any("error", err))
		os.Exit(1)
	}
	defer db.Close()

	// Apply migrations
	log.InfoContext(ctx, "Applying database migrations")
	if err := pgxdb.ApplyMigrations(db, "./migrations"); err != nil {
		log.ErrorContext(ctx, "Failed to apply migrations", slog.Any("error", err))
		os.Exit(1)
	}

	// Initialize checkpoint
	if err := pgxdb.InittialiseCheckpointIfNotSet(ctx, db, cfg.InitialCheckpoint); err != nil {
		log.ErrorContext(ctx, "Failed to initialize checkpoint", slog.Any("error", err))
		os.Exit(1)
	}

	// Initialize store
	store, storeCloser := pgxstore.New(db)
	defer storeCloser()

	// HTTP client & tzkt client
	httpClient := &http.Client{Timeout: cfg.HttpClientTimeout}
	tzktClient := tzkt.NewClient(httpClient, cfg.TzktAPIURL)

	// Create scraper service
	scraperService := scraper.NewService(
		tzktClient,
		store,
		scraper.WithChunkSize(cfg.ChunkSize),
		scraper.WithPollInterval(cfg.PollInterval),
	)

	// Start service
	log.InfoContext(ctx, "Starting delegation scraper service",
		slog.Uint64("chunkSize", cfg.ChunkSize),
		slog.Uint64("initialCheckpoint", cfg.InitialCheckpoint),
	)
	events, done := scraperService.Start(ctx)

	// Subscribe to events for logging
	subCloser := setupEventLogging(ctx, events, log)
	defer subCloser()

	// Wait for shutdown
	<-done
	log.InfoContext(ctx, "Scraper service stopped gracefully")
}

// setupEventLogging configures event handlers using slog directly
func setupEventLogging(ctx context.Context, events <-chan scraper.Event, log *slog.Logger) func() {
	return scraper.NewSubscriber(events,
		scraper.OnBackfillStarted(func(event scraper.BackfillStarted) {
			log.InfoContext(ctx, "Backfill started",
				slog.String("startedAt", event.StartedAt.Format(logger.BritishTimeFormat)),
				slog.Int64("checkpointID", event.CheckpointID),
			)
		}),
		scraper.OnBackfillSyncCompleted(func(event scraper.BackfillSyncCompleted) {
			log.InfoContext(ctx, "Backfill batch completed",
				slog.Int("fetched", event.Fetched),
				slog.Int64("checkpointID", event.CheckpointID),
				slog.Uint64("chunkSize", event.ChunkSize),
			)
		}),
		scraper.OnBackfillDone(func(event scraper.BackfillDone) {
			log.InfoContext(ctx, "Backfill completed",
				slog.Int64("totalProcessed", event.TotalProcessed),
				slog.Duration("duration", event.Duration),
			)
		}),
		scraper.OnBackfillError(func(event scraper.BackfillError) {
			log.ErrorContext(ctx, "Backfill failed", slog.Any("error", event.Err))
		}),
		scraper.OnPollingStarted(func(event scraper.PollingStarted) {
			log.InfoContext(ctx, "Polling started",
				slog.Duration("interval", event.Interval),
			)
		}),
		scraper.OnPollingSyncCompleted(func(event scraper.PollingSyncCompleted) {
			if event.Fetched > 0 {
				log.InfoContext(ctx, "Polling cycle completed",
					slog.Int("fetched", event.Fetched),
					slog.Int64("checkpointID", event.CheckpointID),
					slog.Uint64("chunkSize", event.ChunkSize),
				)
			} else {
				log.InfoContext(ctx, "Polling cycle completed, no new records")
			}
		}),
		scraper.OnPollingShutdown(func(event scraper.PollingShutdown) {
			log.InfoContext(ctx, "Polling stopped",
				slog.String("reason", event.Reason.Error()),
			)
		}),
		scraper.OnPollingError(func(event scraper.PollingError) {
			log.ErrorContext(ctx, "Polling failed", slog.Any("error", event.Err))
		}),
	)
}
