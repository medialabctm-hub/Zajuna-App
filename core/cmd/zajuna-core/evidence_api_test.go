package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zajuna-app/core/internal/evidence"
	"github.com/zajuna-app/core/internal/reports"
	"github.com/zajuna-app/core/internal/storage/sqlite"
	"github.com/zajuna-app/core/internal/zajuna"
)

func newEvidenceAPIFixture(t *testing.T) (string, *sqlite.Store, string, *httptest.Server) {
	t.Helper()
	dataDir := t.TempDir()
	store, err := sqlite.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if _, err := store.UpsertFichas(context.Background(), []zajuna.Ficha{{ExternalID: "900", Name: "Ficha", CourseID: "41080"}}); err != nil {
		t.Fatal(err)
	}
	fichas, err := store.ListFichas(context.Background(), 10)
	if err != nil || len(fichas) != 1 {
		t.Fatalf("fichas: %#v (%v)", fichas, err)
	}
	server := httptest.NewServer(newRouterWithServices(dataDir, &memoryCredentialStore{}, nil, store, nil))
	t.Cleanup(server.Close)
	return dataDir, store, fichas[0].ID, server
}

func writeEvidenceFile(t *testing.T, dataDir, name string) string {
	t.Helper()
	dir := filepath.Join(dataDir, "evidences", "checklist")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("png-"+name), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func doRequest(t *testing.T, server *httptest.Server, method, path string, body io.Reader, contentType string) (int, []byte) {
	t.Helper()
	request, err := http.NewRequest(method, server.URL+path, body)
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, payload
}

func TestDeleteEvidenceAPIKeepsFileSharedByAnotherItem(t *testing.T) {
	dataDir, store, fichaID, server := newEvidenceAPIFixture(t)
	ctx := context.Background()
	shared := writeEvidenceFile(t, dataDir, "shared.png")
	for _, item := range []string{"10.1.1", "10.1.2"} {
		if err := store.CreateEvidence(ctx, evidence.Record{
			ID: "e-" + item, FichaID: fichaID, ItemCode: item, SlotNumber: 1, Name: item, FilePath: shared,
			Format: "png", Source: "capture-checklist", SHA256: "same-hash", CapturedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatal(err)
		}
	}

	status, body := doRequest(t, server, http.MethodDelete, "/api/evidences/e-10.1.1", nil, "")
	if status != http.StatusOK {
		t.Fatalf("delete status %d: %s", status, body)
	}
	if _, err := os.Stat(shared); err != nil {
		t.Fatalf("shared file removed while 10.1.2 still references it: %v", err)
	}
	if _, err := store.GetEvidence(ctx, "e-10.1.2"); err != nil {
		t.Fatalf("the other row must survive: %v", err)
	}
	if _, err := store.GetEvidence(ctx, "e-10.1.1"); err == nil {
		t.Fatal("the deleted row is still present")
	}
	groups, err := store.ListEvidenceGroups(ctx, fichaID)
	if err != nil || len(groups) != 1 || len(groups[0].EvidenceIDs) != 1 || groups[0].EvidenceIDs[0] != "e-10.1.2" {
		t.Fatalf("groups must be rebuilt after delete: %#v (%v)", groups, err)
	}

	status, body = doRequest(t, server, http.MethodDelete, "/api/evidences/e-10.1.2", nil, "")
	if status != http.StatusOK {
		t.Fatalf("second delete status %d: %s", status, body)
	}
	if _, err := os.Stat(shared); !os.IsNotExist(err) {
		t.Fatalf("file must be removed once unreferenced: %v", err)
	}
}

func TestDeleteEvidenceAPIRetiresRowWhoseFileIsMissing(t *testing.T) {
	dataDir, store, fichaID, server := newEvidenceAPIFixture(t)
	missing := filepath.Join(dataDir, "evidences", "checklist", "gone.png")
	if err := store.CreateEvidence(context.Background(), evidence.Record{
		ID: "missing", FichaID: fichaID, ItemCode: "1.1.1", SlotNumber: 1, Name: "gone", FilePath: missing,
		Format: "png", Source: "capture-checklist", SHA256: "gone", CapturedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if status, body := doRequest(t, server, http.MethodDelete, "/api/evidences/missing", nil, ""); status != http.StatusOK {
		t.Fatalf("delete status %d: %s", status, body)
	}
}

func TestDeleteEvidenceAPIRejectsFileOutsideEvidenceStorage(t *testing.T) {
	_, store, fichaID, server := newEvidenceAPIFixture(t)
	outside := filepath.Join(t.TempDir(), "outside.png")
	if err := os.WriteFile(outside, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateEvidence(context.Background(), evidence.Record{
		ID: "outside", FichaID: fichaID, ItemCode: "1.1.1", SlotNumber: 1, Name: "outside", FilePath: outside,
		Format: "png", Source: "capture-checklist", SHA256: "outside", CapturedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if status, _ := doRequest(t, server, http.MethodDelete, "/api/evidences/outside", nil, ""); status != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", status)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("file outside storage must not be touched: %v", err)
	}
}

func assertNoAbsolutePath(t *testing.T, label string, body []byte, dataDir string) {
	t.Helper()
	escaped, _ := json.Marshal(dataDir)
	text := string(body)
	if strings.Contains(text, dataDir) || strings.Contains(text, strings.Trim(string(escaped), `"`)) || strings.Contains(text, filepath.ToSlash(dataDir)) {
		t.Fatalf("%s exposes the absolute data directory: %s", label, text)
	}
	if strings.Contains(text, `"filePath"`) {
		t.Fatalf("%s still exposes filePath: %s", label, text)
	}
}

func TestEvidenceReportAndAppViewsDoNotExposeAbsolutePaths(t *testing.T) {
	dataDir, store, fichaID, server := newEvidenceAPIFixture(t)
	ctx := context.Background()
	shared := writeEvidenceFile(t, dataDir, "view.png")
	for _, item := range []string{"2.1.1", "2.1.2"} {
		if err := store.CreateEvidence(ctx, evidence.Record{
			ID: "v-" + item, FichaID: fichaID, ItemCode: item, SlotNumber: 1, Name: item, FilePath: shared,
			Format: "png", Source: "capture-checklist", SHA256: "view-" + item, CapturedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.CreateReport(ctx, reports.Record{ID: "r1", Name: "Reporte", FilePath: filepath.Join(dataDir, "reports", "r1.html"), Format: "html", SHA256: "r1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RebuildEvidenceGroups(ctx, fichaID); err != nil {
		t.Fatal(err)
	}

	status, body := doRequest(t, server, http.MethodGet, "/api/evidences?fichaId="+fichaID, nil, "")
	if status != http.StatusOK {
		t.Fatalf("evidences status %d: %s", status, body)
	}
	assertNoAbsolutePath(t, "GET /api/evidences", body, dataDir)
	var views []evidenceView
	if err := json.Unmarshal(body, &views); err != nil {
		t.Fatal(err)
	}
	if len(views) != 2 || views[0].FileKey == "" || views[0].FileKey != views[1].FileKey {
		t.Fatalf("rows sharing a file must expose the same opaque fileKey: %#v", views)
	}

	for _, path := range []string{"/api/evidences", "/api/evidences/groups?fichaId=" + fichaID, "/api/reports", "/api/app/info", "/api/checklist/dashboard?fichaId=" + fichaID, "/api/checklist/items/2.1.1?fichaId=" + fichaID} {
		status, body := doRequest(t, server, http.MethodGet, path, nil, "")
		if status != http.StatusOK {
			t.Fatalf("%s status %d: %s", path, status, body)
		}
		assertNoAbsolutePath(t, path, body, dataDir)
	}
}

func TestDisplayDataDirHidesUserProfile(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LOCALAPPDATA", root)
	if got, want := displayDataDir(filepath.Join(root, "ZajunaApp")), "%LOCALAPPDATA%"+string(filepath.Separator)+"ZajunaApp"; got != want {
		t.Fatalf("displayDataDir = %q, want %q", got, want)
	}
	outside := filepath.Join(t.TempDir(), "custom-data")
	t.Setenv("LOCALAPPDATA", "")
	if got := displayDataDir(outside); got != "custom-data" && !strings.HasPrefix(got, "~") {
		t.Fatalf("unexpected fallback: %q", got)
	}
	if got := displayDataDir(outside); strings.Contains(got, outside) {
		t.Fatalf("fallback leaks absolute path: %q", got)
	}
}

func uploadEvidence(t *testing.T, server *httptest.Server, fichaID, itemCode, fileName string, contents []byte) (int, evidenceView) {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	_ = form.WriteField("fichaId", fichaID)
	if itemCode != "" {
		_ = form.WriteField("itemCode", itemCode)
	}
	part, err := form.CreateFormFile("file", fileName)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(contents)
	_ = form.Close()
	status, payload := doRequest(t, server, http.MethodPost, "/api/evidences/upload", &body, form.FormDataContentType())
	var view evidenceView
	if status < 300 {
		if err := json.Unmarshal(payload, &view); err != nil {
			t.Fatalf("decode upload: %v (%s)", err, payload)
		}
	} else {
		t.Fatalf("upload status %d: %s", status, payload)
	}
	return status, view
}

func TestUploadEvidenceAPIRebuildsGroupsAndKeepsPerItemRows(t *testing.T) {
	dataDir, store, fichaID, server := newEvidenceAPIFixture(t)
	ctx := context.Background()
	image := []byte("\x89PNG manual fixture")

	status, first := uploadEvidence(t, server, fichaID, "6.1", "captura.png", image)
	if status != http.StatusCreated {
		t.Fatalf("first upload status %d", status)
	}
	groups, err := store.ListEvidenceGroups(ctx, fichaID)
	if err != nil || len(groups) != 1 || len(groups[0].EvidenceIDs) != 1 || groups[0].EvidenceIDs[0] != first.ID {
		t.Fatalf("upload must rebuild groups without a manual rebuild: %#v (%v)", groups, err)
	}

	_, second := uploadEvidence(t, server, fichaID, "10.1.1", "captura.png", image)
	if second.ID == first.ID {
		t.Fatalf("the same file for another ítem must create its own row: %s", second.ID)
	}
	if _, err := store.GetEvidence(ctx, first.ID); err != nil {
		t.Fatalf("first ítem lost its evidence: %v", err)
	}
	groups, err = store.ListEvidenceGroups(ctx, fichaID)
	if err != nil || len(groups) != 1 || len(groups[0].ItemCodes) != 2 {
		t.Fatalf("identical uploads must share one content group covering both ítems: %#v (%v)", groups, err)
	}

	status, again := uploadEvidence(t, server, fichaID, "6.1", "captura.png", image)
	if status != http.StatusOK || again.ID != first.ID {
		t.Fatalf("re-uploading identical content must reuse the row: %d %#v", status, again)
	}
	entries, err := os.ReadDir(filepath.Join(dataDir, "evidences", "manual", fichaID))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("duplicate upload must not leave an orphan file, found %d files", len(entries))
	}
}

func TestListEvidencesByFichaIsNotTruncatedForLargeGalleries(t *testing.T) {
	dataDir, store, fichaID, server := newEvidenceAPIFixture(t)
	ctx := context.Background()
	const total = 130
	for index := 1; index <= total; index++ {
		path := writeEvidenceFile(t, dataDir, fmt.Sprintf("g-%d.png", index))
		if err := store.CreateEvidence(ctx, evidence.Record{
			ID: fmt.Sprintf("g-%d", index), FichaID: fichaID, SlotNumber: index, Name: "g", FilePath: path,
			Format: "png", Source: "manual", SHA256: fmt.Sprintf("g-%d", index), CapturedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	status, body := doRequest(t, server, http.MethodGet, "/api/evidences?fichaId="+fichaID, nil, "")
	if status != http.StatusOK {
		t.Fatalf("status %d: %s", status, body)
	}
	var views []evidenceView
	if err := json.Unmarshal(body, &views); err != nil {
		t.Fatal(err)
	}
	if len(views) != total {
		t.Fatalf("gallery truncated: got %d of %d", len(views), total)
	}
	if status, _ := doRequest(t, server, http.MethodGet, "/api/evidences?limit=101", nil, ""); status != http.StatusBadRequest {
		t.Fatalf("global listing must keep its 100 cap, got %d", status)
	}
	if status, _ := doRequest(t, server, http.MethodGet, "/api/evidences?limit=500&fichaId="+fichaID, nil, ""); status != http.StatusOK {
		t.Fatalf("per-ficha listing must accept larger limits, got %d", status)
	}
}
