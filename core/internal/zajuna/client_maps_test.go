package zajuna

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"

	"github.com/zajuna-app/core/internal/coursemaps"
)

func TestGroupRoutesProjectsGenericKindsIntoChecklistItemCodes(t *testing.T) {
	routes := []coursemaps.Route{
		{URL: "https://zajuna.sena.edu.co/zajuna/mod/page/view.php?id=1", Kind: "page"},
		{URL: "https://zajuna.sena.edu.co/zajuna/course/view.php?id=41080&section=2", Kind: "phase"},
		{URL: "https://zajuna.sena.edu.co/zajuna/user/profile.php?id=7", Kind: "profile"},
		{URL: "https://zajuna.sena.edu.co/zajuna/mod/forum/view.php?id=3", Kind: "forum"},
		{URL: "https://zajuna.sena.edu.co/zajuna/grade/report/grader/index.php?id=41080", Kind: "grading"},
	}
	groups, _ := groupRoutes(routes)
	for itemCode, expectedURL := range map[string]string{
		"1.1.1": routes[0].URL,
		"1.2.1": routes[1].URL,
		"2.1.1": routes[2].URL,
		"5.1":   routes[4].URL,
	} {
		var values []string
		if err := json.Unmarshal(groups[itemCode], &values); err != nil {
			t.Fatalf("item %s map is not JSON: %v", itemCode, err)
		}
		if len(values) != 1 || values[0] != expectedURL {
			t.Fatalf("unexpected projection for %s: %#v", itemCode, values)
		}
	}
	if _, ok := groups["route.page"]; !ok {
		t.Fatal("generic route.page group should remain available")
	}
	// Forum items are title-resolved: an untitled forum is not assigned.
	var forum []string
	if err := json.Unmarshal(groups["9.1.1"], &forum); err != nil || len(forum) != 0 {
		t.Fatalf("an untitled forum must not be projected into 9.1.1: %#v (%v)", forum, err)
	}
}

func TestGroupRoutesUsesTitlesToResolveChecklistActivitiesAndSlots(t *testing.T) {
	routes := []coursemaps.Route{
		{URL: "https://zajuna.sena.edu.co/zajuna/mod/page/view.php?id=10", Kind: "page", Title: "Cronograma General"},
		{URL: "https://zajuna.sena.edu.co/zajuna/mod/page/view.php?id=11", Kind: "page", Title: "Cronograma Fase Análisis"},
		{URL: "https://zajuna.sena.edu.co/zajuna/mod/page/view.php?id=12", Kind: "page", Title: "Cronograma Fase Hacer"},
		{URL: "https://zajuna.sena.edu.co/zajuna/mod/forum/view.php?id=20", Kind: "forum", Title: "Foro de Dudas e Inquietudes"},
		{URL: "https://zajuna.sena.edu.co/zajuna/mod/forum/view.php?id=21", Kind: "forum", Title: "Foro Temático - Fase Análisis"},
		{URL: "https://zajuna.sena.edu.co/zajuna/mod/assign/view.php?id=30", Kind: "assign", Title: "GA1 Evidencia de aprendizaje"},
	}
	groups, _ := groupRoutesForCourse(routes, "41080", "https://zajuna.sena.edu.co/zajuna/user/profile.php")
	assertMappedURL := func(itemCode, expected string) {
		t.Helper()
		var values []string
		if err := json.Unmarshal(groups[itemCode], &values); err != nil {
			t.Fatalf("item %s map is not JSON: %v", itemCode, err)
		}
		if len(values) == 0 || values[0] != expected {
			t.Fatalf("unexpected URL for %s: %#v", itemCode, values)
		}
	}
	assertMappedURL("1.1.1", "https://zajuna.sena.edu.co/zajuna/mod/page/view.php?forceview=1&id=10")
	assertMappedURL("1.2.1", "https://zajuna.sena.edu.co/zajuna/mod/page/view.php?forceview=1&id=11")
	assertMappedURL("9.1.1", "https://zajuna.sena.edu.co/zajuna/mod/forum/view.php?forceview=1&id=20")
	assertMappedURL("9.1.3", "https://zajuna.sena.edu.co/zajuna/mod/forum/view.php?forceview=1&id=21")
	assertMappedURL("10.1.1", "https://zajuna.sena.edu.co/zajuna/mod/assign/view.php?forceview=1&id=30")
	assertMappedURL("5.1", "https://zajuna.sena.edu.co/zajuna/grade/edit/tree/index.php?id=41080")
	assertMappedURL("2.1.1", "https://zajuna.sena.edu.co/zajuna/user/profile.php")
	assertMappedURL("4.1", "https://zajuna.sena.edu.co/zajuna/course/view.php?id=41080")

	var phaseValues []string
	if err := json.Unmarshal(groups["1.2.1"], &phaseValues); err != nil {
		t.Fatal(err)
	}
	if len(phaseValues) != 2 || phaseValues[1] != "https://zajuna.sena.edu.co/zajuna/mod/page/view.php?forceview=1&id=12" {
		t.Fatalf("phase slots were not ordered: %#v", phaseValues)
	}
}

func TestDiscoverCourseMapReadsJumpOptionsWithActivityTitles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/zajuna/course/view.php" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`<html><head><title>Curso fixture</title></head><body>
            <select id="jump-to-activity">
              <option value="/zajuna/mod/page/view.php?id=10">Cronograma General</option>
              <option value="/zajuna/mod/forum/view.php?id=20">Foro de Dudas e Inquietudes</option>
            </select>
          </body></html>`))
	}))
	defer server.Close()
	client, err := newClient(server.URL, true)
	if err != nil {
		t.Fatal(err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	clientSession := Session{Client: &http.Client{Jar: jar}, BaseURL: server.URL}
	record, err := client.DiscoverCourseMap(context.Background(), clientSession, "41080", CrawlOptions{MaxDepth: 1, MaxPages: 2, MaxLinksPerPage: 20})
	if err != nil {
		t.Fatal(err)
	}
	var values []string
	if err := json.Unmarshal(record.ByItemCode["1.1.1"], &values); err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 || values[0] != server.URL+"/zajuna/mod/page/view.php?forceview=1&id=10" {
		t.Fatalf("jump option title was not resolved: %#v", values)
	}
	if record.Routes[0].Title == "" {
		t.Fatalf("expected route title from jump option: %#v", record.Routes)
	}
}

func TestIsCourseMapFollowCandidateStaysOnCourse(t *testing.T) {
	course := "https://zajuna.sena.edu.co/zajuna/course/view.php?id=41080"
	other := "https://zajuna.sena.edu.co/zajuna/course/view.php?id=99999"
	forum := "https://zajuna.sena.edu.co/zajuna/mod/forum/view.php?id=12"
	if !isCourseMapFollowCandidate(course, "course", "41080") {
		t.Fatal("same course page must be followed")
	}
	if isCourseMapFollowCandidate(other, "course", "41080") {
		t.Fatal("other course pages must not be crawled")
	}
	if !isCourseMapFollowCandidate(forum, "forum", "41080") {
		t.Fatal("same-site activity pages remain follow candidates")
	}
	if fallbackRouteTitle(forum, "forum") == "" {
		t.Fatal("empty anchors still get a title fallback")
	}
}
