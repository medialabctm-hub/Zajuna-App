package main

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/zajuna-app/core/internal/checklist"
	"github.com/zajuna-app/core/internal/coursemaps"
	"github.com/zajuna-app/core/internal/evidence"
	"github.com/zajuna-app/core/internal/storage/sqlite"
)

// checklistGuideStore is what the guides endpoint reads; the SQLite store
// implements it.
type checklistGuideStore interface {
	GetFicha(context.Context, string) (sqlite.FichaRecord, error)
	GetCourseMap(context.Context, string) (coursemaps.Record, error)
	GetChecklistDashboard(context.Context, string) (sqlite.ChecklistDashboard, error)
	EvidenceReviewReport(context.Context, string) (evidence.ReviewReport, error)
	CaptureAbsenceReasons(context.Context, string) (map[string]string, error)
}

type checklistGuidesView struct {
	FichaID  string            `json:"fichaId"`
	MapReady bool              `json:"mapReady"`
	Guides   []checklist.Guide `json:"guides"`
	// Advice: recommendations (content with errors) that are not pending.
	Advice []checklist.Guide `json:"advice"`
}

// registerChecklistGuideRoutes exposes the guides for the items the app
// cannot complete by itself (see checklist.DetectGuides). It works for any
// ficha: the guides come from its live state, not from a fixed list.
func registerChecklistGuideRoutes(mux *http.ServeMux, store checklistGuideStore) {
	mux.HandleFunc("GET /api/checklist/guides", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeError(w, http.StatusServiceUnavailable, errors.New("las guías del checklist no están disponibles"))
			return
		}
		fichaID := strings.TrimSpace(r.URL.Query().Get("fichaId"))
		if fichaID == "" {
			writeError(w, http.StatusBadRequest, errors.New("fichaId es obligatorio"))
			return
		}
		view, err := buildChecklistGuides(r.Context(), store, fichaID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusNotFound, errors.New("la ficha seleccionada no existe"))
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
	})
}

func buildChecklistGuides(ctx context.Context, store checklistGuideStore, fichaID string) (checklistGuidesView, error) {
	ficha, err := store.GetFicha(ctx, fichaID)
	if err != nil {
		return checklistGuidesView{}, err
	}
	input := checklist.GuideInput{}
	dashboard, err := store.GetChecklistDashboard(ctx, ficha.ID)
	if err != nil {
		return checklistGuidesView{}, err
	}
	for _, item := range dashboard.Items {
		input.Items = append(input.Items, checklist.GuideItemState{ItemCode: item.ItemCode, Status: item.Status})
	}
	report, err := store.EvidenceReviewReport(ctx, ficha.ID)
	if err != nil {
		return checklistGuidesView{}, err
	}
	for _, entry := range report.Evidences {
		empty, contentError := false, ""
		for _, reason := range entry.Reasons {
			switch reason.Code {
			case evidence.ReasonEmptySection:
				empty = true
			case evidence.ReasonSheetErrors:
				contentError = strings.TrimPrefix(reason.Message, "La hoja publicada en Zajuna tiene errores: ")
			}
		}
		input.Evidences = append(input.Evidences, checklist.GuideEvidence{
			ItemCode: entry.ItemCode, Slot: entry.SlotNumber, Approved: entry.Status == evidence.ReviewApproved,
			Superseded: entry.Superseded, EmptySection: empty, ContentError: contentError,
		})
	}
	if input.Absences, err = store.CaptureAbsenceReasons(ctx, ficha.ID); err != nil {
		return checklistGuidesView{}, err
	}
	record, err := store.GetCourseMap(ctx, ficha.CourseID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// Without a map no route can be called missing yet.
	case err != nil:
		return checklistGuidesView{}, err
	default:
		input.MapReady = true
		var selected map[string]bool
		if activityStore, ok := store.(checklistActivityStore); ok {
			if selected, err = activityStore.ListSelectedActivityIDs(ctx, ficha.ID); err != nil {
				return checklistGuidesView{}, err
			}
		}
		targets, _, err := checklist.BuildCaptureTargetsForActivities(record, selected)
		if err != nil {
			return checklistGuidesView{}, err
		}
		var reviews []checklist.RouteReview
		if reviewStore, ok := store.(checklistRouteReviewStore); ok {
			if reviews, err = reviewStore.ListRouteReviews(ctx, ficha.ID); err != nil {
				return checklistGuidesView{}, err
			}
		}
		input.Targets = checklist.ApplyRouteReviews(targets, reviews)
	}
	return checklistGuidesView{FichaID: ficha.ID, MapReady: input.MapReady, Guides: checklist.DetectGuides(input), Advice: checklist.DetectAdvice(input)}, nil
}
