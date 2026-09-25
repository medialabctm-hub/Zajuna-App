package scheduler

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/zajuna-app/core/internal/jobs"
)

type memoryStore struct {
	schedules []Schedule
	lastJobID string
	nextRunAt time.Time
}

func (m *memoryStore) CreateSchedule(_ context.Context, schedule Schedule) error {
	m.schedules = append(m.schedules, schedule)
	return nil
}
func (m *memoryStore) ListSchedules(_ context.Context) ([]Schedule, error) { return m.schedules, nil }
func (m *memoryStore) ListDueSchedules(_ context.Context, now time.Time) ([]Schedule, error) {
	result := make([]Schedule, 0)
	for _, schedule := range m.schedules {
		if schedule.Enabled && !schedule.NextRunAt.After(now) {
			result = append(result, schedule)
		}
	}
	return result, nil
}
func (m *memoryStore) MarkScheduleRun(_ context.Context, _ string, jobID string, _, next time.Time) error {
	m.lastJobID, m.nextRunAt = jobID, next
	return nil
}
func (m *memoryStore) SetScheduleEnabled(_ context.Context, _ string, _ bool) error { return nil }

type fakeSubmitter struct{ input any }

func (f *fakeSubmitter) Submit(_ context.Context, _ string, input any) (jobs.Job, error) {
	f.input = input
	return jobs.Job{ID: "job-scheduled"}, nil
}

func TestRunDueSubmitsAndAdvancesSchedule(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	store := &memoryStore{schedules: []Schedule{{ID: "schedule-1", WorkerType: "sync-fichas", Input: json.RawMessage(`{"username":"u"}`), Interval: time.Hour, Enabled: true, NextRunAt: now.Add(-time.Minute)}}}
	submitter := &fakeSubmitter{}
	runner, err := New(store, submitter, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	count, err := runner.RunDue(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || store.lastJobID != "job-scheduled" {
		t.Fatalf("expected one scheduled job, got count=%d id=%q", count, store.lastJobID)
	}
	expectedNextRun := now.Add(-time.Minute).Add(time.Hour)
	if !store.nextRunAt.Equal(expectedNextRun) {
		t.Fatalf("expected next run at %s, got %s", expectedNextRun, store.nextRunAt)
	}
	input, ok := submitter.input.(map[string]any)
	if !ok || input["username"] != "u" {
		t.Fatalf("unexpected submitted input: %#v", submitter.input)
	}
}
