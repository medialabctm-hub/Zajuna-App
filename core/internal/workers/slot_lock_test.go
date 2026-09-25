package workers

import (
	"context"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zajuna-app/core/internal/capture"
	"github.com/zajuna-app/core/internal/checklist"
)

func TestKeyedLocksSerializeTheSameKeyOnly(t *testing.T) {
	locks := &keyedLocks{slots: make(map[string]*keyedLock)}
	var inFlight, maxInFlight int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock, err := locks.lockAll(context.Background(), []string{"ficha|1.1.1|1"})
			if err != nil {
				t.Error(err)
				return
			}
			current := atomic.AddInt32(&inFlight, 1)
			for {
				seen := atomic.LoadInt32(&maxInFlight)
				if current <= seen || atomic.CompareAndSwapInt32(&maxInFlight, seen, current) {
					break
				}
			}
			time.Sleep(2 * time.Millisecond)
			atomic.AddInt32(&inFlight, -1)
			unlock()
		}()
	}
	wg.Wait()
	if maxInFlight != 1 {
		t.Fatalf("the same slot was held by %d captures at once", maxInFlight)
	}
	if len(locks.slots) != 0 {
		t.Fatalf("released slots must not stay in the map: %d", len(locks.slots))
	}

	first, _ := locks.lockAll(context.Background(), []string{"ficha|1.1.1|1"})
	done := make(chan struct{})
	go func() {
		other, err := locks.lockAll(context.Background(), []string{"ficha|1.1.1|2"})
		if err == nil {
			other()
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("a different slot must not wait for another slot's lock")
	}
	first()
}

func TestKeyedLocksHonourCancellationAndOverlappingSets(t *testing.T) {
	locks := &keyedLocks{slots: make(map[string]*keyedLock)}
	held, _ := locks.lockAll(context.Background(), []string{"b"})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := locks.lockAll(ctx, []string{"a", "b"}); err == nil {
		t.Fatal("a waiter must give up when its context ends")
	}
	held()
	if len(locks.slots) != 0 {
		t.Fatalf("a cancelled waiter must not leave its keys held: %v", locks.slots)
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		keys := []string{"a", "b"}
		if i%2 == 1 {
			keys = []string{"b", "a"}
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock, err := locks.lockAll(context.Background(), keys)
			if err == nil {
				unlock()
				unlock()
			}
		}()
	}
	finished := make(chan struct{})
	go func() { wg.Wait(); close(finished) }()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("overlapping key sets deadlocked")
	}
}

type recordingSession struct {
	mu          sync.Mutex
	options     []capture.CaptureOptions
	inFlight    *int32
	maxInFlight *int32
}

func (s *recordingSession) CaptureURLWithMetadataAndOptions(_ context.Context, _ string, _ string, options capture.CaptureOptions) (capture.CaptureResult, error) {
	current := atomic.AddInt32(s.inFlight, 1)
	for {
		seen := atomic.LoadInt32(s.maxInFlight)
		if current <= seen || atomic.CompareAndSwapInt32(s.maxInFlight, seen, current) {
			break
		}
	}
	time.Sleep(10 * time.Millisecond)
	atomic.AddInt32(s.inFlight, -1)
	s.mu.Lock()
	s.options = append(s.options, options)
	s.mu.Unlock()
	return capture.CaptureResult{}, capture.ErrSelectorNotFound
}

func (s *recordingSession) Close() {}

func TestChecklistTargetsOfTheSameSlotNeverCaptureConcurrently(t *testing.T) {
	var inFlight, maxInFlight int32
	sessions := make([]*recordingSession, 0)
	var sessionsMu sync.Mutex
	pool := newBrowserSessionPool(func(context.Context) (checklistBrowserSession, error) {
		session := &recordingSession{inFlight: &inFlight, maxInFlight: &maxInFlight}
		sessionsMu.Lock()
		sessions = append(sessions, session)
		sessionsMu.Unlock()
		return session, nil
	})
	worker := &CaptureChecklistWorker{dataDir: t.TempDir()}
	baseURL, _ := url.Parse("https://93.184.216.34")
	target := checklist.CaptureTarget{ItemCode: "3.1", CoveredItemCodes: []string{"3.1"}, SlotNumber: 1, URL: "https://93.184.216.34/zajuna/course/view.php?id=1", CSSSelector: "#region-main .course-content"}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Replayed payloads may carry RequireSelector=false; the worker
			// must still capture strictly.
			worker.captureChecklistTarget(context.Background(), checklistTargetParams{
				Input: CaptureChecklistInput{FichaID: "ficha-1"}, Target: target,
				BaseURL: baseURL, UseBrowser: true, Sessions: pool,
			})
		}()
	}
	wg.Wait()
	pool.closeAll()
	if maxInFlight != 1 {
		t.Fatalf("two captures wrote the same ficha slot at once (max %d)", maxInFlight)
	}
	total := 0
	for _, session := range sessions {
		for _, options := range session.options {
			total++
			if !options.RequireSelector {
				t.Fatalf("checklist capture ran without a required selector: %#v", options)
			}
		}
	}
	if total != 4 {
		t.Fatalf("expected 4 captures, got %d", total)
	}
}
