package zajuna

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/zajuna-app/core/internal/coursemaps"
	"github.com/zajuna-app/core/internal/security"
)

const (
	defaultMapMaxDepth    = 2
	defaultMapMaxPages    = 80
	defaultMapMaxLinks    = 300
	maxMapMaxDepth        = 6
	maxMapMaxPages        = 500
	maxMapMaxLinksPerPage = 1000
)

// CrawlOptions protects the local worker from unexpectedly large course
// graphs while still allowing a larger discovery run when explicitly asked.
type CrawlOptions struct {
	MaxDepth        int
	MaxPages        int
	MaxLinksPerPage int
}

type crawlNode struct {
	URL       string
	Depth     int
	SourceURL string
}

var anchorHrefPattern = regexp.MustCompile(`(?is)<a\b[^>]*\bhref\s*=\s*["']([^"']+)["'][^>]*>(.*?)</a\s*>`)
var optionValuePattern = regexp.MustCompile(`(?is)<option\b[^>]*\bvalue\s*=\s*["']([^"']+)["'][^>]*>(.*?)</option\s*>`)

// DiscoverCourseMap crawls same-origin course pages using the authenticated
// HTTP session and stores only normalized route metadata in its result. It
// does not execute JavaScript and deliberately does not follow external URLs.
func (c *Client) DiscoverCourseMap(ctx context.Context, session Session, courseID string, options CrawlOptions) (coursemaps.Record, error) {
	if session.Client == nil {
		return coursemaps.Record{}, ErrSessionExpired
	}
	if !isNumericID(courseID) {
		return coursemaps.Record{}, errors.New("el ID del curso debe ser numérico")
	}
	options = normalizeCrawlOptions(options)

	courseURL := c.baseURL + "/zajuna/course/view.php?id=" + url.QueryEscape(courseID)
	queue := []crawlNode{{URL: courseURL, Depth: 0}}
	visitedPages := map[string]bool{}
	routeIndex := map[string]coursemaps.Route{}
	routeOrder := make([]string, 0, options.MaxLinksPerPage)
	mergeDiscoveredRoute := func(route coursemaps.Route) {
		if _, exists := routeIndex[route.URL]; !exists {
			routeOrder = append(routeOrder, route.URL)
		}
		mergeRoute(routeIndex, route)
	}
	warning := ""
	truncated := false
	foreignPages := map[string]bool{}
	pagesVisited := 0
	sessionUserID := ""
	restricted := map[string]bool{}

	for len(queue) > 0 && pagesVisited < options.MaxPages {
		if err := ctx.Err(); err != nil {
			return coursemaps.Record{}, err
		}
		node := queue[0]
		queue = queue[1:]
		if visitedPages[node.URL] {
			continue
		}
		visitedPages[node.URL] = true

		body, err := c.doGet(ctx, session, node.URL, node.SourceURL)
		if err != nil {
			var statusErr *HTTPStatusError
			if errors.As(err, &statusErr) && (statusErr.StatusCode == 403 || statusErr.StatusCode == 404) {
				warning = firstNonEmpty(warning, "algunas rutas internas no están disponibles para esta sesión")
				continue
			}
			return coursemaps.Record{}, err
		}
		if looksLikeLoginPage(body) {
			return coursemaps.Record{}, fmt.Errorf("%w: la sesión venció durante el descubrimiento", ErrSessionExpired)
		}
		pagesVisited++
		if node.URL == courseURL {
			sessionUserID = parseSessionUserID(body)
		}
		if classifyRoute(node.URL) == "forum" && forumAccessDenied(body) {
			// Moodle redirected to the course page: its links are not the
			// forum's and the forum itself is unusable as evidence.
			restricted[node.URL] = true
			continue
		}
		if owner, known := pageCourseID(body); known && owner != courseID {
			// Moodle stamps every page with its course (body class
			// course-<id>). A followed link that lands in another course
			// (site news, a shared activity) is dropped with all its links.
			foreignPages[node.URL] = true
			continue
		}

		matches := anchorHrefPattern.FindAllStringSubmatch(body, -1)
		if len(matches) > options.MaxLinksPerPage {
			matches = matches[:options.MaxLinksPerPage]
			warning = "se alcanzó el límite de enlaces por página"
			truncated = true
		}
		for _, match := range matches {
			target, ok := normalizeInternalURL(node.URL, match[1], c.baseURL)
			if !ok {
				continue
			}
			if target == node.URL || belongsToOtherCourse(target, courseID) {
				continue
			}
			kind := classifyRoute(target)
			route := coursemaps.Route{
				URL:       target,
				Kind:      kind,
				Title:     cleanText(match[2]),
				Depth:     node.Depth + 1,
				SourceURL: node.URL,
			}
			if kind == "forum" || kind == "assign" || kind == "grading" {
				route.Technical = coursemaps.IsTechnicalActivity(route.Title)
			}
			// A link without text (icon, image) keeps an empty title: the
			// source page's <title> describes that page, not the target, and
			// used to make icon links match forum/page title pools.
			mergeDiscoveredRoute(route)

			if node.Depth < options.MaxDepth && isCourseMapFollowCandidate(target, kind, courseID) && !visitedPages[target] {
				queue = append(queue, crawlNode{URL: target, Depth: node.Depth + 1, SourceURL: node.URL})
			}
		}
		// Moodle's "Ir a..." selector exposes activity URLs in option values,
		// often with clearer labels than the visible anchor list. Include those
		// entries in the same normalized route map and deduplicate by URL.
		for _, match := range optionValuePattern.FindAllStringSubmatch(body, -1) {
			target, ok := normalizeInternalURL(node.URL, match[1], c.baseURL)
			if !ok || target == node.URL || belongsToOtherCourse(target, courseID) {
				continue
			}
			route := coursemaps.Route{
				URL: target, Kind: classifyRoute(target), Title: cleanText(match[2]),
				Depth: node.Depth + 1, SourceURL: node.URL,
			}
			if route.Kind == "forum" || route.Kind == "assign" || route.Kind == "grading" {
				route.Technical = coursemaps.IsTechnicalActivity(route.Title)
			}
			mergeDiscoveredRoute(route)
			if node.Depth < options.MaxDepth && isCourseMapFollowCandidate(target, route.Kind, courseID) && !visitedPages[target] {
				queue = append(queue, crawlNode{URL: target, Depth: node.Depth + 1, SourceURL: node.URL})
			}
		}
		if node.URL == courseURL {
			for _, route := range extractCourseStructure(body, courseID, c.baseURL, node.URL) {
				mergeDiscoveredRoute(route)
				if node.Depth < options.MaxDepth && isCourseMapFollowCandidate(route.URL, route.Kind, courseID) && !visitedPages[route.URL] {
					queue = append(queue, crawlNode{URL: route.URL, Depth: route.Depth, SourceURL: node.URL})
				}
			}
		}
	}
	if len(queue) > 0 {
		warning = firstNonEmpty(warning, "se alcanzó el límite de páginas del mapa")
		truncated = true
	}
	if pagesVisited == 0 {
		return coursemaps.Record{}, errors.New("no se pudo descubrir ninguna página del curso")
	}

	routes := make([]coursemaps.Route, 0, len(routeOrder))
	for _, target := range routeOrder {
		if foreignPages[target] {
			continue
		}
		route := routeIndex[target]
		if restricted[target] {
			route.Restricted = true
		}
		routes = append(routes, route)
	}

	// Without an id, profile.php shows whoever owns the capture session; with
	// the id read from the course page the evidence is pinned to that user.
	profileURL := c.baseURL + "/zajuna/user/profile.php"
	if sessionUserID != "" {
		profileURL += "?id=" + url.QueryEscape(sessionUserID)
	}
	byItemCode, stats := groupRoutesForCourseMap(routes, courseID, profileURL, truncated)
	now := time.Now().UTC()
	return coursemaps.Record{
		CourseID:      courseID,
		CourseURL:     security.RedactURL(courseURL),
		ProfileURL:    security.RedactURL(profileURL),
		ByItemCode:    byItemCode,
		Routes:        routes,
		LinkCount:     len(routes),
		ItemCodeCount: len(byItemCode),
		ScrapeStats:   stats,
		Warning:       warning,
		Source:        "discover-local-http",
		DiscoveredAt:  now,
		UpdatedAt:     now,
	}, nil
}

// sessionUserIDPatterns read the authenticated user's id from a Moodle page:
// M.cfg.userId (Moodle 4) or the notification popover of the user menu. Links
// to other users' profiles (teachers, forum authors) are never used.
var sessionUserIDPatterns = []*regexp.Regexp{
	regexp.MustCompile(`M\.cfg\s*=\s*\{[^;]*?"userId"\s*:\s*"?(\d+)`),
	regexp.MustCompile(`(?is)<[^>]+id=["']nav-notification-popover-container["'][^>]*\bdata-userid=["'](\d+)["']`),
}

func parseSessionUserID(body string) string {
	for _, pattern := range sessionUserIDPatterns {
		match := pattern.FindStringSubmatch(body)
		// 0 is "not logged in" and 1 is Moodle's guest user.
		if len(match) == 2 && match[1] != "0" && match[1] != "1" {
			return match[1]
		}
	}
	return ""
}

// forumAccessDeniedMarkers are Moodle's "noviewdiscussionspermission" notice
// (shown after redirecting away from the forum) in Spanish and English.
var forumAccessDeniedMarkers = []string{
	"no dispone de permiso para ver los debates",
	"no tiene permiso para ver los debates",
	"do not have the permission to view discussions",
	"do not have permission to view discussions",
}

func forumAccessDenied(body string) bool {
	lower := strings.ToLower(body)
	for _, marker := range forumAccessDeniedMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func normalizeCrawlOptions(options CrawlOptions) CrawlOptions {
	if options.MaxDepth <= 0 {
		options.MaxDepth = defaultMapMaxDepth
	}
	if options.MaxPages <= 0 {
		options.MaxPages = defaultMapMaxPages
	}
	if options.MaxLinksPerPage <= 0 {
		options.MaxLinksPerPage = defaultMapMaxLinks
	}
	if options.MaxDepth > maxMapMaxDepth {
		options.MaxDepth = maxMapMaxDepth
	}
	if options.MaxPages > maxMapMaxPages {
		options.MaxPages = maxMapMaxPages
	}
	if options.MaxLinksPerPage > maxMapMaxLinksPerPage {
		options.MaxLinksPerPage = maxMapMaxLinksPerPage
	}
	return options
}

func normalizeInternalURL(rawBase, rawTarget, configuredBase string) (string, bool) {
	target := strings.TrimSpace(html.UnescapeString(rawTarget))
	if target == "" || strings.HasPrefix(target, "#") {
		return "", false
	}
	parsed, err := url.Parse(target)
	if err != nil || parsed.Scheme == "mailto" || parsed.Scheme == "tel" || parsed.Scheme == "javascript" {
		return "", false
	}
	base, err := url.Parse(rawBase)
	if err != nil {
		return "", false
	}
	resolved := base.ResolveReference(parsed)
	configured, err := url.Parse(configuredBase)
	if err != nil || resolved.Host != configured.Host || resolved.Scheme != configured.Scheme {
		return "", false
	}
	if !strings.HasPrefix(resolved.Path, "/zajuna/") {
		return "", false
	}
	lowerPath := strings.ToLower(resolved.Path)
	if strings.Contains(lowerPath, "/login/") || strings.Contains(lowerPath, "login_user") {
		return "", false
	}
	resolved.Fragment = ""
	return security.RedactURL(resolved.String()), true
}

func classifyRoute(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "route"
	}
	value := strings.ToLower(parsed.Path + "?" + parsed.RawQuery)
	switch {
	case strings.Contains(value, "action=grader"), strings.Contains(value, "action=grading"):
		return "grading"
	case strings.Contains(value, "section=") && strings.Contains(value, "/course/view.php"):
		return "phase"
	case strings.Contains(value, "/mod/forum/"):
		return "forum"
	case strings.Contains(value, "/mod/page/"):
		return "page"
	case strings.Contains(value, "/mod/assign/"):
		return "assign"
	case strings.Contains(value, "/mod/resource/"), strings.Contains(value, "pluginfile.php"), hasResourceExtension(parsed.Path):
		return "resource"
	case strings.Contains(value, "/mod/url/"):
		return "url"
	case strings.Contains(value, "/course/view.php"):
		return "course"
	case strings.Contains(value, "/user/profile.php"):
		return "profile"
	default:
		return "route"
	}
}

func isHTMLCandidate(rawURL, kind string) bool {
	if kind == "resource" {
		return false
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Path == "" || hasResourceExtension(parsed.Path) {
		return false
	}
	path := strings.ToLower(parsed.Path)
	return strings.Contains(path, "/zajuna/course/") || strings.Contains(path, "/zajuna/mod/")
}

func isCourseMapFollowCandidate(rawURL, kind, courseID string) bool {
	if !isHTMLCandidate(rawURL, kind) {
		return false
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	path := strings.ToLower(parsed.Path)
	if strings.Contains(path, "/login/") || strings.Contains(path, "/admin/") || strings.Contains(path, "/enrol/") {
		return false
	}
	query := parsed.Query()
	if strings.Contains(path, "/course/") {
		id := query.Get("id")
		return id == courseID
	}
	if strings.Contains(path, "/grade/") {
		id := query.Get("id")
		return id == "" || id == courseID
	}
	if course := query.Get("course"); course != "" && course != courseID {
		return false
	}
	return true
}

func fallbackRouteTitle(rawURL, kind string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Path == "" {
		return kind
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	last := parts[len(parts)-1]
	if last == "" && len(parts) > 1 {
		last = parts[len(parts)-2]
	}
	if id := parsed.Query().Get("id"); id != "" {
		return strings.TrimSpace(kind + " " + last + " " + id)
	}
	return strings.TrimSpace(kind + " " + last)
}

func hasResourceExtension(path string) bool {
	lower := strings.ToLower(path)
	for _, extension := range []string{".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".zip", ".rar", ".7z", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp", ".mp4", ".mp3"} {
		if strings.HasSuffix(lower, extension) {
			return true
		}
	}
	return false
}

func groupRoutes(routes []coursemaps.Route) (map[string]json.RawMessage, coursemaps.Stats) {
	return groupRoutesForCourse(routes, "", "")
}

func groupRoutesForCourse(routes []coursemaps.Route, courseID, profileURL string) (map[string]json.RawMessage, coursemaps.Stats) {
	return groupRoutesForCourseMap(routes, courseID, profileURL, false)
}

// groupRoutesForCourseMap projects routes into checklist items. truncated
// marks a crawl that hit its page/link limits: a title-resolved item that
// found no match may simply not have been crawled, so it stays empty instead
// of falling back to every route of the same kind.
func groupRoutesForCourseMap(routes []coursemaps.Route, courseID, profileURL string, truncated bool) (map[string]json.RawMessage, coursemaps.Stats) {
	groups := map[string][]string{}
	stats := coursemaps.Stats{}
	for _, route := range routes {
		groups[route.Kind] = append(groups[route.Kind], route.URL)
		switch route.Kind {
		case "phase":
			stats.Phases++
		case "forum":
			stats.Forums++
		case "page":
			stats.Pages++
		case "assign":
			stats.Assigns++
		case "grading":
			stats.Gradings++
		case "url":
			stats.URLs++
		case "resource":
			stats.Resources++
		}
	}
	stats.Total = len(routes)
	result := make(map[string]json.RawMessage, len(groups))
	for kind, values := range groups {
		encoded, _ := json.Marshal(values)
		result["route."+kind] = encoded
	}
	for itemCode, values := range checklistRouteGroups(routes) {
		encoded, _ := json.Marshal(values)
		result[itemCode] = encoded
	}
	for itemCode, values := range buildExactChecklistRouteGroupsForMap(routes, courseID, profileURL, truncated) {
		encoded, _ := json.Marshal(values)
		result[itemCode] = encoded
	}
	return result, stats
}

// checklistRouteGroups projects the generic crawl into the checklist contract.
// Generic route.* groups remain in the map for diagnostics and future rules.
func checklistRouteGroups(routes []coursemaps.Route) map[string][]string {
	rules := map[string][]string{
		"1.1.1": {"page"}, "1.1.2": {"page"}, "1.1.3": {"page"}, "1.1.4": {"page"}, "1.1.5": {"page"},
		"1.2.1": {"phase"}, "1.2.2": {"phase"}, "1.2.3": {"phase"}, "1.2.4": {"phase"}, "1.2.5": {"phase"}, "1.2.6": {"phase"}, "1.2.7": {"phase"},
		"2.1.1": {"profile"}, "2.1.2": {"profile"}, "2.1.3": {"profile"}, "2.1.4": {"profile"}, "2.1.5": {"profile"},
		"3.1": {"page", "resource", "url"}, "4.1": {"course"}, "5.1": {"grading"}, "6.1": {"page", "assign"},
		"7.1.1": {"page", "course", "phase"}, "7.1.2": {"page", "course", "phase"}, "7.2": {"page", "course", "phase"},
		"7.3.1": {"page", "course"}, "7.3.2": {"page", "course"}, "7.3.3": {"page", "course"},
		"7.4.1": {"page", "course"}, "7.4.2": {"page", "course"}, "7.4.3": {"page", "course"}, "7.4.4": {"page", "course"},
		"8.1": {"page", "course"}, "8.2": {"page", "course"}, "8.3": {"page", "course"},
		"9.1.1": {"forum"}, "9.1.2": {"forum"}, "9.1.3": {"forum"}, "9.1.4": {"forum"}, "9.1.5": {"forum"}, "9.1.6": {"forum"}, "9.1.7": {"forum"},
		"10.1.1": {"assign", "grading"}, "10.1.2": {"assign", "grading"},
		"11.1.1": {"forum"}, "11.1.2": {"forum"}, "11.1.3": {"forum"}, "11.1.4": {"forum"},
		"11.2.1": {"forum"}, "11.2.2": {"forum"}, "11.2.3": {"forum"}, "11.3": {"forum"}, "11.4": {"forum"},
		"12.1.1": {"page", "resource", "url"}, "12.1.2": {"page", "resource", "url"},
		"13.1.1": {"page", "course"}, "13.1.2": {"page", "course"}, "13.1.3": {"page", "course"}, "13.2.1": {"page", "course"}, "13.2.2": {"page", "course"},
		"14.1.1": {"forum"}, "14.1.2": {"forum"}, "15.1": {"forum"},
	}
	groups := make(map[string][]string, len(rules))
	for itemCode, kinds := range rules {
		seen := map[string]bool{}
		for _, route := range routes {
			if route.Restricted {
				continue
			}
			for _, kind := range kinds {
				if route.Kind == kind && !seen[route.URL] {
					groups[itemCode] = append(groups[itemCode], route.URL)
					seen[route.URL] = true
					break
				}
			}
		}
	}
	return groups
}

func mergeRoute(routes map[string]coursemaps.Route, candidate coursemaps.Route) {
	if existing, found := routes[candidate.URL]; found {
		if candidate.Title != "" {
			existing.Title = candidate.Title
		}
		if candidate.SourceURL != "" {
			existing.SourceURL = candidate.SourceURL
		}
		if candidate.PhaseName != "" {
			existing.PhaseName = candidate.PhaseName
		}
		if candidate.PhaseSection != 0 {
			existing.PhaseSection = candidate.PhaseSection
		}
		if candidate.ActivityID != "" {
			existing.ActivityID = candidate.ActivityID
		}
		if candidate.Subsection != "" {
			existing.Subsection = candidate.Subsection
		}
		if candidate.Technical {
			existing.Technical = true
		}
		if candidate.Restricted {
			existing.Restricted = true
		}
		if candidate.Depth < existing.Depth {
			existing.Depth = candidate.Depth
		}
		if existing.Kind == "route" && candidate.Kind != "route" {
			existing.Kind = candidate.Kind
		}
		routes[candidate.URL] = existing
		return
	}
	routes[candidate.URL] = candidate
}

var bodyCourseClassPattern = regexp.MustCompile(`(?is)<body\b[^>]*\bclass\s*=\s*["'][^"']*\bcourse-(\d+)\b`)

// pageCourseID reads the course Moodle renders a page for. known is false
// when the page does not declare it (then membership is not verifiable).
func pageCourseID(body string) (string, bool) {
	match := bodyCourseClassPattern.FindStringSubmatch(body)
	if len(match) != 2 {
		return "", false
	}
	return match[1], true
}

// routeCourseID extracts the course a URL is explicitly scoped to, for the
// Moodle routes whose parameters name it. Activity URLs (mod/*/view.php?id=)
// carry a module id instead, so they are not verifiable from the URL alone.
func routeCourseID(rawURL string) (string, bool) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", false
	}
	query := parsed.Query()
	for _, key := range []string{"courseid", "course"} {
		if value := strings.TrimSpace(query.Get(key)); isNumericID(value) {
			return value, true
		}
	}
	path := strings.ToLower(parsed.Path)
	id := strings.TrimSpace(query.Get("id"))
	if !isNumericID(id) {
		return "", false
	}
	switch {
	case strings.HasSuffix(path, "/course/view.php"),
		strings.HasSuffix(path, "/user/index.php"),
		strings.Contains(path, "/grade/report/") && strings.HasSuffix(path, "/index.php"),
		strings.Contains(path, "/mod/") && strings.HasSuffix(path, "/index.php"):
		return id, true
	}
	return "", false
}

func belongsToOtherCourse(rawURL, courseID string) bool {
	if strings.TrimSpace(courseID) == "" {
		return false
	}
	owner, known := routeCourseID(rawURL)
	return known && owner != courseID
}

func isNumericID(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}
