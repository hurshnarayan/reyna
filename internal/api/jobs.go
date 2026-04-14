package api

import (
	"strconv"
	"sync"
	"time"
)

// Background-job registry. Long-running user-initiated tasks like Drive
// ingest or staged-commit push run as goroutines; their progress is stored
// here in memory so the dashboard can poll status without the frontend
// needing to stay mounted on a specific page.
//
// Scoped by user + job kind, so a user can have at most one sync-drive job
// and one push job in flight at a time. Finished jobs linger for ~5 min so
// the UI can show the final counters after completion.
type JobKind string

const (
	JobKindIngestDrive JobKind = "ingest_drive"
	JobKindPushStaged  JobKind = "push_staged"
)

type JobState string

const (
	JobStatePending  JobState = "pending"
	JobStateRunning  JobState = "running"
	JobStateDone     JobState = "done"
	JobStateFailed   JobState = "failed"
)

// Job is the public, JSON-serialisable view of a background task.
type Job struct {
	ID         string    `json:"id"`
	Kind       JobKind   `json:"kind"`
	State      JobState  `json:"state"`
	Total      int       `json:"total"`
	Done       int       `json:"done"`
	Errored    int       `json:"errored"`
	Skipped    int       `json:"skipped"`
	Message    string    `json:"message"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
}

type jobRegistry struct {
	mu   sync.Mutex
	jobs map[string]*Job // key: "userID:kind"
}

func newJobRegistry() *jobRegistry {
	return &jobRegistry{jobs: map[string]*Job{}}
}

func jobKey(userID int64, kind JobKind) string {
	return strconv.FormatInt(userID, 10) + ":" + string(kind)
}

// start registers a new job for (userID, kind). If a job of the same kind
// is already running for this user, start returns the existing job and
// false — the handler should not kick off a duplicate goroutine.
func (r *jobRegistry) start(userID int64, kind JobKind) (*Job, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := jobKey(userID, kind)
	if j, ok := r.jobs[key]; ok && j.State == JobStateRunning {
		return j, false
	}
	j := &Job{
		ID: key, Kind: kind,
		State: JobStateRunning, StartedAt: time.Now(),
	}
	r.jobs[key] = j
	return j, true
}

// get returns the current job state (may be nil if no such job has run).
func (r *jobRegistry) get(userID int64, kind JobKind) *Job {
	r.mu.Lock()
	defer r.mu.Unlock()
	if j, ok := r.jobs[jobKey(userID, kind)]; ok {
		// GC: if it finished >10 min ago, drop it.
		if !j.FinishedAt.IsZero() && time.Since(j.FinishedAt) > 10*time.Minute {
			delete(r.jobs, jobKey(userID, kind))
			return nil
		}
		// Return a copy so callers can't race on our fields.
		cp := *j
		return &cp
	}
	return nil
}

// update mutates a running job's counters atomically. Safe to call from
// the worker goroutine at any granularity.
func (r *jobRegistry) update(userID int64, kind JobKind, mut func(*Job)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if j, ok := r.jobs[jobKey(userID, kind)]; ok {
		mut(j)
	}
}

// finish marks a job done (or failed) and timestamps its completion.
func (r *jobRegistry) finish(userID int64, kind JobKind, state JobState, msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if j, ok := r.jobs[jobKey(userID, kind)]; ok {
		j.State = state
		j.Message = msg
		j.FinishedAt = time.Now()
	}
}
