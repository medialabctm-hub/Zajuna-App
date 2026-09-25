package capture

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func testNetworkPolicy(t *testing.T, origin string) *networkPolicy {
	t.Helper()
	policy := newNetworkPolicy(mustURL(t, origin))
	policy.lookup = func(host string) ([]net.IP, error) {
		switch host {
		case "docs.google.com", "fonts.gstatic.com", "doc-0s-sheets.googleusercontent.com":
			return []net.IP{net.ParseIP("142.250.78.14")}, nil
		case "rebind.googleusercontent.com":
			return []net.IP{net.ParseIP("10.0.0.5")}, nil
		}
		return nil, errors.New("no such host")
	}
	return policy
}

func TestNetworkPolicyKeepsTopLevelNavigationOnTheZajunaOrigin(t *testing.T) {
	policy := testNetworkPolicy(t, "https://zajuna.sena.edu.co/zajuna/course/view.php?id=41080")
	if err := policy.check("https://zajuna.sena.edu.co/zajuna/mod/page/view.php?id=1", true); err != nil {
		t.Fatalf("same-origin navigation was blocked: %v", err)
	}
	for _, raw := range []string{
		"https://docs.google.com/spreadsheets/d/x/pubhtml",
		"https://evil.example/phish",
		"http://zajuna.sena.edu.co/zajuna/",
		"http://192.168.1.10/admin",
		"data:text/html,hi",
		"https://user:pass@zajuna.sena.edu.co/",
	} {
		if err := policy.check(raw, true); err == nil {
			t.Fatalf("top-level navigation to %s must be blocked", raw)
		}
	}
}

func TestNetworkPolicyAllowsOnlyExplicitPublicSubresources(t *testing.T) {
	policy := testNetworkPolicy(t, "https://zajuna.sena.edu.co/zajuna/course/view.php?id=41080")
	for _, raw := range []string{
		"https://zajuna.sena.edu.co/zajuna/theme/style.css",
		"https://docs.google.com/spreadsheets/d/x/pubhtml?widget=true",
		"https://fonts.gstatic.com/s/roboto.woff2",
		"https://doc-0s-sheets.googleusercontent.com/img.png",
		"data:image/png;base64,AAAA",
		"blob:https://zajuna.sena.edu.co/uuid",
	} {
		if err := policy.check(raw, false); err != nil {
			t.Fatalf("needed resource %s was blocked: %v", raw, err)
		}
	}
	for _, raw := range []string{
		"http://docs.google.com/spreadsheets/d/x",
		"https://cdn.evil.example/tracker.js",
		"https://evilgoogleusercontent.com/x.png",
		"https://rebind.googleusercontent.com/x.png",
		"https://unresolvable.googleusercontent.com/x.png",
		"http://127.0.0.1:9222/json",
		"http://169.254.169.254/latest/meta-data",
		"https://10.0.0.8/",
		"file:///C:/Windows/win.ini",
		"javascript:alert(1)",
		"ftp://zajuna.sena.edu.co/",
	} {
		if err := policy.check(raw, false); err == nil {
			t.Fatalf("subresource %s must be blocked", raw)
		}
	}
}

func TestNetworkPolicyFixtureOriginDoesNotOpenOtherPrivateHosts(t *testing.T) {
	policy := testNetworkPolicy(t, "http://127.0.0.1:8080/fixture")
	if err := policy.check("http://127.0.0.1:8080/other", true); err != nil {
		t.Fatalf("fixture origin was blocked: %v", err)
	}
	for _, raw := range []string{"http://127.0.0.1:9090/", "http://localhost:8080/", "http://192.168.0.5/"} {
		if err := policy.check(raw, false); err == nil {
			t.Fatalf("%s must stay blocked for a fixture origin", raw)
		}
	}
}

func TestNetworkPolicyCachesHostResolution(t *testing.T) {
	policy := testNetworkPolicy(t, "https://zajuna.sena.edu.co/")
	var lookups int32
	base := policy.lookup
	policy.lookup = func(host string) ([]net.IP, error) {
		atomic.AddInt32(&lookups, 1)
		return base(host)
	}
	for i := 0; i < 5; i++ {
		_ = policy.check("https://fonts.gstatic.com/a.woff2", false)
	}
	if lookups != 1 {
		t.Fatalf("expected one DNS lookup per host, got %d", lookups)
	}
}

func TestRedirectTargetResolvesLocation(t *testing.T) {
	if next, ok := redirectTarget("https://zajuna.sena.edu.co/zajuna/a.php", 302, "/zajuna/b.php"); !ok || next != "https://zajuna.sena.edu.co/zajuna/b.php" {
		t.Fatalf("relative Location = %q,%v", next, ok)
	}
	if next, ok := redirectTarget("https://zajuna.sena.edu.co/", 307, "http://10.0.0.1/"); !ok || next != "http://10.0.0.1/" {
		t.Fatalf("absolute Location = %q,%v", next, ok)
	}
	if _, ok := redirectTarget("https://zajuna.sena.edu.co/", 200, "/x"); ok {
		t.Fatal("a 200 is not a redirect")
	}
	if _, ok := redirectTarget("https://zajuna.sena.edu.co/", 304, ""); ok {
		t.Fatal("a 3xx without Location is not followed")
	}
}

func TestWithPlaywrightEnvIsAtomicPerRuntime(t *testing.T) {
	t.Setenv("PLAYWRIGHT_DRIVER_PATH", "")
	t.Setenv("PLAYWRIGHT_BROWSERS_PATH", "")
	runtimes := []Runtime{newRuntime(filepath.Join(t.TempDir(), "a")), newRuntime(filepath.Join(t.TempDir(), "b"))}
	var wg sync.WaitGroup
	var mismatches int32
	for i := 0; i < 40; i++ {
		runtime := runtimes[i%2]
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = withPlaywrightEnv(runtime, func() error {
				time.Sleep(time.Millisecond)
				if os.Getenv("PLAYWRIGHT_DRIVER_PATH") != runtime.DriverDir || os.Getenv("PLAYWRIGHT_BROWSERS_PATH") != runtime.BrowsersDir {
					atomic.AddInt32(&mismatches, 1)
				}
				return nil
			})
		}()
	}
	wg.Wait()
	if mismatches != 0 {
		t.Fatalf("%d driver launches saw another runtime's environment", mismatches)
	}
}

// TestCaptureNetworkPolicySmoke proves in Chromium that a redirect and a
// subresource to another (private) origin never reach it, while same-origin
// redirects keep working.
func TestCaptureNetworkPolicySmoke(t *testing.T) {
	if os.Getenv("ZAJUNA_RUN_BROWSER_SMOKE") != "1" {
		t.Skip("browser smoke disabled")
	}
	var foreignHits int32
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&foreignHits, 1)
		_, _ = w.Write([]byte(`<html><body><main id="region-main">foreign</main></body></html>`))
	}))
	defer foreign.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/leave":
			http.Redirect(w, r, foreign.URL+"/landing", http.StatusFound)
		case "/hop":
			http.Redirect(w, r, "/page", http.StatusFound)
		default:
			_, _ = w.Write([]byte(fmt.Sprintf(`<html><body><main id="region-main"><h1>Zajuna fixture</h1><img src="%s/pixel.png"><script src="%s/x.js"></script></main></body></html>`, foreign.URL, foreign.URL)))
		}
	}))
	defer origin.Close()
	runtime := Resolve("")
	output := filepath.Join(t.TempDir(), "policy.png")
	result, err := runtime.CaptureURLWithMetadataAndCookiesAndOptions(context.Background(), origin.URL+"/hop", output, nil, CaptureOptions{Selector: "#region-main", RequireSelector: true})
	if err != nil {
		t.Fatalf("same-origin redirect must keep working: %v", err)
	}
	if result.FinalURL != origin.URL+"/page" {
		t.Fatalf("same-origin redirect did not land on /page: %s", result.FinalURL)
	}
	if _, err := runtime.CaptureURLWithMetadataAndCookiesAndOptions(context.Background(), origin.URL+"/leave", filepath.Join(t.TempDir(), "leave.png"), nil, CaptureOptions{Selector: "#region-main", RequireSelector: true}); err == nil {
		t.Fatal("a redirect to another origin must fail the capture")
	}
	if hits := atomic.LoadInt32(&foreignHits); hits != 0 {
		t.Fatalf("the foreign origin was contacted %d times", hits)
	}
}
