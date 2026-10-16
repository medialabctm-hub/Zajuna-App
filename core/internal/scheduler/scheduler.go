package scheduler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/zajuna-app/core/internal/jobs"
)

type Schedule struct {
	ID         string
	WorkerType string
	Input      json.RawMessage
	Interval   time.Duration
	Enabled    bool
	NextRunAt  time.Time
	LastRunAt  *time.Time
	LastJobID  string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type Store interface {
	CreateSchedule(ctx context.Context, schedule Schedule) error
	ListSchedules(ctx context.Context) ([]Schedule, error)
	ListDueSchedules(ctx context.Context, now time.Time) ([]Schedule, error)
	MarkScheduleRun(ctx context.Context, id, jobID string, lastRunAt, nextRunAt time.Time) error
	SetScheduleEnabled(ctx context.Context, id string, enabled bool) error
}

type Submitter interface {
	Submit(ctx context.Context, workerID string, input any) (jobs.Job, error)
}

type Scheduler struct {
	store     Store
	submitter Submitter
	pollEvery time.Duration
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	startOnce sync.Once
	closeOnce sync.Once
}

func New(store Store, submitter Submitter, pollEvery time.Duration) (*Scheduler, error) {
	if store == nil {
		return nil, errors.New("scheduler store is required")
	}
	if submitter == nil {
		return nil, errors.New("scheduler submitter is required")
	}
	if pollEvery <= 0 {
		pollEvery = 30 * time.Second
	}
	return &Scheduler{store: store, submitter: submitter, pollEvery: pollEvery}, nil
}

func NewID() string {
	bytes := make([]byte, 12)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("schedule-%d", time.Now().UnixNano())
	}
	return "schedule-" + hex.EncodeToString(bytes)
}

func (s *Scheduler) Start(parent context.Context) {
	s.startOnce.Do(func() {
		if parent == nil {
			parent = context.Background()
		}
		s.ctx, s.cancel = context.WithCancel(parent)
		s.wg.Add(1)
		go s.loop()
	})
}

func (s *Scheduler) Close() {
	s.closeOnce.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		s.wg.Wait()
	})
}

func (s *Scheduler) loop() {
	defer s.wg.Done()
	ticker := time.NewTicker(s.pollEvery)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case now := <-ticker.C:
			if _, err := s.RunDue(s.ctx, now.UTC()); err != nil {
				// A failed schedule is left due so the next tick can retry it.
				// The worker itself owns retryable job failures.
			}
		}
	}
}

func (s *Scheduler) RunDue(ctx context.Context, now time.Time) (int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	due, err := s.store.ListDueSchedules(ctx, now)
	if err != nil {
		return 0, err
	}
	runCount := 0
	var firstErr error
	for _, schedule := range due {
		if schedule.Interval <= 0 {
			if firstErr == nil {
				firstErr = fmt.Errorf("schedule %s has an invalid interval", schedule.ID)
			}
			continue
		}
		input := any(map[string]any{})
		if len(schedule.Input) > 0 {
			if err := json.Unmarshal(schedule.Input, &input); err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("schedule %s has invalid input: %w", schedule.ID, err)
				}
				continue
			}
		}
		job, err := s.submitter.Submit(ctx, schedule.WorkerType, input)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("submit schedule %s: %w", schedule.ID, err)
			}
			continue
		}
		nextRunAt := schedule.NextRunAt
		if nextRunAt.IsZero() {
			nextRunAt = now
		}
		// A nextRunAt far in the past (year 0001 with a 1 s interval) would
		// spin this loop for billions of steps and block the scheduler.
		if now.Sub(nextRunAt)/schedule.Interval > 1000 {
			nextRunAt = now
		}
		for !nextRunAt.After(now) {
			nextRunAt = nextRunAt.Add(schedule.Interval)
		}
		if err := s.store.MarkScheduleRun(ctx, schedule.ID, job.ID, now, nextRunAt); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("mark schedule %s: %w", schedule.ID, err)
			}
			continue
		}
		runCount++
	}
	return runCount, firstErr
}
