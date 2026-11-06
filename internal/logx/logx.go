package logx

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

type Entry struct {
	Time   time.Time `json:"time"`
	Level  string    `json:"level"`
	Module string    `json:"module"`
	Rule   string    `json:"rule,omitempty"`
	Msg    string    `json:"msg"`
}

type Hub struct {
	mu      sync.RWMutex
	entries []Entry
	max     int
	subs    map[chan Entry]struct{}
}

func NewHub(max int) *Hub {
	if max <= 0 {
		max = 2000
	}
	return &Hub{max: max, subs: map[chan Entry]struct{}{}}
}

func (h *Hub) Add(e Entry) {
	h.mu.Lock()
	h.entries = append(h.entries, e)
	if len(h.entries) > h.max {
		h.entries = append([]Entry(nil), h.entries[len(h.entries)-h.max:]...)
	}
	subs := make([]chan Entry, 0, len(h.subs))
	for ch := range h.subs {
		subs = append(subs, ch)
	}
	h.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- e:
		default:
		}
	}
}

func (h *Hub) List(module, rule string, limit int) []Entry {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]Entry, 0, limit)
	for i := len(h.entries) - 1; i >= 0; i-- {
		e := h.entries[i]
		if module != "" && e.Module != module {
			continue
		}
		if rule != "" && e.Rule != rule {
			continue
		}
		out = append(out, e)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	// reverse to chronological order
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func (h *Hub) Subscribe() (<-chan Entry, func()) {
	ch := make(chan Entry, 128)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.subs, ch)
		h.mu.Unlock()
	}
}

type Logger struct {
	hub    *Hub
	module string
	rule   string
}

func New(hub *Hub, module, rule string) *Logger {
	return &Logger{hub: hub, module: module, rule: rule}
}

func (l *Logger) WithRule(rule string) *Logger {
	return &Logger{hub: l.hub, module: l.module, rule: rule}
}

func (l *Logger) log(level, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	e := Entry{Time: time.Now(), Level: level, Module: l.module, Rule: l.rule, Msg: msg}
	if l.hub != nil {
		l.hub.Add(e)
	}
	prefix := "[" + strings.ToUpper(level) + "] [" + l.module + "]"
	if l.rule != "" {
		prefix += " [" + l.rule + "]"
	}
	log.Printf("%s %s", prefix, msg)
}

func (l *Logger) Debug(format string, args ...any) { l.log("debug", format, args...) }
func (l *Logger) Info(format string, args ...any)  { l.log("info", format, args...) }
func (l *Logger) Warn(format string, args ...any)  { l.log("warn", format, args...) }
func (l *Logger) Error(format string, args ...any) { l.log("error", format, args...) }
