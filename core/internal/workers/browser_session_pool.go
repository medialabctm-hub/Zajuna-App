package workers

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/zajuna-app/core/internal/capture"
)

// checklistBrowserSession is the part of capture.BrowserSession the checklist
// fan-out uses; tests replace it with a fake.
type checklistBrowserSession interface {
	CaptureURLWithMetadataAndOptions(ctx context.Context, targetURL, outputPath string, options capture.CaptureOptions) (capture.CaptureResult, error)
	Close()
}

// browserSessionPool reuses authenticated Chromium sessions across the
// targets of one checklist run. Opening a session per target meant one
// Zajuna login per evidence (68 in a real course), which Zajuna throttled
// with login timeouts. A session is used by one goroutine at a time, so the
// pool never holds more sessions than the fan-out concurrency.
type browserSessionPool struct {
	open     func(context.Context) (checklistBrowserSession, error)
	attempts int
	backoff  time.Duration

	mu   sync.Mutex
	idle []checklistBrowserSession
	all  []checklistBrowserSession
}

func newBrowserSessionPool(open func(context.Context) (checklistBrowserSession, error)) *browserSessionPool {
	return &browserSessionPool{open: open, attempts: 3, backoff: 2 * time.Second}
}

// acquire returns an idle session or opens a new one, retrying transient
// login failures (timeouts, slow Zajuna responses). Blocked pages and
// CAPTCHA/MFA challenges are not retried.
func (p *browserSessionPool) acquire(ctx context.Context) (checklistBrowserSession, error) {
	p.mu.Lock()
	if count := len(p.idle); count > 0 {
		session := p.idle[count-1]
		p.idle = p.idle[:count-1]
		p.mu.Unlock()
		return session, nil
	}
	p.mu.Unlock()
	return p.acquireFresh(ctx)
}

// acquireFresh always opens a new session (a new Zajuna login), skipping
// idle ones that may have expired together with the one being replaced.
func (p *browserSessionPool) acquireFresh(ctx context.Context) (checklistBrowserSession, error) {
	var lastErr error
	for attempt := 0; attempt < p.attempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * p.backoff):
			}
		}
		session, err := p.open(ctx)
		if err == nil {
			p.mu.Lock()
			p.all = append(p.all, session)
			p.mu.Unlock()
			return session, nil
		}
		lastErr = err
		if errors.Is(err, capture.ErrChallengePage) || errors.Is(err, capture.ErrBlockedPage) || ctx.Err() != nil {
			break
		}
	}
	return nil, lastErr
}

// release returns a healthy session for reuse; an unhealthy one (expired
// login, broken page) is closed so the next target opens a fresh session.
func (p *browserSessionPool) release(session checklistBrowserSession, healthy bool) {
	if session == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if healthy {
		p.idle = append(p.idle, session)
		return
	}
	for index, candidate := range p.all {
		if candidate == session {
			p.all = append(p.all[:index], p.all[index+1:]...)
			break
		}
	}
	session.Close()
}

func (p *browserSessionPool) closeAll() {
	p.mu.Lock()
	sessions := p.all
	p.all, p.idle = nil, nil
	p.mu.Unlock()
	for _, session := range sessions {
		session.Close()
	}
}
