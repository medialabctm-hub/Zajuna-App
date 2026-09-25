package main

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/zajuna-app/core/internal/evidence"
	"github.com/zajuna-app/core/internal/storage/sqlite"
	"github.com/zajuna-app/core/internal/zajuna"
)

func TestEvidenceReviewAPIHappyPath(t *testing.T) {
	dataDir := t.TempDir()
	store, err := sqlite.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if _, err := store.UpsertFichas(ctx, []zajuna.Ficha{{ExternalID: "3135429", Name: "Ficha", CourseID: "41080"}}); err != nil {
		t.Fatal(err)
	}
	fichas, err := store.ListFichas(ctx, 10)
	if err != nil || len(fichas) != 1 {
		t.Fatalf("fichas: %#v (%v)", fichas, err)
	}
	fichaID := fichas[0].ID

	path := filepath.Join(dataDir, "evidences", "blank.png")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	img := image.NewGray(image.Rect(0, 0, 600, 400))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	img.SetGray(0, 0, color.Gray{})
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, img); err != nil {
		t.Fatal(err)
	}
	file.Close()
	if err := store.CreateEvidence(ctx, evidence.Record{ID: "ev-1", FichaID: fichaID, ItemCode: "1.1.1", SlotNumber: 1, Name: "Cronograma", FilePath: path, Format: "png", Source: "capture-checklist", SHA256: "sha-1"}); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(newRouterWithServices(dataDir, &memoryCredentialStore{}, nil, store, nil))
	defer server.Close()

	send := func(method, url, body string, target any) int {
		t.Helper()
		request, err := http.NewRequest(method, server.URL+url, bytes.NewBufferString(body))
		if err != nil {
			t.Fatal(err)
		}
		if body != "" {
			request.Header.Set("Content-Type", "application/json")
		}
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if target != nil && response.StatusCode == http.StatusOK {
			if err := json.NewDecoder(response.Body).Decode(target); err != nil {
				t.Fatal(err)
			}
		}
		return response.StatusCode
	}

	var report evidence.ReviewReport
	if status := send(http.MethodGet, "/api/evidences/review?fichaId="+fichaID, "", &report); status != http.StatusOK {
		t.Fatalf("GET review status %d", status)
	}
	if report.FichaID != fichaID || report.Summary.Total != 1 || report.Summary.Pending != 1 || report.Evidences[0].Reasons[0].Code != evidence.ReasonMostlyBlank || report.VerifiedAt == "" {
		t.Fatalf("unexpected report %#v", report)
	}
	if report.Summary.ItemsMissing == 0 || len(report.MissingItems) != report.Summary.ItemsMissing {
		t.Fatalf("expected missing items %#v", report.Summary)
	}

	var entry evidence.ReviewEntry
	if status := send(http.MethodPut, "/api/evidences/ev-1/review", `{"status":"approved","note":"Correcta"}`, &entry); status != http.StatusOK {
		t.Fatalf("PUT review status %d", status)
	}
	if entry.Status != evidence.ReviewApproved || entry.Source != evidence.ReviewSourceManual || entry.Note != "Correcta" {
		t.Fatalf("unexpected entry %#v", entry)
	}

	report = evidence.ReviewReport{}
	if status := send(http.MethodPost, "/api/evidences/verify", `{"fichaId":"`+fichaID+`"}`, &report); status != http.StatusOK {
		t.Fatalf("POST verify status %d", status)
	}
	if report.Summary.Approved != 1 || report.Summary.ItemsApproved != 1 || report.Evidences[0].Source != evidence.ReviewSourceManual {
		t.Fatalf("manual decision lost after verify %#v", report)
	}

	if status := send(http.MethodPut, "/api/evidences/ev-1/review", `{"status":"bogus"}`, nil); status != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid status, got %d", status)
	}
	if status := send(http.MethodPut, "/api/evidences/missing/review", `{"status":"approved"}`, nil); status != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown evidence, got %d", status)
	}
	if status := send(http.MethodGet, "/api/evidences/review", "", nil); status != http.StatusBadRequest {
		t.Fatalf("expected 400 without fichaId, got %d", status)
	}
}
