package security

import (
	"errors"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

var sensitiveParameter = regexp.MustCompile(`(?i)^(?:sesskey|token|access_token|refresh_token|id_token|auth|authorization|cookie|password|secret|key|signature|sig)$`)

// RedactURL removes credential-like query parameters before a URL is persisted
// in a course map, evidence metadata, event or error. Functional Moodle route
// parameters (id, section, action, cmid, etc.) are intentionally preserved.
func RedactURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed == nil {
		return raw
	}
	parsed.User = nil
	query := parsed.Query()
	for key := range query {
		if sensitiveParameter.MatchString(key) {
			delete(query, key)
		}
	}
	parsed.RawQuery = query.Encode()
	parsed.Fragment = ""
	return parsed.String()
}

// RedactText removes the most common secret-bearing key/value forms from
// diagnostics. It is deliberately conservative: ordinary URLs remain intact
// except for sensitive query keys, while credential values are replaced.
func RedactText(value string) string {
	// Before the key patterns: "Authorization: Bearer x" would otherwise
	// only lose the scheme word.
	value = bearerCredential.ReplaceAllString(value, `${1}[redacted]`)
	for _, re := range sensitiveTextPatterns {
		value = re.ReplaceAllString(value, `${1}[redacted]`)
	}
	return value
}

var sensitiveTextPatterns = func() []*regexp.Regexp {
	keys := []string{"password", "token", "sesskey", "access_token", "refresh_token", "secret", "cookie", "authorization", "moodlesession"}
	patterns := make([]*regexp.Regexp, 0, len(keys))
	for _, key := range keys {
		patterns = append(patterns, regexp.MustCompile(`(?i)(`+regexp.QuoteMeta(key)+`\s*[:=]\s*)([^&\s,;]+)`))
	}
	return patterns
}()

var bearerCredential = regexp.MustCompile(`(?i)(\b(?:bearer|basic)\s+)[A-Za-z0-9._~+/=-]{8,}`)

// ValidateHTTPURL validates a target before a worker starts network I/O. When
// allowedOrigins is non-empty, the scheme/host/port must match one of them.
// Private/loopback IPs are rejected unless allowPrivate is explicitly enabled
// (used only by fixture tests and developer-only workers).
func ValidateHTTPURL(raw string, allowedOrigins []string, allowPrivate bool) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" {
		return nil, errors.New("url debe usar http o https")
	}
	if parsed.User != nil {
		return nil, errors.New("la URL no puede contener credenciales embebidas")
	}
	if len(allowedOrigins) > 0 && !AllowedOrigin(parsed, allowedOrigins) {
		return nil, errors.New("la URL debe pertenecer al origen permitido de Zajuna")
	}
	if !allowPrivate && isPrivateHost(parsed.Hostname()) {
		return nil, errors.New("la URL apunta a una red local o privada no permitida")
	}
	return parsed, nil
}

func AllowedOrigin(target *url.URL, origins []string) bool {
	if target == nil {
		return false
	}
	for _, raw := range origins {
		origin, err := url.Parse(strings.TrimRight(strings.TrimSpace(raw), "/"))
		if err != nil || origin.Scheme == "" || origin.Host == "" {
			continue
		}
		if strings.EqualFold(target.Scheme, origin.Scheme) && strings.EqualFold(target.Host, origin.Host) {
			return true
		}
	}
	return false
}

func IsPrivateIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsInterfaceLocalMulticast()
}

func isPrivateHost(host string) bool {
	if ip := net.ParseIP(host); ip != nil {
		return IsPrivateIP(ip)
	}
	// Always resolve, even for an allowlisted host: an allowed public hostname
	// could still DNS-rebind to a private address, and a generic worker must
	// never be able to resolve into private infrastructure either way.
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return true
	}
	for _, ip := range ips {
		if IsPrivateIP(ip) {
			return true
		}
	}
	return false
}

// SortedKeys is useful for deterministic, redacted diagnostics and tests.
func SortedKeys(values url.Values) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
