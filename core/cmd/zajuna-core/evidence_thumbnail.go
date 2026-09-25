package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/zajuna-app/core/internal/evidence"
)

// thumbnailWidth covers the gallery cards (170–250 px wide) on 2× screens.
// Captures are ~2000×2600 px: decoding a full one costs ~21 MB of browser
// memory, so the gallery must never render the original file as a miniature.
const thumbnailWidth = 480

// maxThumbnailSourcePixels caps what is decoded (~128 MB as RGBA). Uploads
// are limited by bytes, not dimensions, so a small compressed image could
// otherwise declare huge dimensions and exhaust memory. Larger sources fall
// back to the original file. Real captures are ~2000×2600 (5 MP); even very
// long pages stay far below the cap.
const maxThumbnailSourcePixels = 32_000_000

// orphanTempAge is how old a leftover thumb-*.tmp must be before prune
// removes it, so an in-progress generation is never interrupted.
const orphanTempAge = 10 * time.Minute

// thumbnailDir lives outside evidences/ so backups do not copy a cache that
// can always be regenerated.
const thumbnailDir = "thumbnails"

type thumbnailer struct {
	dir string
	// slots bounds how many full-size captures are decoded at once; the
	// gallery requests dozens of miniatures in parallel.
	slots chan struct{}
	mu    sync.Mutex
	// inflight collapses concurrent requests for the same thumbnail.
	inflight map[string]*thumbnailJob
}

type thumbnailJob struct {
	done chan struct{}
	err  error
}

func newThumbnailer(dataDir string) *thumbnailer {
	workers := runtime.NumCPU() / 2
	if workers < 1 {
		workers = 1
	}
	if workers > 3 {
		workers = 3
	}
	return &thumbnailer{dir: filepath.Join(dataDir, thumbnailDir), slots: make(chan struct{}, workers), inflight: map[string]*thumbnailJob{}}
}

func thumbnailSupported(format string) bool {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "png", "jpg", "jpeg":
		return true
	}
	return false
}

// idPrefix groups every cached version of one evidence, so deleting the
// evidence (or replacing its file) can remove them.
func thumbnailIDPrefix(id string) string {
	sum := sha256.Sum256([]byte(id))
	return hex.EncodeToString(sum[:10])
}

// path returns the cache file for the evidence as it is on disk now. The
// version part includes the source size and modification time, so a replaced
// file never serves a stale miniature.
func (t *thumbnailer) path(item evidence.Record, info os.FileInfo) string {
	version := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%d|%d", item.SHA256, info.Size(), info.ModTime().UnixNano(), thumbnailWidth)))
	return filepath.Join(t.dir, thumbnailIDPrefix(item.ID)+"-"+hex.EncodeToString(version[:10])+".jpg")
}

// remove deletes every cached version of an evidence except keep.
func (t *thumbnailer) remove(id, keep string) {
	matches, _ := filepath.Glob(filepath.Join(t.dir, thumbnailIDPrefix(id)+"-*.jpg"))
	for _, match := range matches {
		if match != keep {
			_ = os.Remove(match)
		}
	}
}

// prune drops the thumbnails of evidences that no longer exist.
func (t *thumbnailer) prune(ctx context.Context, store evidence.Store) {
	records, err := store.ListEvidences(ctx, 1<<20)
	if err != nil {
		return
	}
	live := make(map[string]bool, len(records))
	for _, record := range records {
		live[thumbnailIDPrefix(record.ID)] = true
	}
	matches, _ := filepath.Glob(filepath.Join(t.dir, "*.jpg"))
	for _, match := range matches {
		prefix, _, _ := strings.Cut(filepath.Base(match), "-")
		if !live[prefix] {
			_ = os.Remove(match)
		}
	}
	// A crash between CreateTemp and Rename leaves a temp file behind.
	temps, _ := filepath.Glob(filepath.Join(t.dir, "thumb-*.tmp"))
	for _, temp := range temps {
		if info, err := os.Stat(temp); err == nil && time.Since(info.ModTime()) > orphanTempAge {
			_ = os.Remove(temp)
		}
	}
}

// ensure returns a cached thumbnail, generating it when missing.
func (t *thumbnailer) ensure(item evidence.Record) (string, error) {
	info, err := os.Stat(item.FilePath)
	if err != nil {
		return "", err
	}
	target := t.path(item, info)
	if _, err := os.Stat(target); err == nil {
		return target, nil
	}

	t.mu.Lock()
	if job, ok := t.inflight[target]; ok {
		t.mu.Unlock()
		<-job.done
		return target, job.err
	}
	job := &thumbnailJob{done: make(chan struct{})}
	t.inflight[target] = job
	t.mu.Unlock()

	// Deferred so a panic while decoding cannot leave waiters blocked or a
	// worker slot taken.
	defer func() {
		t.mu.Lock()
		delete(t.inflight, target)
		t.mu.Unlock()
		close(job.done)
	}()
	job.err = errors.New("no se pudo generar la miniatura")
	t.slots <- struct{}{}
	defer func() { <-t.slots }()
	job.err = writeThumbnail(item.FilePath, item.Format, target)
	if job.err == nil {
		t.remove(item.ID, target)
	}
	return target, job.err
}

func writeThumbnail(source, format, target string) error {
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	defer file.Close()
	config, _, err := image.DecodeConfig(bufio.NewReader(file))
	if err != nil {
		return fmt.Errorf("read evidence image size: %w", err)
	}
	if config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > maxThumbnailSourcePixels {
		return fmt.Errorf("imagen de %d×%d px demasiado grande para miniatura", config.Width, config.Height)
	}
	if _, err := file.Seek(0, 0); err != nil {
		return err
	}
	var decoded image.Image
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "png":
		decoded, err = png.Decode(bufio.NewReader(file))
	case "jpg", "jpeg":
		decoded, err = jpeg.Decode(bufio.NewReader(file))
	default:
		return fmt.Errorf("formato sin miniatura: %s", format)
	}
	if err != nil {
		return fmt.Errorf("decode evidence image: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(target), "thumb-*.tmp")
	if err != nil {
		return err
	}
	if err := jpeg.Encode(temporary, downscale(decoded, thumbnailWidth), &jpeg.Options{Quality: 80}); err != nil {
		_ = temporary.Close()
		_ = os.Remove(temporary.Name())
		return err
	}
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporary.Name())
		return err
	}
	if err := os.Rename(temporary.Name(), target); err != nil {
		_ = os.Remove(temporary.Name())
		return err
	}
	return nil
}

// downscale reduces src to maxWidth keeping its aspect ratio, averaging every
// source pixel that falls into each destination pixel (box filter). Images
// already narrower are only flattened onto white.
func downscale(src image.Image, maxWidth int) *image.RGBA {
	bounds := src.Bounds()
	srcW, srcH := bounds.Dx(), bounds.Dy()
	if srcW <= 0 || srcH <= 0 {
		return image.NewRGBA(image.Rect(0, 0, 1, 1))
	}
	rgba, ok := src.(*image.RGBA)
	if !ok || rgba.Bounds().Min != (image.Point{}) {
		// Normalizing once keeps the averaging loop on direct byte access
		// instead of an interface call per source pixel.
		rgba = image.NewRGBA(image.Rect(0, 0, srcW, srcH))
		draw.Draw(rgba, rgba.Bounds(), src, bounds.Min, draw.Src)
	}
	dstW := srcW
	if dstW > maxWidth {
		dstW = maxWidth
	}
	dstH := srcH * dstW / srcW
	if dstH < 1 {
		dstH = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	for y := 0; y < dstH; y++ {
		y0, y1 := y*srcH/dstH, (y+1)*srcH/dstH
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < dstW; x++ {
			x0, x1 := x*srcW/dstW, (x+1)*srcW/dstW
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var r, g, b, a, n uint64
			for sy := y0; sy < y1; sy++ {
				row := rgba.Pix[sy*rgba.Stride+x0*4 : sy*rgba.Stride+x1*4]
				for i := 0; i < len(row); i += 4 {
					r += uint64(row[i])
					g += uint64(row[i+1])
					b += uint64(row[i+2])
					a += uint64(row[i+3])
				}
				n += uint64(x1 - x0)
			}
			// Premultiplied values composited over white: JPEG has no alpha.
			white := 255*n - a
			offset := y*dst.Stride + x*4
			dst.Pix[offset] = uint8((r + white) / n)
			dst.Pix[offset+1] = uint8((g + white) / n)
			dst.Pix[offset+2] = uint8((b + white) / n)
			dst.Pix[offset+3] = 255
		}
	}
	return dst
}

func registerEvidenceThumbnailRoute(mux *http.ServeMux, store evidence.Store, dataDir string, thumbs *thumbnailer) {
	mux.HandleFunc("GET /api/evidences/{id}/thumbnail", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeError(w, http.StatusServiceUnavailable, errors.New("el almacenamiento de evidencias no está disponible"))
			return
		}
		item, err := store.GetEvidence(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusNotFound, errors.New("evidencia no encontrada"))
			return
		}
		if !isLocalArtifact(item.FilePath, filepath.Join(dataDir, "evidences")) {
			writeError(w, http.StatusForbidden, errors.New("la evidencia está fuera del almacenamiento local permitido"))
			return
		}
		if !thumbnailSupported(item.Format) {
			// Formats the standard library cannot decode (webp) keep working
			// through the original file.
			serveLocalArtifact(w, r, item.FilePath, item.Format)
			return
		}
		path, err := thumbs.ensure(item)
		if err != nil {
			serveLocalArtifact(w, r, item.FilePath, item.Format)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "private, no-cache")
		http.ServeFile(w, r, path)
	})
}
