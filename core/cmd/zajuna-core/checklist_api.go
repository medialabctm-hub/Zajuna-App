package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/zajuna-app/core/internal/evidence"
	"github.com/zajuna-app/core/internal/storage/sqlite"
)

type checklistService interface {
	GetActiveFichaID(ctx context.Context) (string, error)
	SetActiveFicha(ctx context.Context, fichaID string) error
	GetChecklistDashboard(ctx context.Context, fichaID string) (sqlite.ChecklistDashboard, error)
	SetChecklistItemStatus(ctx context.Context, fichaID, itemCode, status string) error
}

type checklistDetailService interface {
	GetChecklistItemDetail(ctx context.Context, fichaID, itemCode string) (sqlite.ChecklistItemDetail, error)
}

type activeFichaRequest struct {
	FichaID string `json:"fichaId"`
}

type checklistStatusRequest struct {
	FichaID string `json:"fichaId"`
	Status  string `json:"status"`
}

func registerChecklistRoutes(mux *http.ServeMux, service checklistService) {
	detailService, _ := service.(checklistDetailService)
	mux.HandleFunc("GET /api/checklist/dashboard", func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeError(w, http.StatusServiceUnavailable, errors.New("el checklist local no está disponible"))
			return
		}
		dashboard, err := service.GetChecklistDashboard(r.Context(), strings.TrimSpace(r.URL.Query().Get("fichaId")))
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusNotFound, errors.New("todavía no hay una ficha activa; sincroniza primero"))
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, toChecklistDashboardView(dashboard))
	})

	mux.HandleFunc("GET /api/checklist/items/{itemCode}", func(w http.ResponseWriter, r *http.Request) {
		if detailService == nil {
			writeError(w, http.StatusServiceUnavailable, errors.New("el detalle local de checklist no está disponible"))
			return
		}
		fichaID := strings.TrimSpace(r.URL.Query().Get("fichaId"))
		if fichaID == "" {
			var err error
			fichaID, err = service.GetActiveFichaID(r.Context())
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
		}
		detail, err := detailService.GetChecklistItemDetail(r.Context(), fichaID, r.PathValue("itemCode"))
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusNotFound, errors.New("el ítem de checklist no existe para esta ficha"))
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		events := make([]checklistItemEventView, 0, len(detail.Events))
		for _, event := range detail.Events {
			events = append(events, checklistItemEventView{
				ID: event.ID, FromStatus: event.FromStatus, ToStatus: event.ToStatus,
				Source: event.Source, Note: event.Note, JobID: event.JobID,
				CreatedAt: event.CreatedAt.Format(time.RFC3339Nano),
			})
		}
		writeJSON(w, http.StatusOK, checklistItemDetailView{Item: toChecklistItemView(detail.Item), Events: events})
	})

	mux.HandleFunc("POST /api/fichas/active", func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeError(w, http.StatusServiceUnavailable, errors.New("el checklist local no está disponible"))
			return
		}
		var request activeFichaRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || strings.TrimSpace(request.FichaID) == "" {
			writeError(w, http.StatusBadRequest, errors.New("fichaId es obligatorio"))
			return
		}
		if err := service.SetActiveFicha(r.Context(), strings.TrimSpace(request.FichaID)); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusNotFound, errors.New("la ficha seleccionada no existe"))
				return
			}
			writeError(w, http.StatusBadRequest, err)
			return
		}
		dashboard, err := service.GetChecklistDashboard(r.Context(), request.FichaID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, toChecklistDashboardView(dashboard))
	})

	mux.HandleFunc("PATCH /api/checklist/items/{itemCode}/status", func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeError(w, http.StatusServiceUnavailable, errors.New("el checklist local no está disponible"))
			return
		}
		var request checklistStatusRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("estado de checklist inválido"))
			return
		}
		fichaID := strings.TrimSpace(request.FichaID)
		if fichaID == "" {
			var err error
			fichaID, err = service.GetActiveFichaID(r.Context())
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
		}
		status := strings.ToUpper(strings.TrimSpace(request.Status))
		if err := service.SetChecklistItemStatus(r.Context(), fichaID, r.PathValue("itemCode"), status); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusNotFound, errors.New("el ítem de checklist no existe para esta ficha"))
				return
			}
			if errors.Is(err, sqlite.ErrItemNotFulfillable) {
				writeError(w, http.StatusConflict, err)
				return
			}
			writeError(w, http.StatusBadRequest, err)
			return
		}
		dashboard, err := service.GetChecklistDashboard(r.Context(), fichaID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, toChecklistDashboardView(dashboard))
	})
}

type checklistDashboardView struct {
	ActiveFichaID string                             `json:"activeFichaId"`
	Ficha         fichaView                          `json:"ficha"`
	Summary       sqlite.ChecklistSummary            `json:"summary"`
	Categories    []sqlite.ChecklistCategoryProgress `json:"categories"`
	Items         []checklistItemView                `json:"items"`
}

type checklistItemView struct {
	ID            int            `json:"id"`
	FichaID       string         `json:"fichaId"`
	CategoryCode  string         `json:"categoryCode"`
	CategoryLabel string         `json:"categoryLabel"`
	ItemCode      string         `json:"itemCode"`
	Description   string         `json:"description"`
	GroupName     string         `json:"groupName"`
	MaxEvidences  int            `json:"maxEvidences"`
	EvidenceCount int            `json:"evidenceCount"`
	Evidences     []evidenceView `json:"evidences,omitempty"`
	Status        string         `json:"status"`
	Position      int            `json:"position"`
	UpdatedAt     string         `json:"updatedAt"`
}

type checklistItemEventView struct {
	ID         int64  `json:"id"`
	FromStatus string `json:"fromStatus,omitempty"`
	ToStatus   string `json:"toStatus"`
	Source     string `json:"source"`
	Note       string `json:"note,omitempty"`
	JobID      string `json:"jobId,omitempty"`
	CreatedAt  string `json:"createdAt"`
}

type checklistItemDetailView struct {
	Item   checklistItemView        `json:"item"`
	Events []checklistItemEventView `json:"events"`
}

func toChecklistItemView(item sqlite.ChecklistItemRecord) checklistItemView {
	return checklistItemView{
		ID: item.ID, FichaID: item.FichaID, CategoryCode: item.CategoryCode,
		CategoryLabel: item.CategoryLabel, ItemCode: item.ItemCode,
		Description: item.Description, GroupName: item.GroupName,
		MaxEvidences: item.MaxEvidences, EvidenceCount: item.EvidenceCount,
		Evidences: toEvidenceViews(item.Evidences), Status: item.Status,
		Position: item.Position, UpdatedAt: item.UpdatedAt.Format(time.RFC3339Nano),
	}
}

func toChecklistDashboardView(dashboard sqlite.ChecklistDashboard) checklistDashboardView {
	items := make([]checklistItemView, 0, len(dashboard.Items))
	for _, item := range dashboard.Items {
		items = append(items, toChecklistItemView(item))
	}
	return checklistDashboardView{
		ActiveFichaID: dashboard.ActiveID,
		Ficha:         toFichaView(dashboard.Ficha),
		Summary:       dashboard.Summary,
		Categories:    dashboard.Categories,
		Items:         items,
	}
}

func toEvidenceViews(items []evidence.Record) []evidenceView {
	views := make([]evidenceView, 0, len(items))
	for _, item := range items {
		views = append(views, toEvidenceView(item))
	}
	return views
}
