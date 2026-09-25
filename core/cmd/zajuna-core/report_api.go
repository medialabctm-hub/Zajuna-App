package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/zajuna-app/core/internal/jobs"
	"github.com/zajuna-app/core/internal/reports"
)

type reportView struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Format    string `json:"format"`
	Status    string `json:"status"`
	SHA256    string `json:"sha256"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type createReportRequest struct {
	Title         string `json:"title"`
	Format        string `json:"format"`
	FichaID       string `json:"fichaId,omitempty"`
	OutputPath    string `json:"outputPath,omitempty"`
	EvidenceLimit int    `json:"evidenceLimit,omitempty"`
}

func registerReportRoutes(mux *http.ServeMux, store reports.Store, runtime *jobs.Runtime, dataDir string) {
	mux.HandleFunc("GET /api/reports", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeError(w, http.StatusServiceUnavailable, errors.New("el almacenamiento de reportes no está disponible"))
			return
		}
		limit := 1000
		if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
			parsed, err := strconv.Atoi(rawLimit)
			if err != nil || parsed < 1 || parsed > 10000 {
				writeError(w, http.StatusBadRequest, errors.New("limit debe ser un número entre 1 y 10000"))
				return
			}
			limit = parsed
		}
		items, err := store.ListReports(r.Context(), limit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		views := make([]reportView, 0, len(items))
		for _, item := range items {
			views = append(views, toReportView(item))
		}
		writeJSON(w, http.StatusOK, views)
	})

	mux.HandleFunc("POST /api/reports", func(w http.ResponseWriter, r *http.Request) {
		if runtime == nil {
			writeError(w, http.StatusServiceUnavailable, errors.New("el runtime de jobs no está disponible"))
			return
		}
		var request createReportRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("input de reporte inválido"))
			return
		}
		job, err := runtime.Submit(r.Context(), "export-report", request)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusAccepted, toJobView(job))
	})

	mux.HandleFunc("GET /api/reports/{id}/download", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeError(w, http.StatusServiceUnavailable, errors.New("el almacenamiento de reportes no está disponible"))
			return
		}
		item, err := store.GetReport(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusNotFound, errors.New("reporte no encontrado"))
			return
		}
		if !isLocalArtifact(item.FilePath, filepath.Join(dataDir, "reports")) {
			writeError(w, http.StatusForbidden, errors.New("el reporte está fuera del almacenamiento local permitido"))
			return
		}
		serveLocalArtifact(w, r, item.FilePath, item.Format)
	})
}

func toReportView(item reports.Record) reportView {
	return reportView{ID: item.ID, Name: item.Name, Format: item.Format, Status: item.Status, SHA256: item.SHA256, CreatedAt: item.CreatedAt.Format("2006-01-02T15:04:05.999Z07:00"), UpdatedAt: item.UpdatedAt.Format("2006-01-02T15:04:05.999Z07:00")}
}
