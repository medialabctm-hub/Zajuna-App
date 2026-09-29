package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/zajuna-app/core/internal/coursemaps"
	"github.com/zajuna-app/core/internal/jobs"
	"github.com/zajuna-app/core/internal/storage/sqlite"
	"github.com/zajuna-app/core/internal/workers"
	"github.com/zajuna-app/core/internal/zajuna"
)

type localCourseMapClient struct{}

func (localCourseMapClient) Login(context.Context, zajuna.Credentials) (zajuna.Session, error) {
	return zajuna.Session{}, nil
}

func (localCourseMapClient) DiscoverCourseMap(_ context.Context, _ zajuna.Session, courseID string, _ zajuna.CrawlOptions) (coursemaps.Record, error) {
	now := time.Now().UTC()
	return coursemaps.Record{
		CourseID:      courseID,
		CourseURL:     "https://fixture.local/zajuna/course/view.php?id=" + courseID,
		ByItemCode:    map[string]json.RawMessage{"route.assign": json.RawMessage(`["https://fixture.local/zajuna/mod/assign/view.php?id=31"]`)},
		Routes:        []coursemaps.Route{{URL: "https://fixture.local/zajuna/mod/assign/view.php?id=31", Kind: "assign", ActivityID: "31", Technical: true}},
		LinkCount:     1,
		ItemCodeCount: 1,
		ScrapeStats:   coursemaps.Stats{Total: 1, Assigns: 1},
		Source:        "fixture",
		DiscoveredAt:  now,
		UpdatedAt:     now,
	}, nil
}

func TestLocalE2ECourseMapDiscoveryPersistsRoutes(t *testing.T) {
	dataDir := t.TempDir()
	store, err := sqlite.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.UpsertFichas(context.Background(), []zajuna.Ficha{{ExternalID: "3135429", Name: "Ficha demo", CourseID: "41080"}}); err != nil {
		t.Fatal(err)
	}
	credentials := &memoryCredentialStore{}
	if err := credentials.Set("qa-user", "qa-password"); err != nil {
		t.Fatal(err)
	}
	runtime, err := jobs.NewRuntime(store, 1)
	if err != nil {
		t.Fatal(err)
	}
	worker, err := workers.NewDiscoverCourseMapsWorker(localCourseMapClient{}, credentials, store)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Register(worker); err != nil {
		t.Fatal(err)
	}
	runtime.Start(context.Background())
	defer runtime.Close()

	server := httptest.NewServer(newRouterWithServices(dataDir, credentials, runtime, store, nil))
	defer server.Close()
	response := doJSON(t, server.Client(), http.MethodPost, server.URL+"/api/course-maps/discover", `{"username":"qa-user"}`)
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("discovery status = %d, body = %s", response.StatusCode, response.Body)
	}
	var created jobView
	decodeJSON(t, response.Body, &created)
	completed := waitForE2EJob(t, server.Client(), server.URL, created.ID)
	if completed.Status != jobs.StatusCompleted {
		t.Fatalf("discovery job ended with status %s, error = %s", completed.Status, completed.ErrorMessage)
	}

	maps := getE2EList(t, server.Client(), server.URL+"/api/course-maps?limit=10")
	if len(maps) != 1 || maps[0]["courseId"] != "41080" {
		t.Fatalf("unexpected persisted maps: %#v", maps)
	}
	itemResponse := doJSON(t, server.Client(), http.MethodGet, server.URL+"/api/course-maps/41080", "")
	if itemResponse.StatusCode != http.StatusOK {
		t.Fatalf("single map status = %d, body = %s", itemResponse.StatusCode, itemResponse.Body)
	}
	var item coursemaps.Record
	decodeJSON(t, itemResponse.Body, &item)
	if item.CourseID != "41080" || len(item.Routes) != 1 {
		t.Fatalf("unexpected single map: %#v", item)
	}
}

// newDiscoveryServer levanta el core local con dos fichas sincronizadas. La
// primera queda como ficha activa, igual que tras la primera sincronización.
func newDiscoveryServer(t *testing.T, fichas []zajuna.Ficha) (*httptest.Server, map[string]string) {
	t.Helper()
	dataDir := t.TempDir()
	store, err := sqlite.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if len(fichas) > 0 {
		if _, err := store.UpsertFichas(context.Background(), fichas); err != nil {
			t.Fatal(err)
		}
	}
	credentials := &memoryCredentialStore{}
	if err := credentials.Set("qa-user", "qa-password"); err != nil {
		t.Fatal(err)
	}
	runtime, err := jobs.NewRuntime(store, 1)
	if err != nil {
		t.Fatal(err)
	}
	worker, err := workers.NewDiscoverCourseMapsWorker(localCourseMapClient{}, credentials, store)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Register(worker); err != nil {
		t.Fatal(err)
	}
	runtime.Start(context.Background())
	t.Cleanup(runtime.Close)

	server := httptest.NewServer(newRouterWithServices(dataDir, credentials, runtime, store, nil))
	t.Cleanup(server.Close)

	ids := map[string]string{}
	stored, err := store.ListFichas(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range stored {
		ids[item.ExternalID] = item.ID
	}
	return server, ids
}

// discoveredCourses ejecuta el descubrimiento y devuelve los cursos con mapa.
func discoveredCourses(t *testing.T, server *httptest.Server, body string) []string {
	t.Helper()
	response := doJSON(t, server.Client(), http.MethodPost, server.URL+"/api/course-maps/discover", body)
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("discovery status = %d, body = %s", response.StatusCode, response.Body)
	}
	var created jobView
	decodeJSON(t, response.Body, &created)
	if completed := waitForE2EJob(t, server.Client(), server.URL, created.ID); completed.Status != jobs.StatusCompleted {
		t.Fatalf("discovery job ended with status %s, error = %s", completed.Status, completed.ErrorMessage)
	}
	var courses []string
	for _, item := range getE2EList(t, server.Client(), server.URL+"/api/course-maps?limit=10") {
		courses = append(courses, item["courseId"].(string))
	}
	sort.Strings(courses)
	return courses
}

var twoFichas = []zajuna.Ficha{
	{ExternalID: "3135429", Name: "Ficha uno", CourseID: "41080"},
	{ExternalID: "3118328", Name: "Ficha dos", CourseID: "41090"},
}

func TestDiscoveryOnlyMapsTheSelectedFicha(t *testing.T) {
	t.Run("ficha indicada", func(t *testing.T) {
		server, ids := newDiscoveryServer(t, twoFichas)
		body := `{"username":"qa-user","fichaId":"` + ids["3118328"] + `"}`
		if got := discoveredCourses(t, server, body); len(got) != 1 || got[0] != "41090" {
			t.Fatalf("cursos con mapa = %v, quiero solo 41090", got)
		}
	})
	t.Run("sin ficha usa la activa", func(t *testing.T) {
		server, _ := newDiscoveryServer(t, twoFichas)
		if got := discoveredCourses(t, server, `{"username":"qa-user"}`); len(got) != 1 || got[0] != "41080" {
			t.Fatalf("cursos con mapa = %v, quiero solo 41080", got)
		}
	})
	t.Run("el job recuerda la ficha resuelta", func(t *testing.T) {
		server, ids := newDiscoveryServer(t, twoFichas)
		response := doJSON(t, server.Client(), http.MethodPost, server.URL+"/api/course-maps/discover", `{"username":"qa-user"}`)
		if response.StatusCode != http.StatusAccepted {
			t.Fatalf("status = %d, body = %s", response.StatusCode, response.Body)
		}
		var created jobView
		decodeJSON(t, response.Body, &created)
		if created.FichaID != ids["3135429"] {
			t.Fatalf("fichaId del job = %q, quiero la ficha activa %q", created.FichaID, ids["3135429"])
		}
	})
	t.Run("allFichas recorre todas", func(t *testing.T) {
		server, _ := newDiscoveryServer(t, twoFichas)
		if got := discoveredCourses(t, server, `{"username":"qa-user","allFichas":true}`); len(got) != 2 {
			t.Fatalf("cursos con mapa = %v, quiero los dos", got)
		}
	})
	t.Run("courseIds explícitos mandan sobre la ficha", func(t *testing.T) {
		server, ids := newDiscoveryServer(t, twoFichas)
		body := `{"username":"qa-user","fichaId":"` + ids["3135429"] + `","courseIds":["41090"]}`
		if got := discoveredCourses(t, server, body); len(got) != 1 || got[0] != "41090" {
			t.Fatalf("cursos con mapa = %v, quiero solo 41090", got)
		}
	})
}

func TestDiscoveryRejectsMissingOrUnknownFicha(t *testing.T) {
	t.Run("sin fichas ni ficha activa", func(t *testing.T) {
		server, _ := newDiscoveryServer(t, nil)
		response := doJSON(t, server.Client(), http.MethodPost, server.URL+"/api/course-maps/discover", `{"username":"qa-user"}`)
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, body = %s", response.StatusCode, response.Body)
		}
	})
	t.Run("ficha inexistente", func(t *testing.T) {
		server, _ := newDiscoveryServer(t, twoFichas)
		response := doJSON(t, server.Client(), http.MethodPost, server.URL+"/api/course-maps/discover", `{"username":"qa-user","fichaId":"ficha-inexistente"}`)
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d, body = %s", response.StatusCode, response.Body)
		}
	})
}
