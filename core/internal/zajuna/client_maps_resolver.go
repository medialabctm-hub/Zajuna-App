package zajuna

import (
	"net/url"
	"strings"
	"unicode"

	"github.com/zajuna-app/core/internal/coursemaps"
)

// The resolver mirrors the proven activity pools from zajuna-sync. Generic
// route groups remain available for diagnostics, while these rules provide
// exact URLs for itemCode/slot whenever the activity title is sufficiently
// descriptive.
var forumResolveModes = map[string]string{
	"9.1.1": "dudas_singleton", "9.1.2": "dudas_singleton", "9.1.5": "dudas_singleton",
	"9.1.3": "tematico_slot", "9.1.4": "tematico_slot", "9.1.6": "tematico_slot", "9.1.7": "tematico_slot",
	"11.1.1": "anuncios_singleton", "11.1.2": "anuncios_singleton", "11.1.3": "anuncios_singleton", "11.1.4": "anuncios_singleton",
	"11.2.1": "anuncios_singleton", "11.2.2": "anuncios_singleton", "11.2.3": "anuncios_singleton",
	"11.3": "anuncios_singleton", "11.4": "anuncio_slot",
	"14.1.1": "tematico_slot", "14.1.2": "tematico_slot", "15.1": "netiqueta_singleton",
}

var forumPoolTerms = map[string][]string{
	"dudas_singleton":    {"foro de dudas", "dudas o inquietudes", "dudas e inquietudes"},
	"tematico_slot":      {"foro temático", "foro tematico", "temático", "tematico"},
	"anuncios_singleton": {"anuncios"},
	"anuncio_slot":       {"anuncios", "anuncio", "comunicativa", "aprendices aprobados"},
	"sesion_slot":        {"sesión en línea", "sesion en linea", "sesión sincrónica", "grabación sesión"},
	// MDL-153: this mode used to be "induccion_singleton" searching for
	// induction/onboarding terms — unrelated to item 15.1 ("Lenguaje cortés y
	// respetuoso con uso de netiqueta"). It resolved to the general student
	// induction forum every time, which is why the owner filter never found
	// an instructor post there. "netiqueta" is the item's own distinctive
	// checklist term and the most likely literal match on a real course.
	"netiqueta_singleton": {"netiqueta", "buena ortografía", "buena ortografia", "lenguaje cortés"},
}

var pagePoolTerms = map[string][]string{
	"cronograma_general_singleton": {"cronograma general", "cronograma  general"},
	"fase_page_slot":               {"cronograma fase", "cronograma  fase", "fase análisis", "fase analisis", "fase hacer", "fase verificar"},
	"grabacion_slot":               {"grabación", "grabacion", "resumen sesión", "resumen sesion", "sesión en línea", "sesion en linea"},
	"assign_slot":                  {"evidencia", "ga1-", "ga2-", "ga3-"},
}

func buildExactChecklistRouteGroups(routes []coursemaps.Route, courseID, profileURL string) map[string][]string {
	return buildExactChecklistRouteGroupsForMap(routes, courseID, profileURL, false)
}

func buildExactChecklistRouteGroupsForMap(routes []coursemaps.Route, courseID, profileURL string, truncated bool) map[string][]string {
	groups := make(map[string][]string)
	origin := "https://zajuna.sena.edu.co"
	if parsed, err := url.Parse(profileURL); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		origin = parsed.Scheme + "://" + parsed.Host
	}
	put := func(codes []string, values []string) {
		if len(values) == 0 {
			return
		}
		for _, code := range codes {
			groups[code] = append([]string(nil), values...)
		}
	}

	cronograma := pickSingletonRoute(routes, []string{"page", "resource"}, pagePoolTerms["cronograma_general_singleton"], 6, nil)
	put([]string{"1.1.1", "1.1.2", "1.1.3", "1.1.4", "1.1.5"}, singleValue(cronograma))
	if cronograma == nil && truncated {
		// The general schedule page may be past the crawl limit; any other
		// page of the course would be a false cronograma.
		for _, code := range []string{"1.1.1", "1.1.2", "1.1.3", "1.1.4", "1.1.5"} {
			groups[code] = []string{}
		}
	}

	fases := buildOrderedRoutePool(routes, []string{"page", "resource"}, pagePoolTerms["fase_page_slot"], 6, func(route coursemaps.Route) bool {
		text := resolverText(route)
		return strings.Contains(text, "fase") || strings.Contains(text, "cronograma")
	})
	put([]string{"1.2.1", "1.2.2", "1.2.3", "1.2.4", "1.2.5", "1.2.6", "1.2.7"}, fases)

	// No Technical filter here: discovery marks every forum without a
	// transversal competency code as Technical (Anuncios, Dudas, Foro
	// Temático included), so filtering on it emptied every forum item. Only
	// forums whose title matches the item's terms enter a pool anyway.
	var forumFilter func(coursemaps.Route) bool
	forumPools := make(map[string][]string, len(forumPoolTerms))
	for mode, terms := range forumPoolTerms {
		if strings.HasSuffix(mode, "_singleton") {
			forumPools[mode] = singleValue(pickSingletonRoute(routes, []string{"forum"}, terms, 8, forumFilter))
		} else {
			forumPools[mode] = buildOrderedRoutePool(routes, []string{"forum"}, terms, 8, forumFilter)
		}
	}
	for itemCode, mode := range forumResolveModes {
		// Forum items are identified only by their forum's title. Without a
		// title match the item stays explicitly empty: the generic projection
		// (every forum of the course) assigned e.g. the thematic forum or the
		// general "Anuncios" forum to unrelated announcement items.
		groups[itemCode] = append([]string{}, forumPools[mode]...)
	}
	if len(groups["15.1"]) == 0 {
		// 15.1 (lenguaje cortés y netiqueta) is shown in the instructor's own
		// posts. Without a forum named for it, the announcements forum is its
		// evidence (as in the audited course), never every forum of the course.
		groups["15.1"] = append([]string{}, forumPools["anuncios_singleton"]...)
	}

	assigns := buildOrderedRoutePool(routes, []string{"assign"}, pagePoolTerms["assign_slot"], 4, nil)
	if len(assigns) == 0 {
		assigns = orderedRoutesByKind(routes, []string{"assign"})
	}
	put([]string{"10.1.1", "10.1.2"}, assigns)

	grabaciones := buildOrderedRoutePool(routes, []string{"page", "resource", "url", "route"}, pagePoolTerms["grabacion_slot"], 6, func(route coursemaps.Route) bool {
		if strings.Contains(strings.ToLower(route.URL), "/mod/plugnmeet/") {
			return true
		}
		// The recording/summary term must be in the activity's own title.
		// Matching only the surrounding section text picked unrelated
		// Inducción pages ("Actualización de los datos personales").
		return route.Kind != "route" && titleHasAnyTerm(route, pagePoolTerms["grabacion_slot"])
	})
	put([]string{"12.1.1", "12.1.2"}, grabaciones)
	if len(grabaciones) == 0 {
		// No recording pages: SENA courses publish recordings inside the
		// per-phase "Grabaciones sesiones en línea" sections of the course
		// page, captured section by section. Without an explicit value the
		// generic kind-based mapping (any page/resource/url) would win and the
		// first Inducción pages became wrong evidence.
		groups["12.1.1"], groups["12.1.2"] = []string{}, []string{}
		if numericCourseID(courseID) {
			coursePage := origin + "/zajuna/course/view.php?id=" + url.QueryEscape(courseID)
			groups["12.1.1"], groups["12.1.2"] = []string{coursePage}, []string{coursePage}
		}
	}

	if numericCourseID(courseID) {
		// 5.1 proves which activities/evidences are associated in the
		// gradebook: its setup page lists them vertically (readable, batched
		// by rows). The grader report had one column per activity (~28000 px).
		groups["5.1"] = []string{origin + "/zajuna/grade/edit/tree/index.php?id=" + url.QueryEscape(courseID)}
		coursePage := origin + "/zajuna/course/view.php?id=" + url.QueryEscape(courseID)
		put([]string{"3.1", "4.1", "6.1", "7.1.1", "7.1.2", "7.2", "7.3.1", "7.3.2", "7.3.3", "7.4.1", "7.4.2", "7.4.3", "7.4.4", "8.1", "8.2", "8.3", "13.1.1", "13.1.2", "13.1.3", "13.2.1", "13.2.2"}, []string{coursePage})
	}
	if strings.TrimSpace(profileURL) != "" {
		put([]string{"2.1.1", "2.1.2", "2.1.3", "2.1.4", "2.1.5"}, []string{strings.TrimSpace(profileURL)})
	}
	return groups
}

type routeMatch struct {
	route coursemaps.Route
	index int
	score int
}

func pickSingletonRoute(routes []coursemaps.Route, kinds, terms []string, minimum int, filter func(coursemaps.Route) bool) *routeMatch {
	matches := matchingRoutes(routes, kinds, terms, minimum, filter)
	if len(matches) == 0 {
		return nil
	}
	best := matches[0]
	for _, match := range matches[1:] {
		if match.score > best.score || (match.score == best.score && match.index < best.index) {
			best = match
		}
	}
	return &best
}

func buildOrderedRoutePool(routes []coursemaps.Route, kinds, terms []string, minimum int, filter func(coursemaps.Route) bool) []string {
	matches := matchingRoutes(routes, kinds, terms, minimum, filter)
	values := make([]string, 0, len(matches))
	seen := map[string]bool{}
	for _, match := range matches {
		value := forceViewURL(match.route.URL)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		values = append(values, value)
	}
	return values
}

func orderedRoutesByKind(routes []coursemaps.Route, kinds []string) []string {
	values := make([]string, 0)
	seen := map[string]bool{}
	for _, route := range routes {
		if route.Restricted || !routeHasKind(route, kinds) {
			continue
		}
		value := forceViewURL(route.URL)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		values = append(values, value)
	}
	return values
}

func matchingRoutes(routes []coursemaps.Route, kinds, terms []string, minimum int, filter func(coursemaps.Route) bool) []routeMatch {
	matches := make([]routeMatch, 0)
	for index, route := range routes {
		if route.Restricted || !routeHasKind(route, kinds) || (filter != nil && !filter(route)) {
			continue
		}
		score := scoreRoute(route, terms)
		if score >= minimum {
			matches = append(matches, routeMatch{route: route, index: index, score: score})
		}
	}
	return matches
}

func routeHasKind(route coursemaps.Route, kinds []string) bool {
	for _, kind := range kinds {
		if route.Kind == kind {
			return true
		}
	}
	return false
}

func scoreRoute(route coursemaps.Route, terms []string) int {
	label := normalizeResolverText(route.Title)
	searchable := resolverText(route)
	score := 0
	for _, term := range terms {
		term = normalizeResolverText(term)
		if term == "" {
			continue
		}
		if label == term {
			score += 50
		} else if containsNormalizedTerm(label, term) {
			score += len([]rune(term)) + 10
		} else if containsNormalizedTerm(searchable, term) {
			score += 4
		}
	}
	return score
}

func containsNormalizedTerm(haystack, term string) bool {
	if term == "" || haystack == "" {
		return false
	}
	if haystack == term {
		return true
	}
	start := 0
	for {
		index := strings.Index(haystack[start:], term)
		if index < 0 {
			return false
		}
		index += start
		left := []rune(haystack[:index])
		right := []rune(haystack[index+len(term):])
		leftOK := len(left) == 0 || !(unicode.IsLetter(left[len(left)-1]) || unicode.IsDigit(left[len(left)-1]))
		rightOK := len(right) == 0 || !(unicode.IsLetter(right[0]) || unicode.IsDigit(right[0]))
		if leftOK && rightOK {
			return true
		}
		start = index + 1
		if start >= len(haystack) {
			return false
		}
	}
}

func resolverText(route coursemaps.Route) string {
	return normalizeResolverText(strings.Join([]string{route.Title, route.PhaseName, route.Subsection, route.URL}, " "))
}

// normalizeResolverText folds case and accents and turns punctuation into
// word separators, so terms are compared as whole words.
func normalizeResolverText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer(
		"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n",
	).Replace(value)
	value = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return ' '
	}, value)
	return strings.Join(strings.Fields(value), " ")
}

// containsWords reports whether term appears in text on word boundaries:
// "anuncio" matches "Anuncio de inicio" but not the general "Anuncios"
// forum, and "fase" does not match "fases".
func containsWords(text, term string) bool {
	if term == "" {
		return false
	}
	return strings.Contains(" "+text+" ", " "+term+" ")
}

func titleHasAnyTerm(route coursemaps.Route, terms []string) bool {
	title := normalizeResolverText(route.Title)
	for _, term := range terms {
		if term = normalizeResolverText(term); term != "" && containsNormalizedTerm(title, term) {
			return true
		}
	}
	return false
}

func singleValue(match *routeMatch) []string {
	if match == nil {
		return nil
	}
	value := forceViewURL(match.route.URL)
	if value == "" {
		return nil
	}
	return []string{value}
}

func forceViewURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" || strings.Contains(strings.ToLower(rawURL), "/user/profile.php") || strings.Contains(strings.ToLower(rawURL), "forceview=1") {
		return rawURL
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return rawURL
	}
	if !strings.Contains(strings.ToLower(parsed.Path), "/mod/") || !strings.HasSuffix(strings.ToLower(parsed.Path), "/view.php") {
		return rawURL
	}
	query := parsed.Query()
	query.Set("forceview", "1")
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func numericCourseID(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}
