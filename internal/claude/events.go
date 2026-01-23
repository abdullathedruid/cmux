package claude

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// EventReader reads hook events from a per-session JSONL file.
// File format: one JSON object per line with ts and tmux_session fields added by hook.
type EventReader struct {
	path      string
	offset    int64
	mu        sync.Mutex
	callbacks []func(HookEvent)
}

// NewEventReader creates a reader for a session's event file.
func NewEventReader(path string) *EventReader {
	return &EventReader{path: path}
}

// OnEvent registers a callback for new events.
func (r *EventReader) OnEvent(cb func(HookEvent)) {
	r.mu.Lock()
	r.callbacks = append(r.callbacks, cb)
	r.mu.Unlock()
}

// Poll reads new events from the file.
func (r *EventReader) Poll() ([]HookEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	file, err := os.Open(r.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()

	if _, err := file.Seek(r.offset, 0); err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(file)
	// Large buffer for tool responses
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 16*1024*1024)

	var events []HookEvent
	for scanner.Scan() {
		line := scanner.Bytes()
		r.offset += int64(len(line)) + 1

		var event HookEvent
		if err := json.Unmarshal(line, &event); err != nil {
			continue
		}

		event.ParseTimestamp()
		events = append(events, event)

		// Fire callbacks
		for _, cb := range r.callbacks {
			cb(event)
		}
	}

	return events, scanner.Err()
}

// EventWatcher watches for new event files and reads from them.
type EventWatcher struct {
	eventsDir string
	readers   map[string]*EventReader // tmux session -> reader
	mu        sync.RWMutex

	watcher   *fsnotify.Watcher
	callbacks []func(string, HookEvent) // (tmuxSession, event)
	stopCh    chan struct{}
}

// NewEventWatcher creates a watcher for the events directory.
func NewEventWatcher(eventsDir string) (*EventWatcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(eventsDir, 0755); err != nil {
		w.Close()
		return nil, err
	}

	if err := w.Add(eventsDir); err != nil {
		w.Close()
		return nil, err
	}

	return &EventWatcher{
		eventsDir: eventsDir,
		readers:   make(map[string]*EventReader),
		watcher:   w,
		stopCh:    make(chan struct{}),
	}, nil
}

// OnEvent registers a callback for events from any session.
func (w *EventWatcher) OnEvent(cb func(tmuxSession string, event HookEvent)) {
	w.mu.Lock()
	w.callbacks = append(w.callbacks, cb)
	w.mu.Unlock()
}

// Start begins watching for events.
func (w *EventWatcher) Start() error {
	// Initial scan
	w.scanAll()

	go w.watchLoop()
	return nil
}

// Stop stops the watcher.
func (w *EventWatcher) Stop() {
	close(w.stopCh)
	w.watcher.Close()
}

// GetReader returns the reader for a tmux session.
func (w *EventWatcher) GetReader(tmuxSession string) *EventReader {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.readers[tmuxSession]
}

// Sessions returns all known tmux session names.
func (w *EventWatcher) Sessions() []string {
	w.mu.RLock()
	defer w.mu.RUnlock()

	sessions := make([]string, 0, len(w.readers))
	for s := range w.readers {
		sessions = append(sessions, s)
	}
	return sessions
}

func (w *EventWatcher) watchLoop() {
	// Also poll periodically in case fsnotify misses events
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopCh:
			return

		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
				if filepath.Ext(event.Name) == ".jsonl" {
					w.handleFile(event.Name)
				}
			}

		case <-ticker.C:
			w.pollAll()

		case <-w.watcher.Errors:
			// Continue on errors
		}
	}
}

func (w *EventWatcher) scanAll() {
	entries, err := os.ReadDir(w.eventsDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".jsonl" {
			w.handleFile(filepath.Join(w.eventsDir, entry.Name()))
		}
	}
}

func (w *EventWatcher) pollAll() {
	w.mu.RLock()
	readers := make([]*EventReader, 0, len(w.readers))
	sessions := make([]string, 0, len(w.readers))
	for s, r := range w.readers {
		readers = append(readers, r)
		sessions = append(sessions, s)
	}
	w.mu.RUnlock()

	for i, reader := range readers {
		reader.Poll()
		_ = sessions[i] // callbacks already fired in Poll
	}
}

func (w *EventWatcher) handleFile(path string) {
	tmuxSession := filepath.Base(path)
	tmuxSession = tmuxSession[:len(tmuxSession)-len(".jsonl")]

	w.mu.Lock()
	reader, exists := w.readers[tmuxSession]
	if !exists {
		reader = NewEventReader(path)
		// Wire up callbacks
		for _, cb := range w.callbacks {
			tmux := tmuxSession // capture
			reader.OnEvent(func(e HookEvent) {
				cb(tmux, e)
			})
		}
		w.readers[tmuxSession] = reader
	}
	w.mu.Unlock()

	reader.Poll()
}

// EventsDir returns the default events directory path.
func EventsDir() string {
	tmpdir := os.Getenv("TMPDIR")
	if tmpdir == "" {
		tmpdir = "/tmp"
	}
	return filepath.Join(tmpdir, "cmux", "events")
}
