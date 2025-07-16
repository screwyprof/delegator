package scraper

// Subscriber handles event subscriptions.
type Subscriber struct {
	done                   chan struct{}
	backfillHandler        func(BackfillDone)
	backfillStartedHandler func(BackfillStarted)
	backfillSyncHandler    func(BackfillSyncCompleted)
	backfillErrorHandler   func(BackfillError)
	pollingSyncHandler     func(PollingSyncCompleted)
	pollStartedHandler     func(PollingStarted)
	pollShutdownHandler    func(PollingShutdown)
	pollingErrorHandler    func(PollingError)
}

// OnBackfillDone sets the handler for BackfillDone events
func OnBackfillDone(fn func(BackfillDone)) func(*Subscriber) {
	return func(s *Subscriber) { s.backfillHandler = fn }
}

// OnBackfillStarted sets the handler for BackfillStarted events
func OnBackfillStarted(fn func(BackfillStarted)) func(*Subscriber) {
	return func(s *Subscriber) { s.backfillStartedHandler = fn }
}

// OnBackfillSyncCompleted sets the handler for BackfillSyncCompleted events
func OnBackfillSyncCompleted(fn func(BackfillSyncCompleted)) func(*Subscriber) {
	return func(s *Subscriber) { s.backfillSyncHandler = fn }
}

// OnBackfillError sets the handler for BackfillError events
func OnBackfillError(fn func(BackfillError)) func(*Subscriber) {
	return func(s *Subscriber) { s.backfillErrorHandler = fn }
}

// OnPollingSyncCompleted sets the handler for PollingSyncCompleted events
func OnPollingSyncCompleted(fn func(PollingSyncCompleted)) func(*Subscriber) {
	return func(s *Subscriber) { s.pollingSyncHandler = fn }
}

// OnPollingStarted sets the handler for PollingStarted events
func OnPollingStarted(fn func(PollingStarted)) func(*Subscriber) {
	return func(s *Subscriber) { s.pollStartedHandler = fn }
}

// OnPollingShutdown sets the handler for PollingShutdown events
func OnPollingShutdown(fn func(PollingShutdown)) func(*Subscriber) {
	return func(s *Subscriber) { s.pollShutdownHandler = fn }
}

// OnPollingError sets the handler for PollingError events
func OnPollingError(fn func(PollingError)) func(*Subscriber) {
	return func(s *Subscriber) { s.pollingErrorHandler = fn }
}

// NewSubscriber creates a Subscriber with the given options and starts the dispatch loop.
// Returns a closer function that waits for all events to be processed.
//
// Cleanup guarantee pattern:
//
//	The closer function ensures all events are handled before returning.
//	Use defer closer() immediately to guarantee cleanup before function exit.
//
// Example:
//
//	closer := scraper.NewSubscriber(events,
//	  scraper.OnBackfillDone(func(b BackfillDone) { ... }),
//	)
//	defer closer()  // Ensures all events processed before exit
//
// The subscriber processes events until the events channel closes,
// then the closer function confirms all processing is complete.
func NewSubscriber(events <-chan Event, opts ...func(*Subscriber)) func() {
	s := &Subscriber{
		done:                   make(chan struct{}),
		backfillHandler:        func(BackfillDone) {},          // nop by default
		backfillStartedHandler: func(BackfillStarted) {},       // nop by default
		backfillSyncHandler:    func(BackfillSyncCompleted) {}, // nop by default
		backfillErrorHandler:   func(BackfillError) {},         // nop by default
		pollingSyncHandler:     func(PollingSyncCompleted) {},  // nop by default
		pollStartedHandler:     func(PollingStarted) {},        // nop by default
		pollShutdownHandler:    func(PollingShutdown) {},       // nop by default
		pollingErrorHandler:    func(PollingError) {},          // nop by default
	}

	for _, opt := range opts {
		opt(s)
	}

	// Start the dispatch loop immediately
	go func() {
		defer close(s.done)
		for ev := range events {
			switch e := ev.(type) {
			case BackfillStarted:
				s.backfillStartedHandler(e)
			case BackfillSyncCompleted:
				s.backfillSyncHandler(e)
			case BackfillDone:
				s.backfillHandler(e)
			case BackfillError:
				s.backfillErrorHandler(e)
			case PollingStarted:
				s.pollStartedHandler(e)
			case PollingSyncCompleted:
				s.pollingSyncHandler(e)
			case PollingShutdown:
				s.pollShutdownHandler(e)
			case PollingError:
				s.pollingErrorHandler(e)
			}
		}
	}()

	return func() {
		<-s.done
	}
}
