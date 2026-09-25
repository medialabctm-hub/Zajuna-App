package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func waitForStatus(t *testing.T, store *memoryStore, id string, want Status) Job {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		job, _ := store.GetJob(context.Background(), id)
		if job.Status == want {
			return job
		}
		time.Sleep(5 * time.Millisecond)
	}
	job, _ := store.GetJob(context.Background(), id)
	t.Fatalf("job %s status = %s, want %s", id, job.Status, want)
	return Job{}
}

func TestSubmitMarksPersistedJobCancelledWhenEnqueueIsAbandoned(t *testing.T) {
	store := newMemoryStore()
	runtime, err := NewRuntime(store, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Register(demoWorker{}); err != nil {
		t.Fatal(err)
	}
	// Running runtime whose queue is full: no worker loop drains it.
	runtime.ctx, runtime.cancel = context.WithCancel(context.Background())
	defer runtime.cancel()
	runtime.queue = make(chan string)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := runtime.Submit(ctx, "demo", map[string]string{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Submit error = %v", err)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.jobs) != 1 {
		t.Fatalf("expected the persisted job, got %d", len(store.jobs))
	}
	for _, job := range store.jobs {
		if job.Status != StatusCancelled {
			t.Fatalf("abandoned submit left the job %s instead of cancelled", job.Status)
		}
	}
}

type blockingWorker struct{ started chan struct{} }

func (blockingWorker) ID() string { return "blocking" }
func (w blockingWorker) Execute(ctx context.Context, _ Job, _ Reporter) Result {
	close(w.started)
	<-ctx.Done()
	return Result{ErrorCode: "cancelled", ErrorMessage: ctx.Err().Error(), Output: map[string]any{"captured": 2, "partial": true}}
}

func TestCancelRunningJobPersistsCancelledStateAndPartialOutput(t *testing.T) {
	store := newMemoryStore()
	runtime, _ := NewRuntime(store, 1)
	worker := blockingWorker{started: make(chan struct{})}
	if err := runtime.Register(worker); err != nil {
		t.Fatal(err)
	}
	runtime.Start(context.Background())
	defer runtime.Close()
	job, err := runtime.Submit(context.Background(), "blocking", map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-worker.started:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not start")
	}
	if err := runtime.Cancel(context.Background(), job.ID); err != nil {
		t.Fatalf("Cancel = %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		stored, _ := store.GetJob(context.Background(), job.ID)
		if stored.Status == StatusCancelled && len(stored.Result) > 0 {
			var output map[string]any
			if err := json.Unmarshal(stored.Result, &output); err != nil || output["captured"] != float64(2) {
				t.Fatalf("partial output = %s (%v)", stored.Result, err)
			}
			if err := runtime.Cancel(context.Background(), job.ID); !errors.Is(err, ErrJobFinished) {
				t.Fatalf("second cancel = %v, want ErrJobFinished", err)
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	stored, _ := store.GetJob(context.Background(), job.ID)
	t.Fatalf("cancelled job = %s result=%s", stored.Status, stored.Result)
}

func TestCancelQueuedJobNeverRuns(t *testing.T) {
	store := newMemoryStore()
	runtime, _ := NewRuntime(store, 1)
	worker := blockingWorker{started: make(chan struct{})}
	_ = runtime.Register(worker)
	_ = runtime.Register(demoWorker{})
	runtime.Start(context.Background())
	defer runtime.Close()
	blocker, _ := runtime.Submit(context.Background(), "blocking", map[string]string{})
	<-worker.started
	queued, err := runtime.Submit(context.Background(), "demo", map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Cancel(context.Background(), queued.ID); err != nil {
		t.Fatal(err)
	}
	_ = runtime.Cancel(context.Background(), blocker.ID)
	waitForStatus(t, store, blocker.ID, StatusCancelled)
	time.Sleep(50 * time.Millisecond)
	if stored, _ := store.GetJob(context.Background(), queued.ID); stored.Status != StatusCancelled || stored.StartedAt != nil {
		t.Fatalf("cancelled queued job ran: %s started=%v", stored.Status, stored.StartedAt)
	}
}

type partialFailureWorker struct{}

func (partialFailureWorker) ID() string { return "partial-failure" }
func (partialFailureWorker) Execute(context.Context, Job, Reporter) Result {
	return Result{
		ErrorCode:    "capture_partial_failure",
		ErrorMessage: "falló https://zajuna.example/course.php?id=1&sesskey=abc123 con password=hunter2\nAuthorization: Bearer abcdefghijklmnop",
		Output:       map[string]any{"captured": 3, "failed": 1, "failures": []string{"1.1: token=qwerty rechazado"}},
	}
}

func TestFailedJobKeepsSanitizedErrorAndStructuredPartialOutput(t *testing.T) {
	store := newMemoryStore()
	runtime, _ := NewRuntime(store, 1)
	_ = runtime.Register(partialFailureWorker{})
	runtime.Start(context.Background())
	defer runtime.Close()
	job, err := runtime.Submit(context.Background(), "partial-failure", map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	stored := waitForStatus(t, store, job.ID, StatusFailed)
	deadline := time.Now().Add(time.Second)
	for len(stored.Result) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
		stored, _ = store.GetJob(context.Background(), job.ID)
	}
	for _, secret := range []string{"abc123", "hunter2", "abcdefghijklmnop"} {
		if strings.Contains(stored.ErrorMessage, secret) {
			t.Fatalf("persisted error leaks %q: %s", secret, stored.ErrorMessage)
		}
	}
	if strings.ContainsAny(stored.ErrorMessage, "\n\r") || !strings.Contains(stored.ErrorMessage, "zajuna.example/course.php?id=1") {
		t.Fatalf("error message must stay useful and single-line: %q", stored.ErrorMessage)
	}
	var output struct {
		Captured int      `json:"captured"`
		Failed   int      `json:"failed"`
		Failures []string `json:"failures"`
	}
	if err := json.Unmarshal(stored.Result, &output); err != nil || output.Captured != 3 || output.Failed != 1 {
		t.Fatalf("partial output = %s (%v)", stored.Result, err)
	}
	if strings.Contains(string(stored.Result), "qwerty") {
		t.Fatalf("partial output leaks a secret: %s", stored.Result)
	}
}

func TestSanitizeMessageBoundsLength(t *testing.T) {
	message := SanitizeMessage(strings.Repeat("á", maxPersistedMessageRunes+50))
	if got := len([]rune(message)); got != maxPersistedMessageRunes+1 {
		t.Fatalf("sanitized length = %d", got)
	}
}
