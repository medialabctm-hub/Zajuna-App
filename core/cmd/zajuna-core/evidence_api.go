package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/zajuna-app/core/internal/evidence"
)

type evidenceView struct {
	ID         string `json:"id"`
	FichaID    string `json:"fichaId,omitempty"`
	ItemCode   string `json:"itemCode,omitempty"`
	SlotNumber int    `json:"slotNumber"`
	Name       string `json:"name"`
	FileKey    string `json:"fileKey,omitempty"`
	Format     string `json:"format"`
	Source     string `json:"source"`
	SHA256     string `json:"sha256"`
	CapturedAt string `json:"capturedAt"`
}

type evidenceGroupView struct {
	ID          string         `json:"id"`
	FichaID     string         `json:"fichaId"`
	GroupKey    string         `json:"groupKey"`
	Title       string         `json:"title"`
	Confidence  string         `json:"confidence"`
	Reason      string         `json:"reason"`
	EvidenceIDs []string       `json:"evidenceIds"`
	ItemCodes   []string       `json:"itemCodes"`
	Evidences   []evidenceView `json:"evidences"`
}

type rebuildEvidenceGroupsRequest struct {
	FichaID string `json:"fichaId"`
}

func registerEvidenceRoutes(mux *http.ServeMux, store evidence.Store, dataDir string) {
	thumbs := newThumbnailer(dataDir)
	registerEvidenceThumbnailRoute(mux, store, dataDir, thumbs)
	mux.HandleFunc("GET /api/evidences", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeError(w, http.StatusServiceUnavailable, errors.New("el almacenamiento de evidencias no está disponible"))
			return
		}
		fichaID := strings.TrimSpace(r.URL.Query().Get("fichaId"))
		// The per-ficha gallery must list every evidence of the ficha; the
		// global listing stays capped at 100.
		limit, maxLimit := 50, 100
		if fichaID != "" {
			limit, maxLimit = maxFichaEvidences, maxFichaEvidences
		}
		if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
			parsed, err := strconv.Atoi(rawLimit)
			if err != nil || parsed < 1 || parsed > maxLimit {
				writeError(w, http.StatusBadRequest, fmt.Errorf("limit debe ser un número entre 1 y %d", maxLimit))
				return
			}
			limit = parsed
		}
		var items []evidence.Record
		var err error
		if fichaID != "" {
			groupStore, ok := store.(evidence.GroupStore)
			if !ok {
				writeError(w, http.StatusNotImplemented, errors.New("el filtro por ficha no está disponible"))
				return
			}
			items, err = groupStore.ListEvidencesByFicha(r.Context(), fichaID, limit)
		} else {
			items, err = store.ListEvidences(r.Context(), limit)
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		views := make([]evidenceView, 0, len(items))
		for _, item := range items {
			views = append(views, toEvidenceView(item))
		}
		writeJSON(w, http.StatusOK, views)
	})

	mux.HandleFunc("GET /api/evidences/groups", func(w http.ResponseWriter, r *http.Request) {
		groupStore, ok := store.(evidence.GroupStore)
		if !ok {
			writeError(w, http.StatusNotImplemented, errors.New("los grupos de evidencia no están disponibles"))
			return
		}
		fichaID := strings.TrimSpace(r.URL.Query().Get("fichaId"))
		if fichaID == "" {
			writeError(w, http.StatusBadRequest, errors.New("fichaId es obligatorio"))
			return
		}
		groups, err := groupStore.ListEvidenceGroups(r.Context(), fichaID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		views := make([]evidenceGroupView, 0, len(groups))
		for _, group := range groups {
			views = append(views, toEvidenceGroupView(group))
		}
		writeJSON(w, http.StatusOK, views)
	})

	mux.HandleFunc("POST /api/evidences/groups/rebuild", func(w http.ResponseWriter, r *http.Request) {
		groupStore, ok := store.(evidence.GroupStore)
		if !ok {
			writeError(w, http.StatusNotImplemented, errors.New("los grupos de evidencia no están disponibles"))
			return
		}
		var request rebuildEvidenceGroupsRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("input de agrupación inválido"))
			return
		}
		request.FichaID = strings.TrimSpace(request.FichaID)
		if request.FichaID == "" {
			writeError(w, http.StatusBadRequest, errors.New("fichaId es obligatorio"))
			return
		}
		groups, err := groupStore.RebuildEvidenceGroups(r.Context(), request.FichaID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		views := make([]evidenceGroupView, 0, len(groups))
		for _, group := range groups {
			views = append(views, toEvidenceGroupView(group))
		}
		writeJSON(w, http.StatusOK, views)
	})

	mux.HandleFunc("POST /api/evidences/upload", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeError(w, http.StatusServiceUnavailable, errors.New("el almacenamiento de evidencias no está disponible"))
			return
		}
		const maxUploadSize = 25 << 20
		r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
		if err := r.ParseMultipartForm(maxUploadSize); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("el archivo supera el tamaño permitido o no es válido"))
			return
		}
		fichaID := strings.TrimSpace(r.FormValue("fichaId"))
		if fichaID == "" {
			writeError(w, http.StatusBadRequest, errors.New("fichaId es obligatorio"))
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			writeError(w, http.StatusBadRequest, errors.New("selecciona un archivo de evidencia"))
			return
		}
		defer file.Close()
		ext := strings.ToLower(filepath.Ext(header.Filename))
		allowed := map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".pdf": true, ".html": true}
		if !allowed[ext] {
			writeError(w, http.StatusBadRequest, errors.New("solo se permiten archivos PNG, JPG, PDF o HTML"))
			return
		}
		itemCode := strings.TrimSpace(r.FormValue("itemCode"))
		if len(itemCode) > 64 {
			writeError(w, http.StatusBadRequest, errors.New("el código de la actividad no es válido"))
			return
		}
		slotNumber := 1
		if rawSlot := strings.TrimSpace(r.FormValue("slotNumber")); rawSlot != "" {
			slotNumber, err = strconv.Atoi(rawSlot)
			if err != nil || slotNumber < 1 || slotNumber > 100 {
				writeError(w, http.StatusBadRequest, errors.New("el número de evidencia no es válido"))
				return
			}
		}
		name := strings.TrimSpace(r.FormValue("name"))
		if name == "" {
			name = strings.TrimSpace(header.Filename)
		}
		if len(name) > 240 {
			name = name[:240]
		}
		if name == "" {
			name = "Evidencia manual"
		}
		manualDir := filepath.Join(dataDir, "evidences", "manual", safeEvidencePathPart(fichaID))
		if err := os.MkdirAll(manualDir, 0o700); err != nil {
			writeError(w, http.StatusInternalServerError, errors.New("no se pudo preparar el almacenamiento local"))
			return
		}
		fileHash := sha256.New()
		fileName := fmt.Sprintf("manual-%d%s", time.Now().UTC().UnixNano(), ext)
		outputPath := filepath.Join(manualDir, fileName)
		output, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			writeError(w, http.StatusInternalServerError, errors.New("no se pudo crear el archivo local"))
			return
		}
		_, copyErr := io.Copy(io.MultiWriter(output, fileHash), file)
		closeErr := output.Close()
		if copyErr != nil || closeErr != nil {
			_ = os.Remove(outputPath)
			writeError(w, http.StatusBadRequest, errors.New("no se pudo guardar el archivo de evidencia"))
			return
		}
		sum := fileHash.Sum(nil)
		format := strings.TrimPrefix(ext, ".")
		if format == "jpg" {
			format = "jpeg"
		}
		contentHash := hex.EncodeToString(sum)
		// The ID is scoped to the slot: the same file uploaded for two ítems
		// must produce two rows instead of moving the first one.
		idSum := sha256.Sum256([]byte(fichaID + "\x00" + itemCode + "\x00" + strconv.Itoa(slotNumber) + "\x00" + contentHash))
		recordID := "evidence-manual-" + hex.EncodeToString(idSum[:8])
		if existing, err := store.GetEvidence(r.Context(), recordID); err == nil && existing.SHA256 == contentHash {
			if _, statErr := os.Stat(existing.FilePath); statErr == nil {
				_ = os.Remove(outputPath)
				if err := rebuildFichaGroups(r, store, fichaID); err != nil {
					writeError(w, http.StatusInternalServerError, err)
					return
				}
				writeJSON(w, http.StatusOK, toEvidenceView(existing))
				return
			}
		}
		metadata, _ := json.Marshal(map[string]any{"manual": true, "originalName": header.Filename})
		record := evidence.Record{ID: recordID, FichaID: fichaID, ItemCode: itemCode, SlotNumber: slotNumber, Name: name, FilePath: outputPath, Format: format, Source: "manual", SHA256: contentHash, Metadata: metadata, CapturedAt: time.Now().UTC()}
		if err := store.CreateEvidence(r.Context(), record); err != nil {
			_ = os.Remove(outputPath)
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if err := rebuildFichaGroups(r, store, fichaID); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusCreated, toEvidenceView(record))
	})

	mux.HandleFunc("DELETE /api/evidences/{id}", func(w http.ResponseWriter, r *http.Request) {
		deleteStore, ok := store.(evidence.DeleteStore)
		if !ok {
			writeError(w, http.StatusNotImplemented, errors.New("la eliminación de evidencias no está disponible"))
			return
		}
		item, err := store.GetEvidence(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusNotFound, errors.New("evidencia no encontrada"))
			return
		}
		// A missing file must not block retiring its row; an existing one must
		// live inside the evidence storage before the store may remove it.
		if _, statErr := os.Lstat(item.FilePath); statErr == nil && !isLocalArtifact(item.FilePath, filepath.Join(dataDir, "evidences")) {
			writeError(w, http.StatusForbidden, errors.New("la evidencia está fuera del almacenamiento local permitido"))
			return
		}
		// The store removes the file only when no other row references it:
		// one capture may back several checklist ítems.
		if _, err := deleteStore.DeleteEvidence(r.Context(), item.ID); err != nil {
			writeError(w, http.StatusInternalServerError, errors.New("no se pudo retirar el registro de la evidencia"))
			return
		}
		thumbs.remove(item.ID, "")
		if item.FichaID != "" {
			if err := rebuildFichaGroups(r, store, item.FichaID); err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "id": item.ID})
	})

	mux.HandleFunc("POST /api/evidences/clear", func(w http.ResponseWriter, r *http.Request) {
		clearStore, ok := store.(evidence.ClearStore)
		if !ok {
			writeError(w, http.StatusNotImplemented, errors.New("el reinicio de evidencias no está disponible"))
			return
		}
		var body struct {
			FichaID string `json:"fichaId"`
		}
		if r.ContentLength > 0 {
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
				writeError(w, http.StatusBadRequest, errors.New("cuerpo de reinicio inválido"))
				return
			}
		}
		deletedRows, deletedFiles, err := clearStore.ClearEvidences(r.Context(), strings.TrimSpace(body.FichaID))
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		thumbs.prune(r.Context(), store)
		writeJSON(w, http.StatusOK, map[string]any{
			"cleared":      true,
			"deletedRows":  deletedRows,
			"deletedFiles": deletedFiles,
			"fichaId":      strings.TrimSpace(body.FichaID),
			"note":         "Actualizar la aplicación no borra evidencias; use este endpoint o Ajustes para reiniciarlas.",
		})
	})

	mux.HandleFunc("GET /api/evidences/{id}/download", func(w http.ResponseWriter, r *http.Request) {
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
		serveLocalArtifact(w, r, item.FilePath, item.Format)
	})
}

// maxFichaEvidences matches the ceiling of the SQLite per-ficha listing.
const maxFichaEvidences = 10000

// rebuildFichaGroups keeps the persisted evidence groups in sync after a
// manual change so the gallery and grouped reports see it immediately.
func rebuildFichaGroups(r *http.Request, store evidence.Store, fichaID string) error {
	groupStore, ok := store.(evidence.GroupStore)
	if !ok {
		return nil
	}
	if _, err := groupStore.RebuildEvidenceGroups(r.Context(), fichaID); err != nil {
		return errors.New("el cambio se aplicó, pero no se pudo actualizar la agrupación; usa «Revisar agrupación»")
	}
	return nil
}

func toEvidenceView(item evidence.Record) evidenceView {
	return evidenceView{ID: item.ID, FichaID: item.FichaID, ItemCode: item.ItemCode, SlotNumber: item.SlotNumber, Name: item.Name, FileKey: evidenceFileKey(item.FilePath), Format: item.Format, Source: item.Source, SHA256: item.SHA256, CapturedAt: item.CapturedAt.Format("2006-01-02T15:04:05.999Z07:00")}
}

// evidenceFileKey lets clients tell whether two rows share a file without
// learning the absolute path, which reveals the OS user name.
func evidenceFileKey(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(strings.ToLower(filepath.ToSlash(filepath.Clean(path)))))
	return hex.EncodeToString(sum[:12])
}

func toEvidenceGroupView(group evidence.Group) evidenceGroupView {
	evidences := make([]evidenceView, 0, len(group.Evidences))
	for _, item := range group.Evidences {
		evidences = append(evidences, toEvidenceView(item))
	}
	return evidenceGroupView{
		ID: group.ID, FichaID: group.FichaID, GroupKey: group.GroupKey, Title: group.Title,
		Confidence: group.Confidence, Reason: group.Reason, EvidenceIDs: group.EvidenceIDs,
		ItemCodes: group.ItemCodes, Evidences: evidences,
	}
}

func isLocalArtifact(path, root string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	// Lexical checks alone can be bypassed by a symlink inside the evidence
	// directory. Resolve both paths before authorizing a read/delete.
	realRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return false
	}
	realPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(realRoot, realPath)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func serveLocalArtifact(w http.ResponseWriter, r *http.Request, path, format string) {
	if _, err := os.Stat(path); err != nil {
		writeError(w, http.StatusNotFound, errors.New("archivo de evidencia no encontrado"))
		return
	}
	if format == "html" {
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; img-src data:; base-uri 'none'")
	}
	http.ServeFile(w, r, path)
}

func safeEvidencePathPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	value = strings.NewReplacer("\\", "_", "/", "_", ":", "_", "..", "_").Replace(value)
	return value
}
