package main

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mxschmitt/playwright-go"
	"github.com/zajuna-app/core/internal/capture"
	"github.com/zajuna-app/core/internal/coursemaps"
	"github.com/zajuna-app/core/internal/jobs"
	"github.com/zajuna-app/core/internal/storage/sqlite"
	"github.com/zajuna-app/core/internal/workers"
	"github.com/zajuna-app/core/internal/zajuna"
)

// gatedCourseMapClient descubre rutas solo cuando el test lo libera, para poder
// ver la pantalla de carga mientras la búsqueda sigue en marcha.
type gatedCourseMapClient struct {
	localCourseMapClient
	release <-chan struct{}
}

func (c gatedCourseMapClient) DiscoverCourseMap(ctx context.Context, session zajuna.Session, courseID string, options zajuna.CrawlOptions) (coursemaps.Record, error) {
	select {
	case <-c.release:
	case <-ctx.Done():
		return coursemaps.Record{}, ctx.Err()
	}
	return c.localCourseMapClient.DiscoverCourseMap(ctx, session, courseID, options)
}

// newFirstRunServer levanta el core con el primer arranque pendiente: dos
// fichas ya sincronizadas y ningún mapa de rutas.
func newFirstRunServer(t *testing.T, release <-chan struct{}) *httptest.Server {
	t.Helper()
	dataDir := t.TempDir()
	store, err := sqlite.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if err := writeConfig(dataDir, appConfig{SetupComplete: true, ZajunaUsername: "qa-user", CredentialsStored: true, FirstRunPending: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertFichas(context.Background(), twoFichas); err != nil {
		t.Fatal(err)
	}
	credentials := &memoryCredentialStore{}
	if err := credentials.Set("qa-user", "qa-password"); err != nil {
		t.Fatal(err)
	}
	runtime, err := jobs.NewRuntime(store, 2)
	if err != nil {
		t.Fatal(err)
	}
	worker, err := workers.NewDiscoverCourseMapsWorker(gatedCourseMapClient{release: release}, credentials, store)
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
	return server
}

// TestFirstRunBrowserSmoke recorre la pantalla de carga del primer arranque en
// un navegador real: aparece mientras se buscan las rutas, deja entrar sola
// cuando terminan y se puede omitir sin esperar.
func TestFirstRunBrowserSmoke(t *testing.T) {
	if os.Getenv("ZAJUNA_RUN_BROWSER_SMOKE") != "1" {
		t.Skip("browser smoke disabled")
	}
	pw, err := capture.Resolve("").Start()
	if err != nil {
		t.Fatal(err)
	}
	defer pw.Stop()
	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{Headless: playwright.Bool(true), Args: []string{"--disable-gpu"}})
	if err != nil {
		t.Fatal(err)
	}
	defer browser.Close()

	visible := playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible, Timeout: playwright.Float(20000)}
	open := func(t *testing.T, server *httptest.Server) playwright.Page {
		t.Helper()
		browserContext, err := browser.NewContext()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { browserContext.Close() })
		page, err := browserContext.NewPage()
		if err != nil {
			t.Fatal(err)
		}
		if err := page.SetViewportSize(1366, 768); err != nil {
			t.Fatal(err)
		}
		if _, err := page.Goto(server.URL + "/resumen"); err != nil {
			t.Fatal(err)
		}
		return page
	}
	waitFirstRunCleared := func(t *testing.T, server *httptest.Server) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for firstRunPending(t, server.URL, server.Client()) {
			if time.Now().After(deadline) {
				t.Fatal("el primer arranque sigue marcado como pendiente")
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	shot := func(t *testing.T, page playwright.Page, name string) {
		t.Helper()
		if dir := os.Getenv("ZAJUNA_VISUAL_ARTIFACT_DIR"); dir != "" {
			if _, err := page.Screenshot(playwright.PageScreenshotOptions{Path: playwright.String(filepath.Join(dir, name+".png"))}); err != nil {
				t.Fatal(err)
			}
		}
	}

	t.Run("entra sola cuando terminan las rutas", func(t *testing.T) {
		release := make(chan struct{})
		t.Cleanup(func() {
			select {
			case <-release:
			default:
				close(release)
			}
		})
		server := newFirstRunServer(t, release)
		page := open(t, server)

		if err := page.Locator("h1:has-text('Preparando las rutas de tus cursos')").WaitFor(visible); err != nil {
			t.Fatalf("no apareció la pantalla de carga: %v", err)
		}
		shot(t, page, "first-run-loading")
		if count, _ := page.Locator("#dashboard-main").Count(); count != 0 {
			t.Fatal("la aplicación no debe verse mientras se buscan las rutas")
		}
		if !firstRunPending(t, server.URL, server.Client()) {
			t.Fatal("el primer arranque debe seguir pendiente mientras dura la búsqueda")
		}

		close(release)
		if err := page.Locator("#dashboard-main").WaitFor(visible); err != nil {
			t.Fatalf("la aplicación no se abrió al terminar la búsqueda: %v", err)
		}
		shot(t, page, "first-run-done")
		waitFirstRunCleared(t, server)
		// Las rutas se buscaron para las dos fichas, una sola vez.
		if maps := getE2EList(t, server.Client(), server.URL+"/api/course-maps?limit=10"); len(maps) != 2 {
			t.Fatalf("mapas de rutas = %d, quiero 2", len(maps))
		}
	})

	t.Run("se puede continuar sin esperar", func(t *testing.T) {
		release := make(chan struct{})
		t.Cleanup(func() {
			select {
			case <-release:
			default:
				close(release)
			}
		})
		server := newFirstRunServer(t, release)
		page := open(t, server)

		if err := page.Locator("h1:has-text('Preparando las rutas de tus cursos')").WaitFor(visible); err != nil {
			t.Fatalf("no apareció la pantalla de carga: %v", err)
		}
		if err := page.Locator("button:has-text('Continuar sin esperar')").Click(); err != nil {
			t.Fatal(err)
		}
		if err := page.Locator("#dashboard-main").WaitFor(visible); err != nil {
			t.Fatalf("la aplicación no se abrió al omitir: %v", err)
		}
		waitFirstRunCleared(t, server)
	})

	t.Run("sin primer arranque pendiente no aparece", func(t *testing.T) {
		release := make(chan struct{})
		close(release)
		server := newFirstRunServer(t, release)
		if response := doJSON(t, server.Client(), "POST", server.URL+"/api/setup/first-run/complete", ""); response.StatusCode != 200 {
			t.Fatalf("complete = %d", response.StatusCode)
		}
		page := open(t, server)
		if err := page.Locator("#dashboard-main").WaitFor(visible); err != nil {
			t.Fatalf("la aplicación debe abrir directo: %v", err)
		}
		if count, _ := page.Locator("h1:has-text('Preparando las rutas de tus cursos')").Count(); count != 0 {
			t.Fatal("la pantalla de carga no debe aparecer")
		}
	})
}
