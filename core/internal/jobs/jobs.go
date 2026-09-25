package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"
)

type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusWaiting   Status = "waiting_user"
	StatusRetrying  Status = "retrying"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

type Job struct {
	ID           string
	Type         string
	Status       Status
	Input        json.RawMessage
	Result       json.RawMessage
	Progress     int
	Stage        string
	Message      string
	Attempt      int
	MaxAttempts  int
	ErrorCode    string
	ErrorMessage string
	CreatedAt    time.Time
	StartedAt    *time.Time
	FinishedAt   *time.Time
	UpdatedAt    time.Time
}

type Event struct {
	JobID     string
	Kind      string
	Stage     string
	Progress  int
	Message   string
	Data      json.RawMessage
	CreatedAt time.Time
}

type Result struct {
	Output       any
	Retryable    bool
	ErrorCode    string
	ErrorMessage string
}

type Reporter interface {
	Progress(ctx context.Context, stage string, percent int, message string) error
	Event(ctx context.Context, kind string, message string, data any) error
}

type Worker interface {
	ID() string
	Execute(ctx context.Context, job Job, reporter Reporter) Result
}

type Store interface {
	CreateJob(ctx context.Context, job Job) error
	GetJob(ctx context.Context, id string) (Job, error)
	MarkRunning(ctx context.Context, id string) (Job, error)
	UpdateProgress(ctx context.Context, id string, stage string, progress int, message string) error
	AppendEvent(ctx context.Context, event Event) error
	ListJobEvents(ctx context.Context, id string) ([]Event, error)
	CompleteJob(ctx context.Context, id string, output json.RawMessage) error
	RetryJob(ctx context.Context, id string, code string, message string) error
	FailJob(ctx context.Context, id string, code string, message string) error
	MarkCancelled(ctx context.Context, id string) error
	ReconcileInterrupted(ctx context.Context) ([]Job, error)
}

type Runtime struct {
	store       Store
	workers     map[string]Worker
	queue       chan string
	concurrency int
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	mu          sync.RWMutex
	cancels     map[string]context.CancelFunc
	inFlight    map[string]struct{}
}

func queueBufferSize(concurrency int) int {
	size := concurrency * 4
	if size < 256 {
		return 256
	}
	return size
}

func NewRuntime(store Store, concurrency int) (*Runtime, error) {
	if store == nil {
		return nil, errors.New("job store is required")
	}
	if concurrency < 1 {
		concurrency = 1
	}
	return &Runtime{
		store:       store,
		workers:     map[string]Worker{},
		queue:       make(chan string, queueBufferSize(concurrency)),
		concurrency: concurrency,
		cancels:     map[string]context.CancelFunc{},
		inFlight:    map[string]struct{}{},
	}, nil
}

func (r *Runtime) Concurrency() int { return r.concurrency }

func (r *Runtime) Register(worker Worker) error {
	if worker == nil || worker.ID() == "" {
		return errors.New("worker and worker ID are required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.workers[worker.ID()]; exists {
		return fmt.Errorf("worker %q is already registered", worker.ID())
	}
	r.workers[worker.ID()] = worker
	return nil
}

func (r *Runtime) Start(parent context.Context) {
	if parent == nil {
		parent = context.Background()
	}
	r.ctx, r.cancel = context.WithCancel(parent)
	for i := 0; i < r.concurrency; i++ {
		r.wg.Add(1)
		go r.workerLoop()
	}
	r.recoverInterrupted()
}

func (r *Runtime) Close() {
	if r.cancel != nil {
		r.cancel()
	}
	r.wg.Wait()
}

func (r *Runtime) Submit(ctx context.Context, workerID string, input any) (Job, error) {
	if r.ctx == nil {
		return Job{}, errors.New("job runtime is not running")
	}
	r.mu.RLock()
	_, registered := r.workers[workerID]
	r.mu.RUnlock()
	if !registered {
		return Job{}, fmt.Errorf("worker %q is not registered", workerID)
	}

	contents, err := json.Marshal(input)
	if err != nil {
		return Job{}, fmt.Errorf("marshal job input: %w", err)
	}
	now := time.Now().UTC()
	job := Job{
		ID:          newID(),
		Type:        workerID,
		Status:      StatusQueued,
		Input:       contents,
		Progress:    0,
		MaxAttempts: 3,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := r.store.CreateJob(ctx, job); err != nil {
		return Job{}, err
	}
	if err := ctx.Err(); err != nil {
		// Never ran: cancelled, not failed (a failure raises a "needs
		// attention" notification for a job the person abandoned).
		if markErr := r.store.MarkCancelled(context.WithoutCancel(ctx), job.ID); markErr != nil && !errors.Is(markErr, ErrInvalidTransition) {
			return Job{}, fmt.Errorf("%w (el job %s no pudo marcarse cancelado: %v)", err, job.ID, markErr)
		}
		return Job{}, err
	}

	select {
	case r.queue <- job.ID:
		return job, nil
	case <-ctx.Done():
		// The row is already persisted: without this it would stay queued and
		// never run until the next restart resumed it behind the caller's back.
		if err := r.store.MarkCancelled(context.WithoutCancel(ctx), job.ID); err != nil && !errors.Is(err, ErrInvalidTransition) {
			return Job{}, fmt.Errorf("%w (el job %s no pudo marcarse cancelado: %v)", ctx.Err(), job.ID, err)
		}
		return Job{}, ctx.Err()
	case <-r.runtimeDone():
		// Left queued on purpose: ReconcileInterrupted resumes it on restart.
		return Job{}, errors.New("job runtime is not running")
	}
}

func (r *Runtime) workerLoop() {
	defer r.wg.Done()
	for {
		select {
		case <-r.runtimeDone():
			return
		case id := <-r.queue:
			r.execute(id)
		}
	}
}

func (r *Runtime) Get(ctx context.Context, id string) (Job, error) {
	return r.store.GetJob(ctx, id)
}

func (r *Runtime) Events(ctx context.Context, id string) ([]Event, error) {
	return r.store.ListJobEvents(ctx, id)
}

// ErrJobFinished is returned by Cancel when the job already reached a
// terminal state (or does not exist); nothing was changed.
var ErrJobFinished = errors.New("el trabajo ya terminó o no existe; no se puede cancelar")

// Cancel persists the cancellation first and only then stops the worker, so
// a job is never left "running" with a dead worker, and a job that already
// finished is reported as such instead of being interrupted.
func (r *Runtime) Cancel(ctx context.Context, id string) error {
	if err := r.store.MarkCancelled(ctx, id); err != nil {
		if errors.Is(err, ErrInvalidTransition) {
			return ErrJobFinished
		}
		return err
	}
	r.mu.RLock()
	cancel := r.cancels[id]
	r.mu.RUnlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

// PartialResultStore is optional: stores that implement it keep the
// structured output of failed or cancelled jobs (what was already done).
type PartialResultStore interface {
	SavePartialResult(ctx context.Context, id string, output json.RawMessage) error
}

func (r *Runtime) execute(id string) {
	r.mu.Lock()
	if _, busy := r.inFlight[id]; busy {
		r.mu.Unlock()
		return
	}
	r.inFlight[id] = struct{}{}
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		delete(r.inFlight, id)
		r.mu.Unlock()
	}()

	job, err := r.store.GetJob(r.ctx, id)
	if err != nil {
		return
	}
	if job.Status == StatusCancelled || job.Status == StatusCompleted || job.Status == StatusFailed {
		return
	}
	worker := r.worker(job.Type)
	if worker == nil {
		_ = r.store.FailJob(r.ctx, id, "worker_not_found", "worker no registrado")
		return
	}

	// Registered before MarkRunning so a Cancel racing with the start always
	// finds either a non-runnable row or a cancel func to call.
	workerCtx, cancel := context.WithCancel(r.ctx)
	r.mu.Lock()
	r.cancels[id] = cancel
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		delete(r.cancels, id)
		r.mu.Unlock()
		cancel()
	}()
	job, err = r.store.MarkRunning(r.ctx, id)
	if err != nil {
		return
	}

	reporter := &jobReporter{store: r.store, jobID: job.ID}
	result := executeWorker(workerCtx, worker, job, reporter)
	if workerCtx.Err() != nil {
		if r.ctx.Err() != nil {
			// Core shutdown: the row stays running and ReconcileInterrupted
			// retries it on the next start.
			return
		}
		// Cancelled by the user. Cancel already persisted the state; this is
		// a no-op except if the worker context was cancelled some other way.
		if err := r.store.MarkCancelled(r.ctx, id); err != nil && !errors.Is(err, ErrInvalidTransition) {
			return
		}
		r.savePartial(id, result.Output)
		return
	}
	if result.ErrorMessage != "" {
		message := SanitizeMessage(result.ErrorMessage)
		if result.Retryable && job.Attempt < job.MaxAttempts {
			if err := r.store.RetryJob(r.ctx, id, result.ErrorCode, message); err != nil {
				if errors.Is(err, ErrInvalidTransition) {
					return
				}
				// A transient store error (e.g. SQLite lock contention) must not
				// schedule an in-memory retry as if the attempt counter had been
				// persisted: that would let a job retry more times than
				// MaxAttempts allows and leave its persisted state inconsistent
				// with what actually ran.
				_ = r.store.FailJob(r.ctx, id, "retry_persist_failed", SanitizeMessage(err.Error()))
				return
			}
			r.enqueueRetry(id, time.Duration(job.Attempt)*500*time.Millisecond)
			return
		}
		if err := r.store.FailJob(r.ctx, id, result.ErrorCode, message); err == nil {
			r.savePartial(id, result.Output)
		}
		return
	}

	output, err := encodeOutput(result.Output)
	if err != nil {
		_ = r.store.FailJob(r.ctx, id, "result_encode_failed", SanitizeMessage(err.Error()))
		return
	}
	if err := r.store.CompleteJob(r.ctx, id, output); err != nil && !errors.Is(err, ErrInvalidTransition) {
		_ = r.store.FailJob(r.ctx, id, "result_persist_failed", SanitizeMessage(err.Error()))
	}
}

func (r *Runtime) savePartial(id string, value any) {
	if value == nil {
		return
	}
	partialStore, ok := r.store.(PartialResultStore)
	if !ok {
		return
	}
	output, err := encodeOutput(value)
	if err != nil {
		return
	}
	_ = partialStore.SavePartialResult(context.WithoutCancel(r.ctx), id, output)
}

func (r *Runtime) recoverInterrupted() {
	jobsToResume, err := r.store.ReconcileInterrupted(r.ctx)
	if err != nil {
		return
	}
	for _, job := range jobsToResume {
		select {
		case r.queue <- job.ID:
		case <-r.runtimeDone():
			return
		}
	}
}

func (r *Runtime) enqueueRetry(id string, delay time.Duration) {
	time.AfterFunc(delay, func() {
		select {
		case r.queue <- id:
		case <-r.runtimeDone():
		}
	})
}

func (r *Runtime) worker(id string) Worker {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.workers[id]
}

func (r *Runtime) runtimeDone() <-chan struct{} {
	if r.ctx == nil {
		return neverDone()
	}
	return r.ctx.Done()
}

func newID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("job-%d", time.Now().UnixNano())
	}
	return "job-" + hex.EncodeToString(bytes)
}

type jobReporter struct {
	store Store
	jobID string
}

func (r *jobReporter) Progress(ctx context.Context, stage string, percent int, message string) error {
	return r.store.UpdateProgress(ctx, r.jobID, stage, percent, SanitizeMessage(message))
}

func (r *jobReporter) Event(ctx context.Context, kind string, message string, data any) error {
	contents, err := encodeOutput(data)
	if err != nil {
		return err
	}
	return r.store.AppendEvent(ctx, Event{
		JobID:     r.jobID,
		Kind:      kind,
		Message:   SanitizeMessage(message),
		Data:      contents,
		CreatedAt: time.Now().UTC(),
	})
}

func executeWorker(ctx context.Context, worker Worker, job Job, reporter Reporter) (result Result) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = Result{ErrorCode: "worker_panic", ErrorMessage: "el proceso falló de forma inesperada"}
			log.Printf("worker %s panic en job %s: %v", worker.ID(), job.ID, recovered)
		}
	}()
	return worker.Execute(ctx, job, reporter)
}

func neverDone() <-chan struct{} {
	return make(chan struct{})
}
