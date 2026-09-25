package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/zajuna-app/core/internal/evidence"
)

// evidenceReviewStore is implemented by the SQLite store (see
// internal/storage/sqlite/evidence_reviews.go).
type evidenceReviewStore interface {
	EvidenceReviewReport(ctx context.Context, fichaID string) (evidence.ReviewReport, error)
	VerifyEvidenceReviews(ctx context.Context, fichaID string) (evidence.ReviewReport, error)
	SetEvidenceReview(ctx context.Context, evidenceID, status, note string) (evidence.ReviewEntry, error)
}

func registerEvidenceReviewRoutes(mux *http.ServeMux, store evidence.Store) {
	reviewStore, _ := store.(evidenceReviewStore)
	unavailable := func(w http.ResponseWriter) bool {
		if reviewStore == nil {
			writeError(w, http.StatusNotImplemented, errors.New("la revisión de evidencias no está disponible"))
			return true
		}
		return false
	}

	mux.HandleFunc("GET /api/evidences/review", func(w http.ResponseWriter, r *http.Request) {
		if unavailable(w) {
			return
		}
		fichaID := strings.TrimSpace(r.URL.Query().Get("fichaId"))
		if fichaID == "" {
			writeError(w, http.StatusBadRequest, errors.New("fichaId es obligatorio"))
			return
		}
		report, err := reviewStore.EvidenceReviewReport(r.Context(), fichaID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, report)
	})

	mux.HandleFunc("POST /api/evidences/verify", func(w http.ResponseWriter, r *http.Request) {
		if unavailable(w) {
			return
		}
		var body struct {
			FichaID string `json:"fichaId"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("cuerpo de verificación inválido"))
			return
		}
		fichaID := strings.TrimSpace(body.FichaID)
		if fichaID == "" {
			writeError(w, http.StatusBadRequest, errors.New("fichaId es obligatorio"))
			return
		}
		report, err := reviewStore.VerifyEvidenceReviews(r.Context(), fichaID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, report)
	})

	mux.HandleFunc("PUT /api/evidences/{id}/review", func(w http.ResponseWriter, r *http.Request) {
		if unavailable(w) {
			return
		}
		var body struct {
			Status string `json:"status"`
			Note   string `json:"note"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("cuerpo de revisión inválido"))
			return
		}
		status := strings.TrimSpace(body.Status)
		if status != evidence.ReviewApproved && status != evidence.ReviewRejected && status != evidence.ReviewPending {
			writeError(w, http.StatusBadRequest, errors.New("status debe ser approved, rejected o pending"))
			return
		}
		note := strings.TrimSpace(body.Note)
		if runes := []rune(note); len(runes) > 1000 {
			note = string(runes[:1000])
		}
		entry, err := reviewStore.SetEvidenceReview(r.Context(), r.PathValue("id"), status, note)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, errors.New("evidencia no encontrada"))
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, entry)
	})
}
