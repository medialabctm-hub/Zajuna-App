package main

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	capabilityCookiePrefix = "zajuna_capability_"
	capabilityHeaderName   = "X-Zajuna-Capability"
	capabilityFragmentKey  = "zc"
	launcherSecretEnv      = "ZAJUNA_LAUNCHER_SECRET"
	sessionStartPath       = "/api/session/start"
	sessionBootstrapPath   = "/api/session/bootstrap"
	bootstrapTokenTTL      = 2 * time.Minute
	maxPendingBootstraps   = 32
	maxAPIRequestBytes     = 32 << 20
	localSessionErrorCode  = "local_session_required"
)

// Cookie-only routes are loaded by <img>/<a href> and cannot carry the
// capability header; they are read-only and still require the cookie. The
// evidence thumbnail is an <img> source in Evidencias and Revisión.
var cookieOnlyRoute = regexp.MustCompile(`^/api/(?:(?:evidences|reports|backups)/[^/]+/download|evidences/[^/]+/thumbnail)$`)

func newCapabilityToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

// localSession holds the per-process secrets of the loopback UI.
//
//   - cookieSecret travels as an HttpOnly, SameSite=Strict cookie.
//   - headerSecret is handed to the page through the URL fragment (never sent
//     to any server) and kept in origin-scoped storage. Cookies are not
//     port-isolated, so a process listening on another 127.0.0.1 port could
//     receive the cookie; it can never read the header secret.
//   - launcherSecret is shared only with the Electron supervisor (or kept
//     in-process when the core opens the browser itself) and is the only way
//     to mint single-use bootstrap tokens.
type localSession struct {
	cookieSecret   string
	headerSecret   string
	launcherSecret string

	mu      sync.Mutex
	pending map[string]time.Time
	now     func() time.Time
}

func newLocalSession(launcherSecret string) (*localSession, error) {
	cookieSecret, err := newCapabilityToken()
	if err != nil {
		return nil, err
	}
	headerSecret, err := newCapabilityToken()
	if err != nil {
		return nil, err
	}
	launcherSecret = strings.TrimSpace(launcherSecret)
	if launcherSecret == "" {
		if launcherSecret, err = newCapabilityToken(); err != nil {
			return nil, err
		}
	}
	return &localSession{
		cookieSecret: cookieSecret, headerSecret: headerSecret, launcherSecret: launcherSecret,
		pending: map[string]time.Time{}, now: time.Now,
	}, nil
}

func tokenKey(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// MintBootstrap returns a single-use token that turns one browser navigation
// into an authenticated session. It expires after bootstrapTokenTTL.
func (s *localSession) MintBootstrap() (string, error) {
	token, err := newCapabilityToken()
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for key, expiry := range s.pending {
		if !now.Before(expiry) {
			delete(s.pending, key)
		}
	}
	if len(s.pending) >= maxPendingBootstraps {
		return "", errors.New("hay demasiados inicios de sesión locales pendientes")
	}
	s.pending[tokenKey(token)] = now.Add(bootstrapTokenTTL)
	return token, nil
}

func (s *localSession) redeem(token string) bool {
	if strings.TrimSpace(token) == "" {
		return false
	}
	key := tokenKey(token)
	s.mu.Lock()
	defer s.mu.Unlock()
	expiry, ok := s.pending[key]
	delete(s.pending, key)
	return ok && s.now().Before(expiry)
}

func (s *localSession) StartPath(token string) string {
	return sessionStartPath + "?token=" + url.QueryEscape(token)
}

func secretEqual(got, want string) bool {
	return got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func capabilityCookieName(host string) string {
	_, port, err := net.SplitHostPort(host)
	if err != nil || port == "" {
		port = "default"
	}
	return capabilityCookiePrefix + port
}

func (s *localSession) hasCookie(r *http.Request) bool {
	cookie, err := r.Cookie(capabilityCookieName(r.Host))
	return err == nil && secretEqual(cookie.Value, s.cookieSecret)
}

func (s *localSession) hasHeader(r *http.Request) bool {
	return secretEqual(r.Header.Get(capabilityHeaderName), s.headerSecret)
}

func (s *localSession) hasLauncherSecret(r *http.Request) bool {
	value := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(value) <= len(prefix) || !strings.EqualFold(value[:len(prefix)], prefix) {
		return false
	}
	return secretEqual(strings.TrimSpace(value[len(prefix):]), s.launcherSecret)
}

// protectLocalAPI enforces the local capability on every /api route except
// the minimal health probe. Static assets stay public: they hold no data.
func protectLocalAPI(next http.Handler, session *localSession) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLoopbackHost(r.Host) {
			writeError(w, http.StatusBadRequest, errors.New("el core solo acepta solicitudes loopback"))
			return
		}
		path := r.URL.Path
		if path != "/api" && !strings.HasPrefix(path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		if path == "/api/health" && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
			next.ServeHTTP(w, r)
			return
		}
		if err := rejectForeignFetchSite(r); err != nil {
			writeError(w, http.StatusForbidden, err)
			return
		}
		switch path {
		case sessionStartPath:
			session.serveStart(w, r)
			return
		case sessionBootstrapPath:
			session.serveBootstrap(w, r)
			return
		}
		if !session.hasCookie(r) || (!isCookieOnlyRequest(r) && !session.hasHeader(r)) {
			writeLocalSessionRequired(w)
			return
		}
		if isMutatingMethod(r.Method) {
			if err := validateLocalRequestOrigin(r); err != nil {
				writeError(w, http.StatusForbidden, err)
				return
			}
			if err := validateRequestContentType(r); err != nil {
				writeError(w, http.StatusUnsupportedMediaType, err)
				return
			}
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, maxAPIRequestBytes)
			}
		}
		next.ServeHTTP(w, r)
	})
}

func writeLocalSessionRequired(w http.ResponseWriter) {
	writeJSON(w, http.StatusUnauthorized, map[string]string{
		"error": "la sesión local no es válida; vuelve a abrir Zajuna App desde su acceso directo",
		"code":  localSessionErrorCode,
	})
}

// serveStart redeems a bootstrap token from a top-level navigation, sets the
// session cookie and hands the header secret to the page in the fragment.
func (s *localSession) serveStart(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, errors.New("método no permitido"))
		return
	}
	if !s.redeem(r.URL.Query().Get("token")) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("El enlace de inicio de Zajuna App ya se usó o expiró. Vuelve a abrir la aplicación desde su acceso directo.\n"))
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: capabilityCookieName(r.Host), Value: s.cookieSecret, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteStrictMode,
	})
	http.Redirect(w, r, "/#"+capabilityFragmentKey+"="+s.headerSecret, http.StatusSeeOther)
}

// serveBootstrap lets the launcher mint a bootstrap URL for the first and any
// later launch (second instance, core recovery).
func (s *localSession) serveBootstrap(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, errors.New("método no permitido"))
		return
	}
	if !s.hasLauncherSecret(r) {
		writeError(w, http.StatusUnauthorized, errors.New("el lanzador local no está autorizado"))
		return
	}
	token, err := s.MintBootstrap()
	if err != nil {
		writeError(w, http.StatusTooManyRequests, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": s.StartPath(token)})
}

func isCookieOnlyRequest(r *http.Request) bool {
	return (r.Method == http.MethodGet || r.Method == http.MethodHead) && cookieOnlyRoute.MatchString(r.URL.Path)
}

// rejectForeignFetchSite blocks browser requests initiated by any other
// origin. "same-site" matters here: every port on 127.0.0.1 is the same site.
func rejectForeignFetchSite(r *http.Request) error {
	switch strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site"))) {
	case "cross-site", "same-site":
		return errors.New("la solicitud desde otro sitio no está permitida")
	}
	return nil
}

func isMutatingMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func isLoopbackHost(rawHost string) bool {
	host := rawHost
	if parsedHost, _, err := net.SplitHostPort(rawHost); err == nil {
		host = parsedHost
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func validateLocalRequestOrigin(r *http.Request) error {
	if err := rejectForeignFetchSite(r); err != nil {
		return err
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return nil
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme != "http" || parsed.Host == "" || !isLoopbackHost(parsed.Host) {
		return errors.New("el origen de la solicitud no es loopback")
	}
	if !sameHost(r.Host, parsed.Host) {
		return errors.New("el origen de la solicitud no coincide con el core local")
	}
	return nil
}

func sameHost(left, right string) bool {
	leftHost, leftPort, leftErr := net.SplitHostPort(left)
	rightHost, rightPort, rightErr := net.SplitHostPort(right)
	if leftErr != nil {
		leftHost, leftPort = left, ""
	}
	if rightErr != nil {
		rightHost, rightPort = right, ""
	}
	return strings.EqualFold(strings.Trim(leftHost, "[]"), strings.Trim(rightHost, "[]")) && leftPort == rightPort
}

func validateRequestContentType(r *http.Request) error {
	contentType := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	if strings.HasPrefix(r.URL.Path, "/api/evidences/upload") {
		if !strings.HasPrefix(contentType, "multipart/form-data") {
			return errors.New("la carga de evidencias requiere multipart/form-data")
		}
		return nil
	}
	// Empty-body actions (cancel, restore, create-backup, notification read)
	// do not need a media type. Any request carrying data must declare JSON.
	if r.ContentLength == 0 && len(r.TransferEncoding) == 0 {
		return nil
	}
	if !strings.HasPrefix(contentType, "application/json") {
		return errors.New("las mutaciones JSON requieren Content-Type application/json")
	}
	return nil
}
