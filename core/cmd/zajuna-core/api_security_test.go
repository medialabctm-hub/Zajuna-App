package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

const testLauncherSecret = "launcher-secret-for-tests"

func newTestSession(t *testing.T) *localSession {
	t.Helper()
	session, err := newLocalSession(testLauncherSecret)
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func noContentHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
}

func authenticate(r *http.Request, session *localSession) {
	r.AddCookie(&http.Cookie{Name: capabilityCookieName(r.Host), Value: session.cookieSecret})
	r.Header.Set(capabilityHeaderName, session.headerSecret)
}

func serve(handler http.Handler, r *http.Request) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, r)
	return response
}

func TestProtectLocalAPINeverIssuesCookieOnPlainRequests(t *testing.T) {
	session := newTestSession(t)
	handler := protectLocalAPI(noContentHandler(), session)
	for _, target := range []string{"/", "/index.html", "/resumen", "/api/health", "/api/setup/status", "/api/jobs"} {
		response := serve(handler, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:43123"+target, nil))
		if cookies := response.Result().Cookies(); len(cookies) > 0 {
			t.Fatalf("GET %s issued a cookie: %v", target, cookies)
		}
	}
}

func TestProtectLocalAPIRequiresSessionForSensitiveReadsAndMutations(t *testing.T) {
	session := newTestSession(t)
	handler := protectLocalAPI(noContentHandler(), session)
	cases := []struct{ method, path string }{
		{http.MethodGet, "/api/setup/status"},
		{http.MethodGet, "/api/diagnostics"},
		{http.MethodGet, "/api/backups/zajuna.zip/download"},
		{http.MethodPost, "/api/setup"},
		{http.MethodDelete, "/api/backups/zajuna.zip"},
	}
	for _, item := range cases {
		response := serve(handler, httptest.NewRequest(item.method, "http://127.0.0.1:43123"+item.path, nil))
		if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), localSessionErrorCode) {
			t.Fatalf("%s %s without session = %d %s", item.method, item.path, response.Code, response.Body)
		}
	}
}

func TestProtectLocalAPICookieAloneIsNotEnough(t *testing.T) {
	session := newTestSession(t)
	handler := protectLocalAPI(noContentHandler(), session)

	// A process on another 127.0.0.1 port can harvest the cookie, but never
	// the header secret: JSON reads and mutations must still be refused.
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		request := httptest.NewRequest(method, "http://127.0.0.1:43123/api/setup/status", nil)
		request.AddCookie(&http.Cookie{Name: capabilityCookieName(request.Host), Value: session.cookieSecret})
		if response := serve(handler, request); response.Code != http.StatusUnauthorized {
			t.Fatalf("%s with cookie only = %d", method, response.Code)
		}
	}

	// Downloads are loaded by <img>/<a href> and only need the cookie.
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:43123/api/reports/r1/download", nil)
	request.AddCookie(&http.Cookie{Name: capabilityCookieName(request.Host), Value: session.cookieSecret})
	if response := serve(handler, request); response.Code != http.StatusNoContent {
		t.Fatalf("download with cookie = %d", response.Code)
	}
	// Evidence thumbnails are <img> sources too.
	request = httptest.NewRequest(http.MethodGet, "http://127.0.0.1:43123/api/evidences/e1/thumbnail", nil)
	request.AddCookie(&http.Cookie{Name: capabilityCookieName(request.Host), Value: session.cookieSecret})
	if response := serve(handler, request); response.Code != http.StatusNoContent {
		t.Fatalf("thumbnail with cookie = %d", response.Code)
	}
	// Only GET/HEAD of the exact shapes: other evidence routes still need the header.
	for _, path := range []string{"/api/evidences/e1", "/api/evidences/e1/thumbnail/x", "/api/reports/r1/thumbnail"} {
		request = httptest.NewRequest(http.MethodGet, "http://127.0.0.1:43123"+path, nil)
		request.AddCookie(&http.Cookie{Name: capabilityCookieName(request.Host), Value: session.cookieSecret})
		if response := serve(handler, request); response.Code != http.StatusUnauthorized {
			t.Fatalf("%s with cookie only = %d", path, response.Code)
		}
	}

	// The header secret without the cookie is rejected too.
	request = httptest.NewRequest(http.MethodGet, "http://127.0.0.1:43123/api/setup/status", nil)
	request.Header.Set(capabilityHeaderName, session.headerSecret)
	if response := serve(handler, request); response.Code != http.StatusUnauthorized {
		t.Fatalf("header only = %d", response.Code)
	}

	// A cookie scoped to another port does not unlock this core.
	request = httptest.NewRequest(http.MethodGet, "http://127.0.0.1:43123/api/setup/status", nil)
	request.AddCookie(&http.Cookie{Name: capabilityCookiePrefix + "9999", Value: session.cookieSecret})
	request.Header.Set(capabilityHeaderName, session.headerSecret)
	if response := serve(handler, request); response.Code != http.StatusUnauthorized {
		t.Fatalf("cookie of another port = %d", response.Code)
	}
}

func TestProtectLocalAPIRejectsCrossOriginAndInvalidContentType(t *testing.T) {
	session := newTestSession(t)
	handler := protectLocalAPI(noContentHandler(), session)

	request := httptest.NewRequest(http.MethodPut, "http://127.0.0.1:43123/api/settings", strings.NewReader(`{}`))
	authenticate(request, session)
	request.Header.Set("Content-Type", "text/plain")
	request.Header.Set("Origin", "https://evil.example")
	if response := serve(handler, request); response.Code != http.StatusForbidden {
		t.Fatalf("expected cross-origin request to be rejected first, got %d", response.Code)
	}

	request = httptest.NewRequest(http.MethodPut, "http://127.0.0.1:43123/api/settings", strings.NewReader(`{}`))
	authenticate(request, session)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://127.0.0.1:9999")
	if response := serve(handler, request); response.Code != http.StatusForbidden {
		t.Fatalf("expected another loopback port to be rejected, got %d", response.Code)
	}

	for _, site := range []string{"same-site", "cross-site"} {
		request = httptest.NewRequest(http.MethodGet, "http://127.0.0.1:43123/api/setup/status", nil)
		authenticate(request, session)
		request.Header.Set("Sec-Fetch-Site", site)
		if response := serve(handler, request); response.Code != http.StatusForbidden {
			t.Fatalf("Sec-Fetch-Site %s = %d", site, response.Code)
		}
	}

	request = httptest.NewRequest(http.MethodPut, "http://127.0.0.1:43123/api/settings", strings.NewReader(`{}`))
	authenticate(request, session)
	request.Header.Set("Content-Type", "text/plain")
	if response := serve(handler, request); response.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected invalid content type to be rejected, got %d", response.Code)
	}

	request = httptest.NewRequest(http.MethodGet, "http://evil.example/api/health", nil)
	if response := serve(handler, request); response.Code != http.StatusBadRequest {
		t.Fatalf("non-loopback Host (DNS rebinding) = %d", response.Code)
	}
}

func TestProtectLocalAPIAllowsEmptyBodyMutation(t *testing.T) {
	session := newTestSession(t)
	handler := protectLocalAPI(noContentHandler(), session)
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:43123/api/backups", nil)
	authenticate(request, session)
	if response := serve(handler, request); response.Code != http.StatusNoContent {
		t.Fatalf("expected empty-body action to pass, got %d", response.Code)
	}
}

func TestLocalSessionBootstrapTokensAreSingleUseAndExpire(t *testing.T) {
	session := newTestSession(t)
	now := time.Now()
	session.now = func() time.Time { return now }
	token, err := session.MintBootstrap()
	if err != nil {
		t.Fatal(err)
	}
	if !session.redeem(token) || session.redeem(token) {
		t.Fatal("a bootstrap token must be redeemable exactly once")
	}
	expired, _ := session.MintBootstrap()
	now = now.Add(bootstrapTokenTTL + time.Second)
	if session.redeem(expired) {
		t.Fatal("an expired bootstrap token must be rejected")
	}
	if session.redeem("") || session.redeem("forged") {
		t.Fatal("unknown tokens must be rejected")
	}
	for i := 0; i < maxPendingBootstraps; i++ {
		if _, err := session.MintBootstrap(); err != nil {
			t.Fatalf("mint %d: %v", i, err)
		}
	}
	if _, err := session.MintBootstrap(); err == nil {
		t.Fatal("pending bootstrap tokens must be bounded")
	}
}

// blackBoxClient never follows redirects so the test sees the bootstrap
// response exactly as a browser would receive it.
func blackBoxClient() *http.Client {
	return &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func startBlackBoxCore(t *testing.T) (*httptest.Server, *localSession) {
	t.Helper()
	session := newTestSession(t)
	server := httptest.NewServer(protectLocalAPI(newRouter(t.TempDir()), session))
	t.Cleanup(server.Close)
	return server, session
}

func mintBootstrapPath(t *testing.T, client *http.Client, baseURL, secret string) (int, string) {
	t.Helper()
	request, _ := http.NewRequest(http.MethodPost, baseURL+sessionBootstrapPath, nil)
	if secret != "" {
		request.Header.Set("Authorization", "Bearer "+secret)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var body map[string]string
	_ = json.NewDecoder(response.Body).Decode(&body)
	return response.StatusCode, body["path"]
}

type browserSession struct {
	cookie *http.Cookie
	header string
}

func startBrowserSession(t *testing.T, client *http.Client, baseURL, path string) browserSession {
	t.Helper()
	response, err := client.Get(baseURL + path)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusSeeOther {
		t.Fatalf("session start = %d", response.StatusCode)
	}
	if response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("the bootstrap response must not be cached")
	}
	location, err := url.Parse(response.Header.Get("Location"))
	if err != nil || location.Path != "/" || location.RawQuery != "" {
		t.Fatalf("redirect must drop the token from the URL: %q", response.Header.Get("Location"))
	}
	fragment, _ := url.ParseQuery(location.Fragment)
	cookies := response.Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("expected one HttpOnly SameSite=Strict cookie, got %#v", cookies)
	}
	if fragment.Get(capabilityFragmentKey) == "" || fragment.Get(capabilityFragmentKey) == cookies[0].Value {
		t.Fatal("the header secret must be delivered in the fragment and differ from the cookie")
	}
	return browserSession{cookie: cookies[0], header: fragment.Get(capabilityFragmentKey)}
}

func doWithSession(t *testing.T, client *http.Client, method, target string, body io.Reader, session *browserSession) *http.Response {
	t.Helper()
	request, _ := http.NewRequest(method, target, body)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if session != nil {
		request.AddCookie(session.cookie)
		request.Header.Set(capabilityHeaderName, session.header)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func TestLocalCoreBlackBoxBootstrapFlow(t *testing.T) {
	server, _ := startBlackBoxCore(t)
	client := blackBoxClient()

	// An arbitrary local process: the health probe works but reveals nothing,
	// and no GET hands out a usable cookie.
	health, err := client.Get(server.URL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	var healthBody map[string]any
	_ = json.NewDecoder(health.Body).Decode(&healthBody)
	health.Body.Close()
	if health.StatusCode != http.StatusOK || len(healthBody) != 1 || healthBody["status"] != "ok" || len(health.Cookies()) != 0 {
		t.Fatalf("health must be minimal: %d %#v %v", health.StatusCode, healthBody, health.Cookies())
	}
	for _, target := range []string{"/", "/api/setup/status", "/api/app/info"} {
		response := doWithSession(t, client, http.MethodGet, server.URL+target, nil, nil)
		response.Body.Close()
		if len(response.Cookies()) != 0 {
			t.Fatalf("GET %s issued a cookie", target)
		}
		if strings.HasPrefix(target, "/api/") && response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("GET %s without session = %d", target, response.StatusCode)
		}
	}
	mutation := doWithSession(t, client, http.MethodPost, server.URL+"/api/setup", strings.NewReader(`{"zajunaUsername":"x","zajunaPassword":"y"}`), nil)
	mutation.Body.Close()
	if mutation.StatusCode != http.StatusUnauthorized {
		t.Fatalf("mutation without session = %d", mutation.StatusCode)
	}

	// Minting requires the launcher secret.
	if status, _ := mintBootstrapPath(t, client, server.URL, ""); status != http.StatusUnauthorized {
		t.Fatalf("mint without secret = %d", status)
	}
	if status, _ := mintBootstrapPath(t, client, server.URL, "wrong"); status != http.StatusUnauthorized {
		t.Fatalf("mint with wrong secret = %d", status)
	}
	status, path := mintBootstrapPath(t, client, server.URL, testLauncherSecret)
	if status != http.StatusOK || !strings.HasPrefix(path, sessionStartPath+"?token=") {
		t.Fatalf("mint = %d %q", status, path)
	}

	first := startBrowserSession(t, client, server.URL, path)
	replay, err := client.Get(server.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	replay.Body.Close()
	if replay.StatusCode != http.StatusForbidden || len(replay.Cookies()) != 0 {
		t.Fatalf("a replayed bootstrap URL must be refused: %d", replay.StatusCode)
	}

	status200 := doWithSession(t, client, http.MethodGet, server.URL+"/api/setup/status", nil, &first)
	status200.Body.Close()
	if status200.StatusCode != http.StatusOK {
		t.Fatalf("authenticated read = %d", status200.StatusCode)
	}

	// Second launch (Electron second-instance or another browser): a fresh
	// token yields the same per-process session.
	_, secondPath := mintBootstrapPath(t, client, server.URL, testLauncherSecret)
	second := startBrowserSession(t, client, server.URL, secondPath)
	if second.cookie.Value != first.cookie.Value || second.header != first.header {
		t.Fatal("every launch must join the same per-process session")
	}
	settings := doWithSession(t, client, http.MethodPost, server.URL+"/api/jobs/dismiss", strings.NewReader(`{"ids":["job-1"]}`), &second)
	settings.Body.Close()
	if settings.StatusCode == http.StatusUnauthorized || settings.StatusCode == http.StatusForbidden {
		t.Fatalf("authenticated mutation was blocked: %d", settings.StatusCode)
	}
}

func TestLocalCoreBlackBoxSessionsDoNotSurviveRestart(t *testing.T) {
	server, _ := startBlackBoxCore(t)
	client := blackBoxClient()
	_, path := mintBootstrapPath(t, client, server.URL, testLauncherSecret)
	old := startBrowserSession(t, client, server.URL, path)

	restarted, _ := startBlackBoxCore(t)
	oldOnNew := browserSession{cookie: &http.Cookie{Name: capabilityCookieName(strings.TrimPrefix(restarted.URL, "http://")), Value: old.cookie.Value}, header: old.header}
	response := doWithSession(t, client, http.MethodGet, restarted.URL+"/api/setup/status", nil, &oldOnNew)
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("a previous process session must not unlock a new core: %d", response.StatusCode)
	}
}
