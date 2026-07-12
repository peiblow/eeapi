package trigger

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/peiblow/eeapi/internal/database/redis"
	"github.com/peiblow/eeapi/internal/schema"
	"github.com/peiblow/eeapi/internal/service"
)

type FSProvider struct {
	mark     *watermark
	watchers []*fsnotify.Watcher
	done     chan struct{}
}

func NewFSProvider(rdb *redis.Client) *FSProvider {
	return &FSProvider{
		mark: &watermark{rdb: rdb},
		done: make(chan struct{}),
	}
}

func (f *FSProvider) Type() string { return "filesystem" }

func (f *FSProvider) Start(events service.EventService, bindings []Binding) error {
	started := 0
	for _, b := range bindings {
		if err := f.watch(events, b); err != nil {
			slog.Error("skipping filesystem trigger", "agent", b.Agent.Hash, "error", err)
			continue
		}
		started++
	}
	slog.Info("filesystem provider started", "watchers", started)
	return nil
}

func (f *FSProvider) Stop() {
	select {
	case <-f.done:
	default:
		close(f.done)
	}
	for _, w := range f.watchers {
		w.Close()
	}
}

func (f *FSProvider) watch(events service.EventService, b Binding) error {
	path := resolveEnvTemplate(strConfig(b.Config, "path"))
	if path == "" {
		return fmt.Errorf("filesystem trigger has no path")
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("path %q: %w", path, err)
	}

	recursive := boolConfig(b.Config, "recursive")

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	if err := addWatchPaths(w, path, recursive); err != nil {
		w.Close()
		return err
	}
	f.watchers = append(f.watchers, w)

	spec := fsSpec{
		agent:     b.Agent,
		skill:     strConfig(b.Config, "skill"),
		ops:       toStringSet(b.Config["ops"]),
		pattern:   strConfig(b.Config, "pattern"),
		recursive: recursive,
		debounce:  parseDuration(strConfig(b.Config, "debounce")),
	}

	go f.loop(w, events, spec)
	slog.Info("filesystem trigger watching", "agent", b.Agent.Hash, "path", path, "recursive", recursive, "pattern", spec.pattern)
	return nil
}

type fsSpec struct {
	agent     schema.AgentDefinition
	skill     string
	ops       map[string]bool
	pattern   string
	recursive bool
	debounce  time.Duration
}

func (f *FSProvider) loop(w *fsnotify.Watcher, events service.EventService, spec fsSpec) {
	timers := make(map[string]*time.Timer)
	var mu sync.Mutex

	for {
		select {
		case <-f.done:
			return
		case ev, ok := <-w.Events:
			if !ok {
				return
			}

			if spec.recursive && ev.Op&fsnotify.Create != 0 && isDir(ev.Name) {
				w.Add(ev.Name)
				continue
			}

			op := fsOpName(ev.Op)
			if len(spec.ops) > 0 && !spec.ops[op] {
				continue
			}
			if spec.pattern != "" {
				if matched, _ := filepath.Match(spec.pattern, filepath.Base(ev.Name)); !matched {
					continue
				}
			}

			name := ev.Name
			emit := func() {
				mu.Lock()
				delete(timers, name)
				mu.Unlock()
				f.emit(events, spec, name, op)
			}

			if spec.debounce <= 0 {
				emit()
				continue
			}
			mu.Lock()
			if t, ok := timers[name]; ok {
				t.Stop()
			}
			timers[name] = time.AfterFunc(spec.debounce, emit)
			mu.Unlock()
		case err, ok := <-w.Errors:
			if !ok {
				return
			}
			slog.Error("fs watcher error", "agent", spec.agent.Hash, "error", err)
		}
	}
}

func (f *FSProvider) emit(events service.EventService, spec fsSpec, path, op string) {
	content, _ := os.ReadFile(path)
	sum := sha256.Sum256(content)
	key := spec.agent.Hash + ":" + path + ":" + hex.EncodeToString(sum[:])
	if !f.mark.firstSeen(context.Background(), key, 30*time.Second) {
		slog.Info("fs event deduped", "agent", spec.agent.Hash, "path", path)
		return
	}

	text := ""
	if spec.skill != "" {
		if t, err := resolveSkillText(spec.agent, spec.skill); err != nil {
			slog.Error("fs trigger skill resolve failed", "agent", spec.agent.Hash, "error", err)
		} else {
			text = t
		}
	}
	if text == "" {
		text = fmt.Sprintf("Filesystem %s: %s", op, path)
	}

	payload, err := json.Marshal(map[string]any{
		"text": text,
		"raw":  map[string]string{"path": path, "op": op},
	})
	if err != nil {
		slog.Error("fs trigger marshal failed", "agent", spec.agent.Hash, "error", err)
		return
	}

	if _, err := events.EnqueueAgentEvent(context.Background(), spec.agent.Hash, &service.EnqueueEventInput{
		Source:  "filesystem:" + op,
		Payload: payload,
	}); err != nil {
		slog.Error("fs trigger enqueue failed", "agent", spec.agent.Hash, "path", path, "error", err)
		return
	}
	slog.Info("fs trigger fired", "agent", spec.agent.Hash, "op", op, "path", path)
}

func resolveEnvTemplate(s string) string {
	if strings.HasPrefix(s, "getEnv(") && strings.HasSuffix(s, ")") {
		return os.Getenv(s[len("getEnv(") : len(s)-1])
	}
	return s
}

func strConfig(config map[string]any, key string) string {
	s, _ := config[key].(string)
	return s
}

func boolConfig(config map[string]any, key string) bool {
	b, _ := config[key].(bool)
	return b
}

func toStringSet(v any) map[string]bool {
	out := make(map[string]bool)
	if arr, ok := v.([]any); ok {
		for _, e := range arr {
			if s, ok := e.(string); ok {
				out[s] = true
			}
		}
	}
	return out
}

func parseDuration(s string) time.Duration {
	if s == "" {
		return 0
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0
	}
	return d
}

func fsOpName(op fsnotify.Op) string {
	switch {
	case op&fsnotify.Create != 0:
		return "create"
	case op&fsnotify.Write != 0:
		return "write"
	case op&fsnotify.Remove != 0:
		return "remove"
	case op&fsnotify.Rename != 0:
		return "rename"
	case op&fsnotify.Chmod != 0:
		return "chmod"
	}
	return "unknown"
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func addWatchPaths(w *fsnotify.Watcher, root string, recursive bool) error {
	if !recursive {
		return w.Add(root)
	}
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return w.Add(p)
		}
		return nil
	})
}
