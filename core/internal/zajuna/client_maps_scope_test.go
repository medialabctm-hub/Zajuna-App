package zajuna

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zajuna-app/core/internal/coursemaps"
)

func discoverFixture(t *testing.T, pages map[string]string, options CrawlOptions) (coursemaps.Record, string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := pages[r.URL.RequestURI()]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	client, err := newClient(server.URL, true)
	if err != nil {
		t.Fatal(err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	record, err := client.DiscoverCourseMap(context.Background(), Session{Client: &http.Client{Jar: jar}, BaseURL: server.URL}, "41080", options)
	if err != nil {
		t.Fatal(err)
	}
	return record, server.URL
}

func mappedValues(t *testing.T, record coursemaps.Record, itemCode string) []string {
	t.Helper()
	var values []string
	if raw, ok := record.ByItemCode[itemCode]; ok {
		if err := json.Unmarshal(raw, &values); err != nil {
			t.Fatalf("item %s map is not JSON: %v", itemCode, err)
		}
	}
	return values
}

func routeByURLSuffix(record coursemaps.Record, suffix string) (coursemaps.Route, bool) {
	for _, route := range record.Routes {
		if strings.HasSuffix(route.URL, suffix) {
			return route, true
		}
	}
	return coursemaps.Route{}, false
}

func TestDiscoverCourseMapKeepsTextlessLinksUntitled(t *testing.T) {
	record, _ := discoverFixture(t, map[string]string{
		"/zajuna/course/view.php?id=41080": `<html><head><title>Anuncios</title></head><body class="course-41080">
            <a href="/zajuna/mod/forum/view.php?id=5"><img src="/icon.svg" alt=""></a>
          </body></html>`,
	}, CrawlOptions{MaxDepth: 1, MaxPages: 1, MaxLinksPerPage: 20})
	route, ok := routeByURLSuffix(record, "/zajuna/mod/forum/view.php?id=5")
	if !ok {
		t.Fatalf("icon link was not recorded: %#v", record.Routes)
	}
	if route.Title != "" {
		t.Fatalf("a textless link must not inherit the source page title, got %q", route.Title)
	}
	if values := mappedValues(t, record, "11.1.1"); len(values) != 0 {
		t.Fatalf("an untitled forum must not resolve as the announcements forum: %v", values)
	}
}

func TestDiscoverCourseMapDropsActivitiesOfOtherCourses(t *testing.T) {
	record, _ := discoverFixture(t, map[string]string{
		"/zajuna/course/view.php?id=41080": `<html><body class="format-topics course-41080">
            <a href="/zajuna/course/view.php?id=999">Otro curso</a>
            <a href="/zajuna/mod/forum/index.php?id=999">Foros de otro curso</a>
            <a href="/zajuna/grade/report/grader/index.php?id=999">Calificaciones de otro curso</a>
            <a href="/zajuna/mod/page/view.php?id=50">Cronograma General</a>
            <a href="/zajuna/mod/page/view.php?id=51">Guía de la fase</a>
          </body></html>`,
		"/zajuna/mod/page/view.php?id=50": `<html><body class="path-mod course-999 context-1">
            <a href="/zajuna/mod/forum/view.php?id=60">Foro temático ajeno</a>
          </body></html>`,
		"/zajuna/mod/page/view.php?id=51": `<html><body class="path-mod course-41080 context-2">
            <a href="/zajuna/mod/page/view.php?id=52">Recurso propio</a>
          </body></html>`,
	}, CrawlOptions{MaxDepth: 2, MaxPages: 10, MaxLinksPerPage: 50})
	for _, foreign := range []string{"course/view.php?id=999", "mod/forum/index.php?id=999", "grader/index.php?id=999", "mod/page/view.php?id=50", "mod/forum/view.php?id=60"} {
		if route, ok := routeByURLSuffix(record, foreign); ok {
			t.Fatalf("route of another course leaked into the map: %#v", route)
		}
	}
	for _, own := range []string{"mod/page/view.php?id=51", "mod/page/view.php?id=52"} {
		if _, ok := routeByURLSuffix(record, own); !ok {
			t.Fatalf("own course route %s is missing: %#v", own, record.Routes)
		}
	}
	for _, value := range mappedValues(t, record, "1.1.1") {
		if strings.Contains(value, "id=50") {
			t.Fatalf("another course's cronograma was assigned: %v", value)
		}
	}
}

func TestRouteCourseIDOnlyTrustsCourseScopedURLs(t *testing.T) {
	for raw, want := range map[string]string{
		"https://z/zajuna/course/view.php?id=7&section=2":     "7",
		"https://z/zajuna/mod/forum/index.php?id=7":           "7",
		"https://z/zajuna/grade/report/grader/index.php?id=7": "7",
		"https://z/zajuna/user/view.php?id=3&course=7":        "7",
		"https://z/zajuna/calendar/view.php?courseid=7":       "7",
	} {
		if got, ok := routeCourseID(raw); !ok || got != want {
			t.Fatalf("routeCourseID(%s) = %q,%v want %q", raw, got, ok, want)
		}
	}
	if _, ok := routeCourseID("https://z/zajuna/mod/forum/view.php?id=7"); ok {
		t.Fatal("an activity id is a module id, not a course id")
	}
	if belongsToOtherCourse("https://z/zajuna/course/view.php?id=41080", "41080") {
		t.Fatal("the crawled course is not foreign")
	}
}

func TestResolverDoesNotConfuseAnuncioWithAnuncios(t *testing.T) {
	routes := []coursemaps.Route{
		{Kind: "forum", URL: "https://zajuna.sena.edu.co/zajuna/mod/forum/view.php?id=1", Title: "Anuncios"},
		{Kind: "forum", URL: "https://zajuna.sena.edu.co/zajuna/mod/forum/view.php?id=2", Title: "Anuncio de inicio de fase"},
		{Kind: "forum", URL: "https://zajuna.sena.edu.co/zajuna/mod/forum/view.php?id=3", Title: "Foro temático fase 1"},
	}
	groups := buildExactChecklistRouteGroups(routes, "41080", "")
	// 11.4 ("Anuncios con función comunicativa") is proven by the
	// announcement forums: the general "Anuncios" forum (as in the audited
	// course, see docs/api-local.md) and single announcements, never the
	// thematic forum. "anuncio" still never matches inside "anuncios" (below).
	got := groups["11.4"]
	if len(got) != 2 || !strings.Contains(strings.Join(got, " "), "id=1") || !strings.Contains(strings.Join(got, " "), "id=2") {
		t.Fatalf("11.4 must use the announcement forums, got %v", got)
	}
	if strings.Contains(strings.Join(got, " "), "id=3") {
		t.Fatalf("11.4 must not use the thematic forum, got %v", got)
	}
	if got := groups["11.1.1"]; len(got) != 1 || !strings.Contains(got[0], "id=1") {
		t.Fatalf("11.1.1 (anuncios) must use the announcements forum, got %v", got)
	}
	if !containsWords(normalizeResolverText("GA1-220501096-AA1-EV01"), "ga1") {
		t.Fatal("activity codes must still match on punctuation boundaries")
	}
	if containsWords(normalizeResolverText("Anuncios"), "anuncio") {
		t.Fatal("singular term must not match the plural word")
	}
}

func TestTruncatedMapDoesNotGuessTheCronograma(t *testing.T) {
	routes := []coursemaps.Route{{Kind: "page", URL: "https://zajuna.sena.edu.co/zajuna/mod/page/view.php?id=9", Title: "Bienvenida"}}
	truncated, _ := groupRoutesForCourseMap(routes, "41080", "", true)
	var values []string
	if err := json.Unmarshal(truncated["1.1.1"], &values); err != nil || len(values) != 0 {
		t.Fatalf("a truncated map must leave 1.1.1 empty, got %v (%v)", values, err)
	}

	record, _ := discoverFixture(t, map[string]string{
		"/zajuna/course/view.php?id=41080": `<html><body class="course-41080">
            <a href="/zajuna/mod/page/view.php?id=9">Bienvenida</a>
            <a href="/zajuna/mod/page/view.php?id=10">Otra página</a>
          </body></html>`,
	}, CrawlOptions{MaxDepth: 2, MaxPages: 1, MaxLinksPerPage: 20})
	if record.Warning == "" {
		t.Fatal("the page limit must be reported")
	}
	if values := mappedValues(t, record, "1.1.1"); len(values) != 0 {
		t.Fatalf("a truncated crawl must not assign an arbitrary page as cronograma: %v", values)
	}
}
