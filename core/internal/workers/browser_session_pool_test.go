package workers

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/zajuna-app/core/internal/capture"
	"github.com/zajuna-app/core/internal/checklist"
)

type fakePoolSession struct{ closed bool }

func (f *fakePoolSession) CaptureURLWithMetadataAndOptions(context.Context, string, string, capture.CaptureOptions) (capture.CaptureResult, error) {
	return capture.CaptureResult{}, nil
}
func (f *fakePoolSession) Close() { f.closed = true }

func TestBrowserSessionPoolReusesHealthySessions(t *testing.T) {
	opened := 0
	pool := newBrowserSessionPool(func(context.Context) (checklistBrowserSession, error) {
		opened++
		return &fakePoolSession{}, nil
	})
	for i := 0; i < 10; i++ {
		session, err := pool.acquire(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		pool.release(session, true)
	}
	if opened != 1 {
		t.Fatalf("10 sequential targets must reuse one login, opened %d", opened)
	}
	pool.closeAll()
}

func TestBrowserSessionPoolReplacesUnhealthySessions(t *testing.T) {
	var sessions []*fakePoolSession
	pool := newBrowserSessionPool(func(context.Context) (checklistBrowserSession, error) {
		session := &fakePoolSession{}
		sessions = append(sessions, session)
		return session, nil
	})
	first, _ := pool.acquire(context.Background())
	pool.release(first, false)
	if !sessions[0].closed {
		t.Fatal("an unhealthy session must be closed")
	}
	second, _ := pool.acquire(context.Background())
	if second == first || len(sessions) != 2 {
		t.Fatal("a fresh session must replace the unhealthy one")
	}
	pool.closeAll()
	if !sessions[1].closed {
		t.Fatal("closeAll must close in-use and idle sessions")
	}
}

func TestBrowserSessionPoolRetriesTransientLoginFailures(t *testing.T) {
	calls := 0
	pool := newBrowserSessionPool(func(context.Context) (checklistBrowserSession, error) {
		calls++
		if calls < 3 {
			return nil, fmt.Errorf("abrir login de Zajuna en Chromium: timeout")
		}
		return &fakePoolSession{}, nil
	})
	pool.backoff = 0
	if _, err := pool.acquire(context.Background()); err != nil || calls != 3 {
		t.Fatalf("expected success on the third attempt, calls=%d err=%v", calls, err)
	}
}

func TestBrowserSessionPoolDoesNotRetryChallenges(t *testing.T) {
	calls := 0
	pool := newBrowserSessionPool(func(context.Context) (checklistBrowserSession, error) {
		calls++
		return nil, fmt.Errorf("%w: captcha", capture.ErrChallengePage)
	})
	pool.backoff = 0
	if _, err := pool.acquire(context.Background()); !errors.Is(err, capture.ErrChallengePage) || calls != 1 {
		t.Fatalf("challenges must fail immediately, calls=%d err=%v", calls, err)
	}
}

func TestReusableBrowserSession(t *testing.T) {
	if !reusableBrowserSession(nil, "https://zajuna.sena.edu.co/zajuna/course/view.php?id=1") {
		t.Fatal("a successful capture keeps the session")
	}
	if reusableBrowserSession(nil, "https://zajuna.sena.edu.co/zajuna/login/index.php") {
		t.Fatal("a login redirect means the session expired")
	}
	if !reusableBrowserSession(fmt.Errorf("%w: x", capture.ErrSelectorNotFound), "") {
		t.Fatal("selector not found is a page outcome, the session is still valid")
	}
	if !reusableBrowserSession(fmt.Errorf("%w: x", capture.ErrForumAccessDenied), "") {
		t.Fatal("a forum without access is a page outcome, the session is still valid")
	}
	if reusableBrowserSession(fmt.Errorf("%w: x", capture.ErrLoginPage), "") {
		t.Fatal("a login page error invalidates the session")
	}
}

type scriptedPoolSession struct {
	err    error
	closed bool
}

func (s *scriptedPoolSession) CaptureURLWithMetadataAndOptions(context.Context, string, string, capture.CaptureOptions) (capture.CaptureResult, error) {
	return capture.CaptureResult{}, s.err
}
func (s *scriptedPoolSession) Close() { s.closed = true }

func TestCaptureChecklistTargetAutoRenewLogsInAgainOnce(t *testing.T) {
	base, _ := url.Parse("https://zajuna.sena.edu.co")
	target := checklist.CaptureTarget{ItemCode: "1.1.1", URL: "https://zajuna.sena.edu.co/zajuna/course/view.php?id=1", SlotNumber: 1}
	for _, autoRenew := range []bool{true, false} {
		opened := 0
		pool := newBrowserSessionPool(func(context.Context) (checklistBrowserSession, error) {
			opened++
			if opened == 1 {
				return &scriptedPoolSession{err: fmt.Errorf("%w: expirada", capture.ErrLoginPage)}, nil
			}
			return &scriptedPoolSession{err: fmt.Errorf("%w: sin lista", capture.ErrSelectorNotFound)}, nil
		})
		worker := &CaptureChecklistWorker{dataDir: t.TempDir()}
		outcome := worker.captureChecklistTarget(context.Background(), checklistTargetParams{
			Target: target, BaseURL: base, UseBrowser: true, Sessions: pool, AutoRenew: autoRenew,
		})
		pool.closeAll()
		expired := strings.Contains(outcome.failure, "sesión de Zajuna expirada")
		if autoRenew && (opened != 2 || expired) {
			t.Fatalf("auto-renew must log in again and retry once: opened=%d outcome=%#v", opened, outcome)
		}
		if !autoRenew && (opened != 1 || !expired) {
			t.Fatalf("without auto-renew an expired session fails the target: opened=%d outcome=%#v", opened, outcome)
		}
	}
}
