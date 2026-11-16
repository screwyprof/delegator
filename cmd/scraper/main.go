// Command scraper backfills every Tezos delegation from the TzKT API into the database, then keeps
// polling for new ones until it is stopped.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/screwyprof/delegator/pkg/logger"
	"github.com/screwyprof/delegator/pkg/pgxdb"
	"github.com/screwyprof/delegator/pkg/tzkt"
	"github.com/screwyprof/delegator/scraper"
	"github.com/screwyprof/delegator/scraper/config"
	"github.com/screwyprof/delegator/scraper/store/pgxstore"
)

// Set at build time with -ldflags "-X main.version=... -X main.date=...".
//
//nolint:gochecknoglobals // stamped by the linker, so they must be package-level variables
var (
	version = "dev"
	date    = "unknown"
)

func main() {
	// os.Exit only here, after run has returned: an os.Exit inside run would skip its defers
	// (closing the pool and store, stopping signal delivery).
	if err := run(); err != nil {
		slog.Error("Scraper service failed", slog.Any("error", err))
		os.Exit(1)
	}
}

func run() error {
	cfg := config.New()

	log := logger.NewFromConfig(logger.Config{
		LogLevel:         cfg.LogLevel,
		LogHumanFriendly: cfg.LogHumanFriendly,
	})
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Schema and starting checkpoint are the migrator's job; it has already run.
	db, err := pgxdb.NewConnection(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer db.Close()

	store := pgxstore.New(db)
	httpClient := &http.Client{Timeout: cfg.HTTPClientTimeout}
	tzktClient := tzkt.NewClient(httpClient, cfg.TzktAPIURL)

	scraperService := scraper.NewService(
		tzktClient,
		store,
		scraper.WithChunkSize(cfg.ChunkSize),
		scraper.WithPollInterval(cfg.PollInterval),
	)

	log.InfoContext(ctx, "Starting delegation scraper service",
		slog.Uint64("chunkSize", cfg.ChunkSize),
		slog.String("version", version),
		slog.String("date", date),
	)

	events, done := scraperService.Start(ctx)

	subCloser := setupEventLogging(ctx, events, log)

	<-done
	subCloser() // log every remaining event before the final line, so the log ends in order

	// The service retries every failure and stops only when ctx is cancelled. If it ever stops
	// otherwise, exit non-zero so a restart policy brings it back instead of reading success.
	if ctx.Err() == nil {
		return ErrStoppedUnexpectedly
	}

	log.InfoContext(ctx, "Scraper service stopped gracefully")

	return nil
}

// ErrStoppedUnexpectedly means the scraper stopped without being asked to.
var ErrStoppedUnexpectedly = errors.New("scraper service stopped without a shutdown request")

// setupEventLogging configures event handlers using slog directly.
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
			log.ErrorContext(
				ctx,
				"Backfill step failed, retrying after the poll interval",
				slog.Any("error", event.Err),
			)
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
			log.ErrorContext(ctx, "Polling cycle failed, retrying on the next one", slog.Any("error", event.Err))
		}),
	)
}
