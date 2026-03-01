package synctest_test

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"
)

type detslog struct {
	mu      sync.Mutex
	entries []detslogEntry
}

type detslogEntry struct {
	level slog.Level
	msg   string
	attrs []slog.Attr
}

func newDetslogLogger() (*slog.Logger, *detslog) {
	h := &detslog{entries: make([]detslogEntry, 0, 128)}
	return slog.New(h), h
}

func (h *detslog) Enabled(_ context.Context, _ slog.Level) bool {
	return true
}

func (h *detslog) Handle(_ context.Context, r slog.Record) error {
	entry := detslogEntry{
		level: r.Level,
		msg:   r.Message,
		attrs: make([]slog.Attr, 0, r.NumAttrs()),
	}
	r.Attrs(func(a slog.Attr) bool {
		entry.attrs = append(entry.attrs, a)
		return true
	})

	h.mu.Lock()
	h.entries = append(h.entries, entry)
	h.mu.Unlock()
	return nil
}

func (h *detslog) WithAttrs(_ []slog.Attr) slog.Handler {
	return h
}

func (h *detslog) WithGroup(_ string) slog.Handler {
	return h
}

func (h *detslog) Dump() string {
	h.mu.Lock()
	entries := append([]detslogEntry(nil), h.entries...)
	h.mu.Unlock()

	lines := make([]string, 0, len(entries))
	for i, e := range entries {
		attrs := append([]slog.Attr(nil), e.attrs...)
		sort.Slice(attrs, func(i, j int) bool {
			return attrs[i].Key < attrs[j].Key
		})
		var b strings.Builder
		// Use entry index as a stable sequence number in output.
		fmt.Fprintf(&b, "[%03d] level=%s msg=%q", i, e.level.String(), e.msg)
		for _, a := range attrs {
			fmt.Fprintf(&b, " %s=%v", a.Key, attrValue(a))
		}
		lines = append(lines, b.String())
	}
	return strings.Join(lines, "\n")
}

func attrValue(a slog.Attr) any {
	if a.Value.Kind() == slog.KindTime {
		t := a.Value.Time()
		if t.IsZero() {
			return "0001-01-01T00:00:00Z"
		}
		return t.UTC().Format(time.RFC3339Nano)
	}
	return a.Value.Any()
}
