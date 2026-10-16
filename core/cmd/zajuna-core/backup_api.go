package main

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"time"

	"github.com/zajuna-app/core/internal/storage/backup"
)

type backupView struct {
	Name      string `json:"name"`
	CreatedAt string `json:"createdAt"`
	SizeBytes int64  `json:"sizeBytes"`
}

type restoreView struct {
	Staged          bool   `json:"staged"`
	RestartRequired bool   `json:"restartRequired"`
	BackupName      string `json:"backupName"`
	SafetyBackup    string `json:"safetyBackup"`
}

type cleanupBackupsRequest struct {
	Keep          int `json:"keep"`
	OlderThanDays int `json:"olderThanDays"`
}

func registerBackupRoutes(mux *http.ServeMux, manager *backup.Manager) {
	mux.HandleFunc("GET /api/backups", func(w http.ResponseWriter, r *http.Request) {
		if manager == nil {
			writeError(w, http.StatusServiceUnavailable, errors.New("las copias locales no están disponibles"))
			return
		}
		records, err := manager.List()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		views := make([]backupView, 0, len(records))
		for _, record := range records {
			views = append(views, backupView{Name: filepath.Base(record.Path), CreatedAt: record.CreatedAt.Format("2006-01-02T15:04:05.999Z07:00"), SizeBytes: record.SizeBytes})
		}
		writeJSON(w, http.StatusOK, views)
	})

	mux.HandleFunc("POST /api/backups", func(w http.ResponseWriter, r *http.Request) {
		if manager == nil {
			writeError(w, http.StatusServiceUnavailable, errors.New("las copias locales no están disponibles"))
			return
		}
		record, err := manager.Create(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusCreated, backupView{Name: filepath.Base(record.Path), CreatedAt: record.CreatedAt.Format("2006-01-02T15:04:05.999Z07:00"), SizeBytes: record.SizeBytes})
	})

	mux.HandleFunc("GET /api/backups/{name}/download", func(w http.ResponseWriter, r *http.Request) {
		if manager == nil {
			writeError(w, http.StatusServiceUnavailable, errors.New("las copias locales no están disponibles"))
			return
		}
		record, err := manager.Resolve(r.PathValue("name"))
		if err != nil {
			writeError(w, http.StatusNotFound, errors.New("backup no encontrado"))
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filepath.Base(record.Path)}))
		http.ServeFile(w, r, record.Path)
	})

	mux.HandleFunc("DELETE /api/backups/{name}", func(w http.ResponseWriter, r *http.Request) {
		if manager == nil {
			writeError(w, http.StatusServiceUnavailable, errors.New("las copias locales no están disponibles"))
			return
		}
		if err := manager.Delete(r.PathValue("name")); err != nil {
			writeError(w, http.StatusNotFound, errors.New("backup no encontrado"))
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
	})

	mux.HandleFunc("POST /api/backups/cleanup", func(w http.ResponseWriter, r *http.Request) {
		if manager == nil {
			writeError(w, http.StatusServiceUnavailable, errors.New("las copias locales no están disponibles"))
			return
		}
		request := cleanupBackupsRequest{Keep: 5, OlderThanDays: 30}
		if r.Body != nil {
			decoder := json.NewDecoder(r.Body)
			if err := decoder.Decode(&request); err != nil && !errors.Is(err, io.EOF) {
				writeError(w, http.StatusBadRequest, errors.New("la política de limpieza es inválida"))
				return
			}
		}
		if request.Keep < 1 || request.Keep > 1000 || request.OlderThanDays < 1 || request.OlderThanDays > 3650 {
			writeError(w, http.StatusBadRequest, errors.New("keep y olderThanDays están fuera de rango"))
			return
		}
		deleted, err := manager.Cleanup(request.Keep, time.Duration(request.OlderThanDays)*24*time.Hour)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted, "keep": request.Keep, "olderThanDays": request.OlderThanDays})
	})

	mux.HandleFunc("POST /api/backups/{name}/restore", func(w http.ResponseWriter, r *http.Request) {
		if manager == nil {
			writeError(w, http.StatusServiceUnavailable, errors.New("las copias locales no están disponibles"))
			return
		}
		// An unknown name is a 404 and must not leave a safety snapshot behind
		// nor echo the absolute path of the data folder in the error.
		if _, err := manager.Resolve(r.PathValue("name")); err != nil {
			writeError(w, http.StatusNotFound, errors.New("backup no encontrado"))
			return
		}
		// Always create a safety snapshot before staging a restore. The app keeps
		// running against the current DB; the staged archive applies on restart.
		safety, err := manager.Create(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		record, err := manager.StageRestore(r.Context(), r.PathValue("name"))
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusAccepted, restoreView{Staged: true, RestartRequired: true, BackupName: filepath.Base(record.Path), SafetyBackup: filepath.Base(safety.Path)})
	})
}
