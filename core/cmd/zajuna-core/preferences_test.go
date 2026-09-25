package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zajuna-app/core/internal/jobs"
	"github.com/zajuna-app/core/internal/storage/sqlite"
	"github.com/zajuna-app/core/internal/workers"
)

func TestCapturePreferencesLoaderReflectsSavedSettings(t *testing.T) {
	store, err := sqlite.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	load := capturePreferencesLoader(store)

	if got := load(context.Background()); got != workers.DefaultCapturePreferences() {
		t.Fatalf("without saved settings the defaults apply, got %#v", got)
	}

	saved := defaultSettings()
	saved.Session.AutoRenew = false
	saved.Capture.FullPage = false
	saved.Capture.ReuseSession = false
	if err := saveSettings(context.Background(), store, saved); err != nil {
		t.Fatal(err)
	}
	want := workers.CapturePreferences{FullPage: false, ReuseSession: false, AutoRenew: false}
	if got := load(context.Background()); got != want {
		t.Fatalf("saved preferences must reach the worker: got %#v want %#v", got, want)
	}
}

func TestZajunaConnectionTestUsesConfiguredDocumentType(t *testing.T) {
	dataDir := t.TempDir()
	store, err := sqlite.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	credentials := &memoryCredentialStore{}
	runtime, err := jobs.NewRuntime(store, 1)
	if err != nil {
		t.Fatal(err)
	}
	worker, err := workers.NewTestZajunaConnectionWorker(apiZajunaClient{}, credentials)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Register(worker); err != nil {
		t.Fatal(err)
	}
	runtime.Start(context.Background())
	defer runtime.Close()
	if err := writeConfig(dataDir, appConfig{SetupComplete: true, ZajunaUsername: "987654", ZajunaDocumentType: "TI", CredentialsStored: true}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(newRouterWithServices(dataDir, credentials, runtime, store, nil))
	defer server.Close()

	response, err := server.Client().Post(server.URL+"/api/zajuna/test-connection", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		var body map[string]any
		_ = json.NewDecoder(response.Body).Decode(&body)
		t.Fatalf("unexpected connection test status: %d %v", response.StatusCode, body)
	}
	var created jobView
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	job, err := store.GetJob(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	var input testZajunaConnectionRequest
	if err := json.Unmarshal(job.Input, &input); err != nil {
		t.Fatal(err)
	}
	if input.Username != "987654" || input.DocumentType != "TI" {
		t.Fatalf("the connection test must use the configured account, got %#v", input)
	}
}
