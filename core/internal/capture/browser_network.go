package capture

import (
	"errors"
	"net"
	"net/url"
	"strings"
	"sync"

	"github.com/mxschmitt/playwright-go"
	"github.com/zajuna-app/core/internal/security"
)

// externalResourceHosts is the explicit allow-list of third-party hosts a
// capture may load besides the Zajuna origin. Cronogramas embed published
// Google Sheets, whose iframe pulls its styles, scripts and images from
// these hosts. Only HTTPS is allowed and every host must resolve to public
// addresses; the main frame can never navigate to them.
var externalResourceHosts = []string{
	"docs.google.com",
	"fonts.googleapis.com",
	"fonts.gstatic.com",
	"ssl.gstatic.com",
	"www.gstatic.com",
}

// externalResourceHostSuffixes allow a whole Google-served family, e.g.
// doc-0s-*.googleusercontent.com images inside a published sheet.
var externalResourceHostSuffixes = []string{
	".googleusercontent.com",
}

var errBlockedRequest = errors.New("solicitud bloqueada por la política de red de captura")

// networkPolicy decides, before Chromium sends anything, whether a request
// may leave the browser. It is created per browser context and is safe for
// concurrent route callbacks.
type networkPolicy struct {
	origin *url.URL
	lookup func(string) ([]net.IP, error)

	mu      sync.Mutex
	private map[string]bool
}

func newNetworkPolicy(origin *url.URL) *networkPolicy {
	return &networkPolicy{origin: origin, lookup: net.LookupIP, private: make(map[string]bool)}
}

// check validates one request URL. mainFrame navigations (top-level pages
// and each hop of their redirects) must stay on the Zajuna origin;
// subresources and iframes may also use the explicit external allow-list.
func (p *networkPolicy) check(raw string, mainFrame bool) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return errBlockedRequest
	}
	switch strings.ToLower(parsed.Scheme) {
	case "data", "blob":
		// In-memory content created by an already allowed page; no network.
		if mainFrame {
			return errBlockedRequest
		}
		return nil
	case "http", "https":
	default:
		return errBlockedRequest
	}
	if parsed.User != nil || parsed.Host == "" {
		return errBlockedRequest
	}
	if security.AllowedOrigin(parsed, []string{p.origin.Scheme + "://" + p.origin.Host}) {
		return nil
	}
	if mainFrame || !strings.EqualFold(parsed.Scheme, "https") || !allowedExternalHost(parsed.Hostname()) {
		return errBlockedRequest
	}
	if p.resolvesPrivate(parsed.Hostname()) {
		return errBlockedRequest
	}
	return nil
}

func allowedExternalHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, allowed := range externalResourceHosts {
		if host == allowed {
			return true
		}
	}
	for _, suffix := range externalResourceHostSuffixes {
		if strings.HasSuffix(host, suffix) && len(host) > len(suffix) {
			return true
		}
	}
	return false
}

// resolvesPrivate caches per host: a capture loads dozens of resources from
// the same few hosts. Resolution failures count as private (blocked).
func (p *networkPolicy) resolvesPrivate(host string) bool {
	if ip := net.ParseIP(host); ip != nil {
		return security.IsPrivateIP(ip)
	}
	host = strings.ToLower(host)
	p.mu.Lock()
	cached, ok := p.private[host]
	p.mu.Unlock()
	if ok {
		return cached
	}
	private := true
	if ips, err := p.lookup(host); err == nil && len(ips) > 0 {
		private = false
		for _, ip := range ips {
			if security.IsPrivateIP(ip) {
				private = true
				break
			}
		}
	}
	p.mu.Lock()
	p.private[host] = private
	p.mu.Unlock()
	return private
}

// redirectTarget resolves a 3xx Location against the request URL. ok is
// false when the response is not a redirect.
func redirectTarget(requestURL string, status int, location string) (string, bool) {
	if status < 300 || status > 399 || strings.TrimSpace(location) == "" {
		return "", false
	}
	base, err := url.Parse(requestURL)
	if err != nil {
		return location, true
	}
	next, err := url.Parse(strings.TrimSpace(location))
	if err != nil {
		return location, true
	}
	return base.ResolveReference(next).String(), true
}

// installNetworkPolicy routes every request of the context through the
// policy. Navigations are fetched without following redirects so each hop's
// Location is validated before Chromium requests it; Playwright does not
// call route handlers for redirect hops Chromium follows by itself.
func installNetworkPolicy(browserContext playwright.BrowserContext, origin *url.URL) error {
	policy := newNetworkPolicy(origin)
	return browserContext.Route("**/*", func(route playwright.Route) {
		request := route.Request()
		mainFrame := false
		if request.IsNavigationRequest() {
			if frame := request.Frame(); frame != nil && frame.ParentFrame() == nil {
				mainFrame = true
			}
		}
		if err := policy.check(request.URL(), mainFrame); err != nil {
			_ = route.Abort("blockedbyclient")
			return
		}
		if !request.IsNavigationRequest() {
			_ = route.Continue()
			return
		}
		response, err := route.Fetch(playwright.RouteFetchOptions{MaxRedirects: playwright.Int(0)})
		if err != nil {
			_ = route.Abort("failed")
			return
		}
		if next, redirect := redirectTarget(request.URL(), response.Status(), response.Headers()["location"]); redirect {
			if err := policy.check(next, mainFrame); err != nil {
				_ = response.Dispose()
				_ = route.Abort("blockedbyclient")
				return
			}
		}
		_ = route.Fulfill(playwright.RouteFulfillOptions{Response: response})
	})
}
