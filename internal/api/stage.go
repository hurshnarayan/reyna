package api

import (
	"encoding/json"
	"net/http"
	"sync"
)

// stageWriter reports what a slow request is doing, while it does it.
//
// Answering a question takes as long as it takes to read a document that
// nobody has opened yet, which is a model call over a phone connection and can
// be half a minute. The app showed one fixed line for all of it, so a question
// that was progressing normally and a question that had hung looked exactly
// the same, and the only thing the user could do about either was wait.
//
// The transport is newline-delimited JSON, chosen because it needs nothing on
// either side that is not already here: the server writes a line and flushes,
// the client reads lines until the stream ends. Server-sent events would want
// an event-source client, and a side channel the app polls would want request
// ids and a place to keep them.
//
// A client that does not ask for progress gets exactly the single JSON object
// it always got, so the web app and any existing build are unaffected.
type stageWriter struct {
	w         http.ResponseWriter
	flusher   http.Flusher
	streaming bool

	mu     sync.Mutex
	closed bool
}

// stageEvent is one progress line. Kind separates it from the final answer,
// which is the only object on the stream without one.
type stageEvent struct {
	Kind   string `json:"kind"`
	Stage  string `json:"stage"`
	Detail string `json:"detail,omitempty"`
}

const ndjsonContentType = "application/x-ndjson"

// newStageWriter decides whether this request wants progress, from its Accept
// header, and sets the response up accordingly.
func newStageWriter(w http.ResponseWriter, r *http.Request) *stageWriter {
	s := &stageWriter{w: w}
	if r.Header.Get("Accept") != ndjsonContentType {
		w.Header().Set("Content-Type", "application/json")
		return s
	}
	f, ok := w.(http.Flusher)
	if !ok {
		// Nothing can be pushed before the handler returns, so streaming would
		// arrive all at once at the end, which is worse than not claiming to
		// stream at all.
		w.Header().Set("Content-Type", "application/json")
		return s
	}
	w.Header().Set("Content-Type", ndjsonContentType)
	// Without this a proxy may hold the lines back and deliver them together
	// at the end, which is the exact failure the stream exists to avoid.
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Cache-Control", "no-cache")
	s.flusher, s.streaming = f, true
	return s
}

// Send reports a stage. Detail is written for a person to read, because it is
// shown to one: "Reading Module 1 ODE.pdf" rather than a status code.
//
// Does nothing when the client did not ask for progress, so callers can report
// freely without checking first.
func (s *stageWriter) Send(name, detail string) {
	if s == nil || !s.streaming {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	line, err := json.Marshal(stageEvent{Kind: "stage", Stage: name, Detail: detail})
	if err != nil {
		return
	}
	if _, err := s.w.Write(append(line, '\n')); err != nil {
		// The client hung up, which is normal: they cancelled the question.
		s.closed = true
		return
	}
	s.flusher.Flush()
}

// Final writes the answer and ends the stream.
//
// It is the last line either way, so a client reading newline-delimited JSON
// and a client reading one object see the same bytes for the part that
// carries the answer.
func (s *stageWriter) Final(v interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	if err := json.NewEncoder(s.w).Encode(v); err != nil {
		return
	}
	if s.streaming {
		s.flusher.Flush()
	}
}

// Close marks the stream finished. Safe to call after Final.
func (s *stageWriter) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
}
