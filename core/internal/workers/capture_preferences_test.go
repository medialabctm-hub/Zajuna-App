package workers

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/zajuna-app/core/internal/capture"
	"github.com/zajuna-app/core/internal/checklist"
	"github.com/zajuna-app/core/internal/evidence"
)

type recordingEvidenceStore struct{ records []evidence.Record }

func (s *recordingEvidenceStore) CreateEvidence(_ context.Context, record evidence.Record) error {
	s.records = append(s.records, record)
	return nil
}
func (s *recordingEvidenceStore) GetEvidence(context.Context, string) (evidence.Record, error) {
	return evidence.Record{}, fmt.Errorf("not found")
}
func (s *recordingEvidenceStore) ListEvidences(context.Context, int) ([]evidence.Record, error) {
	return s.records, nil
}

// scriptedSession lands on the login page when expired; otherwise it writes
// the PNG so the worker can hash it.
type scriptedSession struct {
	expired bool
	closed  bool
	options capture.CaptureOptions
}

func (s *scriptedSession) CaptureURLWithMetadataAndOptions(_ context.Context, targetURL, outputPath string, options capture.CaptureOptions) (capture.CaptureResult, error) {
	s.options = options
	if s.expired {
		return capture.CaptureResult{}, fmt.Errorf("%w: redirect", capture.ErrLoginPage)
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o700); err != nil {
		return capture.CaptureResult{}, err
	}
	if err := os.WriteFile(outputPath, []byte("png"), 0o600); err != nil {
		return capture.CaptureResult{}, err
	}
	return capture.CaptureResult{FinalURL: targetURL}, nil
}
func (s *scriptedSession) Close() { s.closed = true }

func preferencesTestParams(t *testing.T, sessions []*scriptedSession, autoRenew bool) (*CaptureChecklistWorker, checklistTargetParams, *int) {
	t.Helper()
	baseURL, _ := url.Parse("https://zajuna.sena.edu.co")
	opened := 0
	pool := newBrowserSessionPool(func(context.Context) (checklistBrowserSession, error) {
		if opened >= len(sessions) {
			return nil, fmt.Errorf("no more sessions")
		}
		session := sessions[opened]
		opened++
		return session, nil
	})
	pool.backoff = 0
	pool.attempts = 1
	worker := &CaptureChecklistWorker{dataDir: t.TempDir(), evidence: &recordingEvidenceStore{}}
	params := checklistTargetParams{
		JobID:      "job-1",
		Input:      CaptureChecklistInput{FichaID: "ficha-1", Username: "123", DocumentType: "CC"},
		Target:     checklist.CaptureTarget{ItemCode: "1.1.1", SlotNumber: 1, URL: "https://zajuna.sena.edu.co/zajuna/course/view.php?id=1"},
		BaseURL:    baseURL,
		UseBrowser: true,
		Sessions:   pool,
		AutoRenew:  autoRenew,
	}
	return worker, params, &opened
}

func TestAutoRenewRetriesTargetWithFreshLogin(t *testing.T) {
	expired := &scriptedSession{expired: true}
	fresh := &scriptedSession{}
	worker, params, opened := preferencesTestParams(t, []*scriptedSession{expired, fresh}, true)

	outcome := worker.captureChecklistTarget(context.Background(), params)

	if !outcome.captured {
		t.Fatalf("autoRenew must recover an expired session, got %#v", outcome)
	}
	if *opened != 2 || !expired.closed {
		t.Fatalf("expected the expired session closed and one fresh login, opened=%d closed=%v", *opened, expired.closed)
	}
}

func TestAutoRenewSkipsIdleSessionsThatMayHaveExpired(t *testing.T) {
	otherStale := &scriptedSession{expired: true}
	stale := &scriptedSession{expired: true}
	fresh := &scriptedSession{}
	worker, params, opened := preferencesTestParams(t, []*scriptedSession{fresh}, true)
	params.Sessions.all = append(params.Sessions.all, otherStale, stale)
	params.Sessions.idle = append(params.Sessions.idle, otherStale, stale)

	outcome := worker.captureChecklistTarget(context.Background(), params)

	if !outcome.captured {
		t.Fatalf("the retry must use a fresh login, not another idle session: %#v", outcome)
	}
	if *opened != 1 || !stale.closed {
		t.Fatalf("expected the expired session closed and one fresh login, opened=%d closed=%v", *opened, stale.closed)
	}
	if otherStale.closed || len(params.Sessions.idle) != 2 || params.Sessions.idle[0] != otherStale {
		t.Fatalf("the retry must not consume another idle session: idle=%d", len(params.Sessions.idle))
	}
}

func TestWithoutAutoRenewExpiredSessionFailsTarget(t *testing.T) {
	expired := &scriptedSession{expired: true}
	worker, params, opened := preferencesTestParams(t, []*scriptedSession{expired, {}}, false)

	outcome := worker.captureChecklistTarget(context.Background(), params)

	if outcome.captured || outcome.failure == "" {
		t.Fatalf("without autoRenew an expired session must fail the target, got %#v", outcome)
	}
	if *opened != 1 {
		t.Fatalf("without autoRenew no extra login is attempted, opened=%d", *opened)
	}
}

func TestApplyCapturePreferencesFullPage(t *testing.T) {
	targets := []checklist.CaptureTarget{{ItemCode: "2.1", FullPage: true}, {ItemCode: "3.1"}}

	kept := applyCapturePreferences(targets, CapturePreferences{FullPage: true})
	if !kept[0].FullPage {
		t.Fatal("fullPage on keeps the full-page rule of the target")
	}

	cropped := applyCapturePreferences(targets, CapturePreferences{FullPage: false})
	if cropped[0].FullPage || cropped[1].FullPage {
		t.Fatalf("fullPage off must capture only the matched block: %#v", cropped)
	}
	if !targets[0].FullPage {
		t.Fatal("applyCapturePreferences must not mutate the planned targets")
	}
}

func TestFullPagePreferenceReachesBrowserOptions(t *testing.T) {
	session := &scriptedSession{}
	worker, params, _ := preferencesTestParams(t, []*scriptedSession{session}, true)
	params.Target.FullPage = true
	params.Target = applyCapturePreferences([]checklist.CaptureTarget{params.Target}, CapturePreferences{FullPage: false})[0]

	if outcome := worker.captureChecklistTarget(context.Background(), params); !outcome.captured {
		t.Fatalf("capture failed: %#v", outcome)
	}
	if session.options.FullPage {
		t.Fatal("the browser must receive FullPage=false when the preference is off")
	}
}

func TestCaptureChecklistPreferencesDefaultAndLoader(t *testing.T) {
	worker := &CaptureChecklistWorker{}
	if got := worker.preferences(context.Background()); got != DefaultCapturePreferences() {
		t.Fatalf("without a loader the worker keeps the defaults, got %#v", got)
	}
	want := CapturePreferences{FullPage: false, ReuseSession: false, AutoRenew: true}
	worker.SetPreferences(func(context.Context) CapturePreferences { return want })
	if got := worker.preferences(context.Background()); got != want {
		t.Fatalf("loader preferences not applied: %#v", got)
	}
}
