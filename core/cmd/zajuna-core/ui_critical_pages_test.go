package main

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/mxschmitt/playwright-go"
	"github.com/zajuna-app/core/internal/capture"
	"github.com/zajuna-app/core/internal/storage/sqlite"
	"github.com/zajuna-app/core/internal/zajuna"
)

// TestCriticalPagesBrowserSmoke checks that every critical route exposes its
// current accessible title (main labelled by a single h1 with the navigation
// label) and no interactive element without an accessible name.
func TestCriticalPagesBrowserSmoke(t *testing.T) {
	if os.Getenv("ZAJUNA_RUN_BROWSER_SMOKE") != "1" {
		t.Skip("browser smoke disabled")
	}

	dataDir := t.TempDir()
	store, err := sqlite.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := writeConfig(dataDir, appConfig{SetupComplete: true, ZajunaUsername: "qa-user", CredentialsStored: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertFichas(context.Background(), []zajuna.Ficha{{ExternalID: "qa-ficha", Name: "Ficha de prueba", CourseID: "qa-course"}}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(newRouterWithServices(dataDir, &memoryCredentialStore{}, nil, store, nil))
	defer server.Close()

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
	browserContext, err := browser.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer browserContext.Close()
	page, err := browserContext.NewPage()
	if err != nil {
		t.Fatal(err)
	}
	if err := page.SetViewportSize(1366, 768); err != nil {
		t.Fatal(err)
	}

	pages := []struct {
		route   string
		title   string
		content string
	}{
		{route: "/resumen", title: "Resumen", content: "#ficha-select"},
		{route: "/fichas", title: "Fichas"},
		{route: "/checklist", title: "Checklist"},
		{route: "/actividades", title: "Actividades"},
		{route: "/evidencias", title: "Evidencias", content: ".evidence-gallery"},
		{route: "/trabajos", title: "Trabajos"},
		{route: "/reportes", title: "Reportes"},
		{route: "/configuracion", title: "Configuración", content: "#settings-tab-account"},
		{route: "/diagnostico", title: "Diagnóstico"},
	}
	for _, expected := range pages {
		if _, err := page.Goto(server.URL + expected.route); err != nil {
			t.Fatalf("navigate %s: %v", expected.route, err)
		}
		if err := page.Locator("#dashboard-main").WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
			t.Fatalf("%s main landmark: %v", expected.route, err)
		}
		if expected.content != "" {
			if err := page.Locator(expected.content).First().WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
				t.Fatalf("%s missing %s: %v", expected.route, expected.content, err)
			}
		}
		heading, err := page.Evaluate(`() => {
			const main = document.getElementById('dashboard-main')
			const labelled = main && document.getElementById(main.getAttribute('aria-labelledby') || '')
			return {
				tag: labelled?.tagName || '',
				text: labelled?.textContent?.trim() || '',
				h1: document.querySelectorAll('h1').length,
			}
		}`)
		if err != nil {
			t.Fatalf("%s inspect heading: %v", expected.route, err)
		}
		headingMap, _ := heading.(map[string]any)
		if fmt.Sprint(headingMap["tag"]) != "H1" || fmt.Sprint(headingMap["text"]) != expected.title || toInt(headingMap["h1"]) != 1 {
			t.Fatalf("%s accessible title: got %#v, want a single h1 %q labelling main", expected.route, heading, expected.title)
		}
		unnamed, err := page.Evaluate(`() => Array.from(document.querySelectorAll('button,a,input,select,textarea')).filter((element) => {
			if (element.getAttribute('aria-hidden') === 'true' || element.disabled || element.closest('[hidden]')) return false
			const label = element.getAttribute('aria-label') || element.getAttribute('title') || element.textContent?.trim()
			if (label) return false
			if (element.id && document.querySelector('label[for="' + CSS.escape(element.id) + '"]')) return false
			return !element.closest('label')
		}).map((element) => ({tag: element.tagName, id: element.id, className: String(element.className)}))`)
		if err != nil {
			t.Fatalf("%s inspect accessible names: %v", expected.route, err)
		}
		if values, ok := unnamed.([]any); !ok || len(values) > 0 {
			t.Fatalf("%s unnamed interactive elements: %#v", expected.route, unnamed)
		}
		t.Logf("critical page %s title=%q names=ok", expected.route, expected.title)
	}

	// Configuración › Cuenta: saved credentials are "Configurada" until a real
	// connection test passes, and the test action is offered explicitly.
	if _, err := page.Goto(server.URL + "/configuracion?tab=account"); err != nil {
		t.Fatal(err)
	}
	testButton := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Probar conexión"})
	if err := testButton.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
		t.Fatalf("connection test action: %v", err)
	}
	if disabled, err := testButton.IsDisabled(); err != nil || disabled {
		t.Fatalf("connection test must be available once setup is complete: disabled=%v err=%v", disabled, err)
	}
	chips, err := page.Locator("#settings-panel-account .status-chip").AllTextContents()
	if err != nil {
		t.Fatal(err)
	}
	if len(chips) != 1 || chips[0] != "Configurada" {
		t.Fatalf("an untested account must show Configurada, got %q", chips)
	}

	// Retention only drives "Limpiar antiguas", so it lives next to it.
	if _, err := page.Goto(server.URL + "/configuracion?tab=backup"); err != nil {
		t.Fatal(err)
	}
	if err := page.GetByLabel("Copias recientes a conservar").WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
		t.Fatalf("retention next to backup cleanup: %v", err)
	}
	if _, err := page.Goto(server.URL + "/configuracion?tab=storage"); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("#settings-panel-storage").WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
		t.Fatal(err)
	}
	if count, err := page.GetByLabel("Copias recientes a conservar").Count(); err != nil || count != 0 {
		t.Fatalf("retention must not be duplicated in Almacenamiento: count=%d err=%v", count, err)
	}
}
