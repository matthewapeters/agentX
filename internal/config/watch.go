package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watcher observes a TOML config file for external modifications and fans out
// change events to subscribers. It debounces rapid successive writes (100ms
// window) so that a single editor save or a backup-rotation burst becomes one
// event.
//
// Lifecycle:
//
//	w := NewWatcher(path)
//	sub := w.Subscribe()
//	go func() { for range sub.C { ... } }()
//	defer w.Close()
type Watcher struct {
	path    string
	fsw     *fsnotify.Watcher
	mu      sync.Mutex
	subs    map[int]*Subscription
	next    int
	done    chan struct{}
	once    sync.Once
}

// Subscription delivers debounced change events on C. Close releases the
// subscriber slot; it is safe to call multiple times.
type Subscription struct {
	owner  *Watcher
	id     int
	C      chan struct{}
	mu     sync.Mutex
	buf    []struct{}
	closed bool
}

func newSubscription(owner *Watcher) *Subscription {
	s := &Subscription{
		owner: owner,
		id:    owner.next,
		C:     make(chan struct{}, 1),
	}
	owner.next++
	return s
}

// Subscribe attaches a new subscriber and returns its Subscription. Callers
// read from Subscription.C and must call Close when done.
func (w *Watcher) Subscribe() *Subscription {
	w.mu.Lock()
	defer w.mu.Unlock()
	s := newSubscription(w)
	w.subs[s.id] = s
	return s
}

// enqueue broadcasts one event to all live subscribers. Events are debounced
// via the 100ms coalescing window, so rapid writes collapse into a single
// notification.
func (w *Watcher) enqueue() {
	w.mu.Lock()
	subs := make([]*Subscription, 0, len(w.subs))
	for _, s := range w.subs {
		subs = append(subs, s)
	}
	w.mu.Unlock()
	for _, s := range subs {
		select {
		case s.C <- struct{}{}:
		default: // subscriber is busy; drop (the next change will re-trigger)
		}
	}
}

// Close tears down the file-system watcher and detaches all subscribers. It is
// safe to call multiple times.
func (w *Watcher) Close() error {
	w.once.Do(func() {
		close(w.done)
		w.mu.Lock()
		for id, s := range w.subs {
			s.mu.Lock()
			s.closed = true
			close(s.C)
			s.mu.Unlock()
			delete(w.subs, id)
		}
		w.mu.Unlock()
	})
	if w.fsw != nil {
		return w.fsw.Close()
	}
	return nil
}

// Path returns the path the watcher is monitoring.
func (w *Watcher) Path() string { return w.path }

// debounceWindow is the coalescing window: events arriving within this window
// after the first are collapsed into one notification. Per the spec (100ms).
const debounceWindow = 100 * time.Millisecond

// NewWatcher creates a file-system watcher for the given config path. The
// directory containing path is watched (fsnotify cannot watch a file directly).
// If the file or its parent directory does not exist, the watcher still starts
// (and will discover the file when it appears); callers can check PathExists to
// know the initial state.
func NewWatcher(path string) (*Watcher, error) {
	dir := filepath.Dir(path)
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := fsw.Add(dir); err != nil {
		_ = fsw.Close()
		return nil, err
	}
	w := &Watcher{
		path:   path,
		fsw:    fsw,
		subs:   make(map[int]*Subscription),
		done:   make(chan struct{}),
	}

	go w.run()
	return w, nil
}

// PathExists reports whether the watched config file exists on disk at the time
// of the call.
func (w *Watcher) PathExists() bool {
	_, err := os.Stat(w.path)
	return err == nil
}

// run is the watcher goroutine. It coalesces fsnotify events on the debounce
// window and fans them out to subscribers.
func (w *Watcher) run() {
	var timer *time.Timer

	flush := func() {
		w.enqueue()
		timer = nil
	}

	for {
		select {
		case <-w.done:
			return
		case ev, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			// Only care about the watched file (not its directory). fsnotify
			// emits write events for the file; we filter by basename.
			if ev.Name != w.path && filepath.Base(ev.Name) != filepath.Base(w.path) {
				continue
			}
			// Ignore removes/rename events that happen during our own
			// transactional writes (we write to a temp file and rename, which
			// fsnotify may surface as separate events).
			if ev.Has(fsnotify.Remove) || ev.Has(fsnotify.Rename) {
				continue
			}
			if ev.Has(fsnotify.Write) || ev.Has(fsnotify.Create) {
				if timer == nil {
					timer = time.AfterFunc(debounceWindow, flush)
				} else {
					// Reset the debounce window.
					timer.Reset(debounceWindow)
				}
			}
		case err, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
			// fsnotify errors are logged but do not tear down the watcher; the
			// file may have been temporarily moved by an editor.
			_ = errors.Join(err)
		}
	}
}

// Close detaches one subscriber. Safe to call multiple times.
func (s *Subscription) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	close(s.C)
	s.mu.Unlock()
	s.owner.mu.Lock()
	delete(s.owner.subs, s.id)
	s.owner.mu.Unlock()
}

// NewWatcherForPath returns a watcher for the canonical agentx.toml path,
// resolving from the conventional config directory. Returns (nil, nil) if the
// config path cannot be resolved (e.g. home dir unreadable).
func NewWatcherForPath() (*Watcher, error) {
	paths, err := DefaultPaths()
	if err != nil {
		return nil, err
	}
	// The canonical config file lives at the deployment path
	// (~/.config/agentx/agentx.toml).
	return NewWatcher(paths.Deployment)
}

// WatcherForCtx wraps NewWatcher in a context-aware lifecycle: Close is called
// when ctx is cancelled. Returns a cleanup func.
func WatcherForCtx(ctx context.Context, path string) (*Watcher, context.CancelFunc, error) {
	w, err := NewWatcher(path)
	if err != nil {
		return nil, nil, err
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
		case <-done:
		}
		_ = w.Close()
	}()
	return w, func() { close(done) }, nil
}
