package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/zajuna-app/core/internal/evidence"
)

type thumbnailEvidenceStore struct{ records map[string]evidence.Record }

func (s thumbnailEvidenceStore) CreateEvidence(context.Context, evidence.Record) error { return nil }
func (s thumbnailEvidenceStore) ListEvidences(context.Context, int) ([]evidence.Record, error) {
	return nil, nil
}
func (s thumbnailEvidenceStore) GetEvidence(_ context.Context, id string) (evidence.Record, error) {
	record, ok := s.records[id]
	if !ok {
		return evidence.Record{}, errors.New("not found")
	}
	return record, nil
}

func writeTestPNG(t *testing.T, path string, width, height int) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.NRGBA{R: 20, G: 120, B: 200, A: 255})
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, img); err != nil {
		t.Fatal(err)
	}
}

func TestEvidenceThumbnailIsSmallCachedJPEG(t *testing.T) {
	dataDir := t.TempDir()
	source := filepath.Join(dataDir, "evidences", "ficha", "capture.png")
	writeTestPNG(t, source, 2000, 2600)
	store := thumbnailEvidenceStore{records: map[string]evidence.Record{
		"ev-1": {ID: "ev-1", FilePath: source, Format: "png", SHA256: "abc"},
	}}
	mux := http.NewServeMux()
	registerEvidenceThumbnailRoute(mux, store, dataDir, newThumbnailer(dataDir))

	var wg sync.WaitGroup
	bodies := make([][]byte, 6)
	for index := range bodies {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/evidences/ev-1/thumbnail", nil))
			if recorder.Code != http.StatusOK || recorder.Header().Get("Content-Type") != "image/jpeg" {
				t.Errorf("status = %d, content-type = %q", recorder.Code, recorder.Header().Get("Content-Type"))
			}
			bodies[index] = recorder.Body.Bytes()
		}(index)
	}
	wg.Wait()

	decoded, err := jpeg.Decode(bytes.NewReader(bodies[0]))
	if err != nil {
		t.Fatal(err)
	}
	if got := decoded.Bounds().Size(); got != (image.Point{X: thumbnailWidth, Y: 624}) {
		t.Fatalf("thumbnail size = %v", got)
	}
	r, g, b, _ := decoded.At(10, 10).RGBA()
	if r>>8 > 40 || g>>8 < 100 || b>>8 < 180 {
		t.Fatalf("thumbnail color was not preserved: %d %d %d", r>>8, g>>8, b>>8)
	}
	cached, err := filepath.Glob(filepath.Join(dataDir, thumbnailDir, "*.jpg"))
	if err != nil || len(cached) != 1 {
		t.Fatalf("expected one cached thumbnail, got %v (%v)", cached, err)
	}
	leftovers, _ := filepath.Glob(filepath.Join(dataDir, thumbnailDir, "*.tmp"))
	if len(leftovers) != 0 {
		t.Fatalf("temporary files were left behind: %v", leftovers)
	}
}

func TestEvidenceThumbnailRejectsFilesOutsideEvidences(t *testing.T) {
	dataDir := t.TempDir()
	outside := filepath.Join(dataDir, "config.png")
	writeTestPNG(t, outside, 10, 10)
	store := thumbnailEvidenceStore{records: map[string]evidence.Record{
		"ev-1": {ID: "ev-1", FilePath: outside, Format: "png"},
	}}
	mux := http.NewServeMux()
	registerEvidenceThumbnailRoute(mux, store, dataDir, newThumbnailer(dataDir))
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/evidences/ev-1/thumbnail", nil))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", recorder.Code)
	}
}

func TestDownscaleCompositesTransparencyOverWhite(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 4, 4))
	dst := downscale(src, 2)
	if dst.Bounds().Dx() != 2 || dst.Bounds().Dy() != 2 {
		t.Fatalf("size = %v", dst.Bounds())
	}
	if got := dst.RGBAAt(0, 0); got != (color.RGBA{R: 255, G: 255, B: 255, A: 255}) {
		t.Fatalf("transparent pixels should become white, got %v", got)
	}
}

func TestThumbnailCacheDropsStaleAndDeletedEvidences(t *testing.T) {
	dataDir := t.TempDir()
	first := filepath.Join(dataDir, "evidences", "a.png")
	second := filepath.Join(dataDir, "evidences", "b.png")
	writeTestPNG(t, first, 40, 40)
	writeTestPNG(t, second, 40, 40)
	store := thumbnailEvidenceStore{records: map[string]evidence.Record{
		"ev-a": {ID: "ev-a", FilePath: first, Format: "png"},
		"ev-b": {ID: "ev-b", FilePath: second, Format: "png"},
	}}
	thumbs := newThumbnailer(dataDir)
	count := func() int {
		matches, _ := filepath.Glob(filepath.Join(dataDir, thumbnailDir, "*.jpg"))
		return len(matches)
	}
	oldPath, err := thumbs.ensure(store.records["ev-a"])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := thumbs.ensure(store.records["ev-b"]); err != nil {
		t.Fatal(err)
	}
	// Replacing the source produces a new version and drops the old one.
	writeTestPNG(t, first, 60, 30)
	newPath, err := thumbs.ensure(store.records["ev-a"])
	if err != nil {
		t.Fatal(err)
	}
	if newPath == oldPath || count() != 2 {
		t.Fatalf("stale thumbnail kept: old=%s new=%s count=%d", oldPath, newPath, count())
	}
	thumbs.remove("ev-a", "")
	if count() != 1 {
		t.Fatalf("deleted evidence kept its thumbnail, count=%d", count())
	}
	delete(store.records, "ev-b")
	thumbs.prune(context.Background(), listingStore{store})
	if count() != 0 {
		t.Fatalf("prune kept thumbnails of missing evidences, count=%d", count())
	}
}

type listingStore struct{ thumbnailEvidenceStore }

func (s listingStore) ListEvidences(context.Context, int) ([]evidence.Record, error) {
	records := make([]evidence.Record, 0, len(s.records))
	for _, record := range s.records {
		records = append(records, record)
	}
	return records, nil
}

// pngHeaderClaiming writes only the signature and IHDR of a PNG that declares
// the given size: a few bytes on disk that would need gigabytes to decode.
func pngHeaderClaiming(t *testing.T, path string, width, height uint32) {
	t.Helper()
	chunk := func(kind string, data []byte) []byte {
		out := make([]byte, 0, 12+len(data))
		out = binary.BigEndian.AppendUint32(out, uint32(len(data)))
		out = append(out, kind...)
		out = append(out, data...)
		return binary.BigEndian.AppendUint32(out, crc32.ChecksumIEEE(append([]byte(kind), data...)))
	}
	ihdr := binary.BigEndian.AppendUint32(nil, width)
	ihdr = binary.BigEndian.AppendUint32(ihdr, height)
	ihdr = append(ihdr, 8, 6, 0, 0, 0)
	contents := append([]byte{0x89, 'P', 'N', 'G', 13, 10, 0x1a, 10}, chunk("IHDR", ihdr)...)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestEvidenceThumbnailRefusesDecompressionBombs(t *testing.T) {
	dataDir := t.TempDir()
	source := filepath.Join(dataDir, "evidences", "bomb.png")
	pngHeaderClaiming(t, source, 60000, 60000)
	store := thumbnailEvidenceStore{records: map[string]evidence.Record{
		"ev-bomb": {ID: "ev-bomb", FilePath: source, Format: "png"},
	}}
	mux := http.NewServeMux()
	registerEvidenceThumbnailRoute(mux, store, dataDir, newThumbnailer(dataDir))
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/evidences/ev-bomb/thumbnail", nil))
	original, _ := os.ReadFile(source)
	if recorder.Code != http.StatusOK || !bytes.Equal(recorder.Body.Bytes(), original) {
		t.Fatalf("oversized image should fall back to the original file: status=%d", recorder.Code)
	}
	if cached, _ := filepath.Glob(filepath.Join(dataDir, thumbnailDir, "*")); len(cached) != 0 {
		t.Fatalf("oversized image must not be decoded or cached: %v", cached)
	}
}

func TestThumbnailPruneRemovesOnlyStaleTempFiles(t *testing.T) {
	dataDir := t.TempDir()
	thumbs := newThumbnailer(dataDir)
	if err := os.MkdirAll(thumbs.dir, 0o700); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(thumbs.dir, "thumb-old.tmp")
	fresh := filepath.Join(thumbs.dir, "thumb-new.tmp")
	for _, path := range []string{stale, fresh} {
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-2 * orphanTempAge)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}
	thumbs.prune(context.Background(), listingStore{thumbnailEvidenceStore{records: map[string]evidence.Record{}}})
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale temp file should be removed: %v", err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatalf("an in-progress temp file must survive: %v", err)
	}
}
