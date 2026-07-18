package sessionsteps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cucumber/godog"

	"agentx/internal/session"
	"agentx/internal/state"
)

type persistenceWorld struct {
	dir      string
	id       session.Identity
	recorder *session.Recorder
	bus      *state.Bus
	sub      *state.Subscription
	runDone  chan error
	epoch    int64
}

// registerPersistenceSteps wires the event-persistence steps (CHT-A4).
func registerPersistenceSteps(sc *godog.ScenarioContext) {
	w := &persistenceWorld{}

	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		if w.sub != nil {
			w.sub.Close()
		}
		if w.dir != "" {
			_ = os.RemoveAll(w.dir)
		}
		*w = persistenceWorld{}
		return ctx, nil
	})

	sc.Step(`^a session with a recorder$`, w.sessionWithRecorder)
	sc.Step(`^a session with a recorder draining the event bus$`, w.sessionWithRecorderOnBus)
	sc.Step(`^a "([^"]*)" event with epoch (\d+) is recorded$`, w.recordEvent)
	sc.Step(`^(\d+) events are published to the bus for the session$`, w.publishToBus)
	sc.Step(`^(\d+) event files exist under the session events directory$`, w.eventFileCount)
	sc.Step(`^loading events returns epochs in order (.+)$`, w.loadedEpochOrder)
	sc.Step(`^loading events returns (\d+) events$`, w.loadedCount)
}

func (w *persistenceWorld) newSession() error {
	dir, err := os.MkdirTemp("", "agentx-persist-")
	if err != nil {
		return err
	}
	w.dir = dir
	store := session.NewStore(dir)
	w.id, err = store.Create()
	if err != nil {
		return err
	}
	w.recorder = store.Recorder(w.id.ID)
	return nil
}

func (w *persistenceWorld) sessionWithRecorder() error {
	return w.newSession()
}

func (w *persistenceWorld) sessionWithRecorderOnBus() error {
	if err := w.newSession(); err != nil {
		return err
	}
	w.bus = state.NewBus()
	sub := w.bus.Subscribe()
	w.sub = sub
	w.runDone = make(chan error, 1)
	go func() { w.runDone <- w.recorder.Run(sub) }()
	return nil
}

func (w *persistenceWorld) recordEvent(contentType string, epoch int64) error {
	return w.recorder.Write(state.Event{
		Epoch:       epoch,
		SessionID:   w.id.ID,
		EventType:   strings.ToUpper(contentType),
		ContentType: state.ContentType(contentType),
		Payload:     map[string]any{"n": epoch},
	})
}

func (w *persistenceWorld) publishToBus(n int) error {
	for i := 1; i <= n; i++ {
		w.bus.Publish(state.Event{
			Epoch:       int64(i),
			SessionID:   w.id.ID,
			EventType:   "AGENT_CONTENT",
			ContentType: state.ContentAgentResponse,
			Payload:     map[string]any{"seq": i},
		})
	}
	// Stop the recorder and wait for it to drain.
	w.sub.Close()
	w.sub = nil
	return <-w.runDone
}

func (w *persistenceWorld) eventFileCount(want int) error {
	entries, err := os.ReadDir(filepath.Join(w.dir, w.id.ID, "events"))
	if err != nil {
		return fmt.Errorf("read events dir: %w", err)
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			count++
		}
	}
	if count != want {
		return fmt.Errorf("event file count = %d, want %d", count, want)
	}
	return nil
}

func (w *persistenceWorld) loadedEpochOrder(list string) error {
	events, err := w.recorder.Load()
	if err != nil {
		return err
	}
	want := parseInts(list)
	if len(events) != len(want) {
		return fmt.Errorf("loaded %d events, want %d", len(events), len(want))
	}
	for i, ev := range events {
		if ev.Epoch != want[i] {
			return fmt.Errorf("event %d epoch = %d, want %d", i, ev.Epoch, want[i])
		}
	}
	return nil
}

func (w *persistenceWorld) loadedCount(want int) error {
	events, err := w.recorder.Load()
	if err != nil {
		return err
	}
	if len(events) != want {
		return fmt.Errorf("loaded %d events, want %d", len(events), want)
	}
	return nil
}

func parseInts(list string) []int64 {
	parts := strings.Split(list, ",")
	out := make([]int64, 0, len(parts))
	for _, p := range parts {
		if n, err := strconv.ParseInt(strings.TrimSpace(p), 10, 64); err == nil {
			out = append(out, n)
		}
	}
	return out
}
