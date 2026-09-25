package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/zajuna-app/core/internal/checklist"
	"github.com/zajuna-app/core/internal/coursemaps"
	"github.com/zajuna-app/core/internal/jobs"
	"github.com/zajuna-app/core/internal/storage/sqlite"
)

type checklistCaptureStore interface {
	GetFicha(context.Context, string) (sqlite.FichaRecord, error)
	GetCourseMap(context.Context, string) (coursemaps.Record, error)
}

type checklistActivityStore interface {
	ListSelectedActivityIDs(context.Context, string) (map[string]bool, error)
	ReplaceSelectedActivities(context.Context, string, []coursemaps.Activity) error
}

type checklistCaptureRequest struct {
	FichaID      string   `json:"fichaId"`
	Username     string   `json:"username,omitempty"`
	DocumentType string   `json:"documentType,omitempty"`
	ItemCodes    []string `json:"itemCodes,omitempty"`
	MaxTargets   int      `json:"maxTargets,omitempty"`
	FullPage     *bool    `json:"fullPage,omitempty"`
	ReuseSession *bool    `json:"reuseSession,omitempty"`
	AutoRenew    *bool    `json:"autoRenew,omitempty"`
}

var errChecklistCourseMapMissing = errors.New("checklist course map is missing")

type checklistActivitySelectionRequest struct {
	FichaID             string   `json:"fichaId"`
	SelectedActivityIDs []string `json:"selectedActivityIds"`
}

type checklistRouteReviewStore interface {
	ListRouteReviews(context.Context, string) ([]checklist.RouteReview, error)
	UpsertRouteReview(context.Context, checklist.RouteReview) error
}

type checklistRouteReviewRequest struct {
	FichaID        string `json:"fichaId"`
	RouteKey       string `json:"routeKey"`
	Status         string `json:"status"`
	ManualURL      string `json:"manualUrl,omitempty"`
	ManualSelector string `json:"manualSelector,omitempty"`
	Note           string `json:"note,omitempty"`
}

func registerChecklistCaptureRoutes(mux *http.ServeMux, store checklistCaptureStore, runtime *jobs.Runtime, dataDir string) {
	mux.HandleFunc("GET /api/checklist/activities", func(w http.ResponseWriter, r *http.Request) {
		activityStore, ok := store.(checklistActivityStore)
		if !ok {
			writeError(w, http.StatusServiceUnavailable, errors.New("la selección de actividades no está disponible"))
			return
		}
		fichaID := strings.TrimSpace(r.URL.Query().Get("fichaId"))
		if fichaID == "" {
			writeError(w, http.StatusBadRequest, errors.New("fichaId es obligatorio"))
			return
		}
		// A newly synced ficha has no local course map yet. That is an
		// expected first-run state, so keep this query successful and expose
		// the discovery action instead of returning a generic 404 that the
		// checklist renders as a broken request.
		ficha, err := store.GetFicha(r.Context(), fichaID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusNotFound, errors.New("la ficha seleccionada no existe"))
				return
			}
			writeChecklistActivitiesError(w, err)
			return
		}
		record, err := store.GetCourseMap(r.Context(), ficha.CourseID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				// Ignore any stale selection left from an older map. It must not
				// look active while the current course has no discovered routes.
				writeJSON(w, http.StatusOK, checklistActivitiesEmptyView(ficha.ID, ficha.CourseID))
				return
			}
			writeChecklistActivitiesError(w, err)
			return
		}
		selected, err := activityStore.ListSelectedActivityIDs(r.Context(), ficha.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, checklistActivitiesView(ficha.ID, ficha.CourseID, record, selected))
	})

	mux.HandleFunc("PUT /api/checklist/activities", func(w http.ResponseWriter, r *http.Request) {
		activityStore, ok := store.(checklistActivityStore)
		if !ok {
			writeError(w, http.StatusServiceUnavailable, errors.New("la selección de actividades no está disponible"))
			return
		}
		var request checklistActivitySelectionRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10))
		if err := decoder.Decode(&request); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("la selección de actividades es inválida"))
			return
		}
		request.FichaID = strings.TrimSpace(request.FichaID)
		if request.FichaID == "" {
			writeError(w, http.StatusBadRequest, errors.New("fichaId es obligatorio"))
			return
		}
		ficha, record, _, err := readChecklistActivities(r.Context(), store, activityStore, request.FichaID)
		if err != nil {
			writeChecklistActivitiesError(w, err)
			return
		}
		available := make(map[string]coursemaps.Activity)
		for _, activity := range coursemaps.Activities(record) {
			available[activity.ID] = activity
		}
		selected := make([]coursemaps.Activity, 0, len(request.SelectedActivityIDs))
		seen := make(map[string]bool, len(request.SelectedActivityIDs))
		for _, activityID := range request.SelectedActivityIDs {
			activityID = strings.TrimSpace(activityID)
			if activityID == "" || seen[activityID] {
				continue
			}
			activity, exists := available[activityID]
			if !exists {
				writeError(w, http.StatusBadRequest, fmt.Errorf("la actividad %s no pertenece al mapa del curso", activityID))
				return
			}
			if !activity.Technical {
				// Transversal competencies are taught by other instructors: their
				// dates, grades and feedback are not this instructor's evidence.
				writeError(w, http.StatusBadRequest, fmt.Errorf("«%s» es de una competencia transversal y no se puede seleccionar: su evidencia corresponde a otro instructor", activity.Title))
				return
			}
			seen[activityID] = true
			selected = append(selected, activity)
		}
		if err := activityStore.ReplaceSelectedActivities(r.Context(), ficha.ID, selected); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		selectedIDs, err := activityStore.ListSelectedActivityIDs(r.Context(), ficha.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, checklistActivitiesView(ficha.ID, ficha.CourseID, record, selectedIDs))
	})

	mux.HandleFunc("GET /api/checklist/targets", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeError(w, http.StatusServiceUnavailable, errors.New("el almacenamiento del checklist no está disponible"))
			return
		}
		fichaID := strings.TrimSpace(r.URL.Query().Get("fichaId"))
		if fichaID == "" {
			writeError(w, http.StatusBadRequest, errors.New("fichaId es obligatorio"))
			return
		}
		ficha, err := store.GetFicha(r.Context(), fichaID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusNotFound, errors.New("la ficha seleccionada no existe"))
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		record, err := store.GetCourseMap(r.Context(), ficha.CourseID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				// A ficha without a map is a normal first-run state, not a
				// missing resource. Return a usable empty plan so the UI can
				// render the explicit "Buscar rutas" action instead of turning
				// the route panel into a generic query error.
				writeJSON(w, http.StatusOK, map[string]any{
					"fichaId":  ficha.ID,
					"courseId": ficha.CourseID,
					"mapReady": false,
					"discovery": map[string]string{
						"status":  "required",
						"action":  "discover-course-maps",
						"message": "Busca las rutas del curso antes de preparar evidencias.",
					},
					"summary": checklist.CapturePlanSummary{},
					"targets": []checklist.CaptureTarget{},
				})
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		selectedActivityIDs := map[string]bool(nil)
		selectionConfigured := false
		if activityStore, ok := store.(checklistActivityStore); ok {
			selectedActivityIDs, err = activityStore.ListSelectedActivityIDs(r.Context(), ficha.ID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			selectionConfigured = len(checklist.TechnicalSelectionForRecord(record, selectedActivityIDs)) > 0
		}
		targets, summary, err := checklist.BuildCaptureTargetsForActivities(record, selectedActivityIDs)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if reviewStore, ok := store.(checklistRouteReviewStore); ok {
			reviews, reviewErr := reviewStore.ListRouteReviews(r.Context(), ficha.ID)
			if reviewErr != nil {
				writeError(w, http.StatusInternalServerError, reviewErr)
				return
			}
			targets = checklist.ApplyRouteReviews(targets, reviews)
		} else {
			targets = checklist.ApplyRouteReviews(targets, nil)
		}
		writeJSON(w, http.StatusOK, map[string]any{"fichaId": ficha.ID, "courseId": ficha.CourseID, "mapReady": true, "summary": summary, "targets": targets, "selectionConfigured": selectionConfigured})
	})

	mux.HandleFunc("GET /api/checklist/reviews", func(w http.ResponseWriter, r *http.Request) {
		reviewStore, ok := store.(checklistRouteReviewStore)
		if !ok {
			writeError(w, http.StatusNotImplemented, errors.New("la revisión de rutas no está disponible"))
			return
		}
		fichaID := strings.TrimSpace(r.URL.Query().Get("fichaId"))
		if fichaID == "" {
			writeError(w, http.StatusBadRequest, errors.New("fichaId es obligatorio"))
			return
		}
		reviews, err := reviewStore.ListRouteReviews(r.Context(), fichaID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, reviews)
	})

	mux.HandleFunc("PUT /api/checklist/reviews", func(w http.ResponseWriter, r *http.Request) {
		reviewStore, ok := store.(checklistRouteReviewStore)
		if !ok {
			writeError(w, http.StatusNotImplemented, errors.New("la revisión de rutas no está disponible"))
			return
		}
		var request checklistRouteReviewRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&request); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("la revisión de ruta es inválida"))
			return
		}
		request.FichaID = strings.TrimSpace(request.FichaID)
		request.RouteKey = strings.TrimSpace(request.RouteKey)
		request.Status = strings.ToLower(strings.TrimSpace(request.Status))
		request.ManualURL = strings.TrimSpace(request.ManualURL)
		request.ManualSelector = strings.TrimSpace(request.ManualSelector)
		request.Note = strings.TrimSpace(request.Note)
		if request.FichaID == "" || request.RouteKey == "" || !checklist.ValidRouteReviewStatus(request.Status) {
			writeError(w, http.StatusBadRequest, errors.New("ficha, ruta y estado válido son obligatorios"))
			return
		}
		ficha, record, selected, err := readChecklistReviewContext(r.Context(), store, request.FichaID)
		if err != nil {
			writeChecklistActivitiesError(w, err)
			return
		}
		targets, _, err := checklist.BuildCaptureTargetsForActivities(record, selected)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		knownRoute := false
		for _, target := range targets {
			if checklist.RouteKey(target) == request.RouteKey {
				knownRoute = true
				break
			}
		}
		if !knownRoute {
			writeError(w, http.StatusBadRequest, errors.New("la ruta no pertenece al mapa actual de la ficha"))
			return
		}
		if request.ManualURL != "" && !sameCourseOrigin(record.CourseURL, request.ManualURL) {
			writeError(w, http.StatusBadRequest, errors.New("el enlace manual debe pertenecer al mismo curso de Zajuna"))
			return
		}
		if err := reviewStore.UpsertRouteReview(r.Context(), checklist.RouteReview{FichaID: ficha.ID, RouteKey: request.RouteKey, Status: request.Status, ManualURL: request.ManualURL, ManualSelector: request.ManualSelector, Note: request.Note}); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		reviews, err := reviewStore.ListRouteReviews(r.Context(), ficha.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, reviews)
	})

	mux.HandleFunc("POST /api/checklist/capture", func(w http.ResponseWriter, r *http.Request) {
		if runtime == nil {
			writeError(w, http.StatusServiceUnavailable, errors.New("el runtime de jobs no está disponible"))
			return
		}
		var request checklistCaptureRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("el input de captura del checklist es inválido"))
			return
		}
		request.FichaID = strings.TrimSpace(request.FichaID)
		if request.FichaID == "" {
			writeError(w, http.StatusBadRequest, errors.New("fichaId es obligatorio"))
			return
		}
		if request.Username == "" || request.DocumentType == "" {
			config, err := readConfig(dataDir)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			if request.Username == "" {
				request.Username = config.ZajunaUsername
			}
			if request.DocumentType == "" {
				request.DocumentType = config.ZajunaDocumentType
			}
		}
		request.Username = strings.TrimSpace(request.Username)
		request.DocumentType = strings.ToUpper(strings.TrimSpace(request.DocumentType))
		if request.DocumentType == "" {
			request.DocumentType = "CC"
		}
		if request.Username == "" {
			writeError(w, http.StatusBadRequest, errors.New("configura primero el usuario de Zajuna"))
			return
		}
		// Fail before creating a job when the course map is missing. This keeps
		// the jobs list free of guaranteed failures and gives the client a
		// machine-readable action to launch discovery first.
		if store != nil {
			ficha, fichaErr := store.GetFicha(r.Context(), request.FichaID)
			if fichaErr != nil {
				if errors.Is(fichaErr, sql.ErrNoRows) {
					writeError(w, http.StatusNotFound, errors.New("la ficha seleccionada no existe"))
					return
				}
				writeError(w, http.StatusInternalServerError, fichaErr)
				return
			}
			if _, mapErr := store.GetCourseMap(r.Context(), ficha.CourseID); mapErr != nil {
				if errors.Is(mapErr, sql.ErrNoRows) {
					writeJSON(w, http.StatusConflict, map[string]any{
						"code":     "course_map_required",
						"error":    "la ficha todavía no tiene un mapa de rutas; busca las rutas antes de preparar evidencias",
						"action":   "discover-course-maps",
						"fichaId":  ficha.ID,
						"courseId": ficha.CourseID,
					})
					return
				}
				writeError(w, http.StatusInternalServerError, mapErr)
				return
			}
		}
		if request.MaxTargets < 0 || request.MaxTargets > 200 {
			writeError(w, http.StatusBadRequest, errors.New("maxTargets debe estar entre 0 y 200"))
			return
		}
		if settingsStore, ok := store.(appSettingsStore); ok {
			if settings, settingsErr := loadSettings(r.Context(), settingsStore); settingsErr == nil {
				if request.FullPage == nil {
					value := settings.Capture.FullPage
					request.FullPage = &value
				}
				if request.ReuseSession == nil {
					value := settings.Capture.ReuseSession
					request.ReuseSession = &value
				}
				if request.AutoRenew == nil {
					value := settings.Session.AutoRenew
					request.AutoRenew = &value
				}
			}
		}
		job, err := runtime.Submit(r.Context(), "capture-checklist", request)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusAccepted, toJobView(job))
	})
}

func readChecklistReviewContext(ctx context.Context, store checklistCaptureStore, fichaID string) (sqlite.FichaRecord, coursemaps.Record, map[string]bool, error) {
	ficha, err := store.GetFicha(ctx, fichaID)
	if err != nil {
		return sqlite.FichaRecord{}, coursemaps.Record{}, nil, err
	}
	record, err := store.GetCourseMap(ctx, ficha.CourseID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ficha, coursemaps.Record{}, nil, errChecklistCourseMapMissing
		}
		return sqlite.FichaRecord{}, coursemaps.Record{}, nil, err
	}
	selected := map[string]bool(nil)
	if activityStore, ok := store.(checklistActivityStore); ok {
		selected, err = activityStore.ListSelectedActivityIDs(ctx, ficha.ID)
		if err != nil {
			return sqlite.FichaRecord{}, coursemaps.Record{}, nil, err
		}
	}
	return ficha, record, selected, nil
}

func sameCourseOrigin(courseURL, candidate string) bool {
	base, baseErr := url.Parse(strings.TrimSpace(courseURL))
	target, targetErr := url.Parse(strings.TrimSpace(candidate))
	if baseErr != nil || targetErr != nil || base.Scheme == "" || base.Host == "" || target.Scheme == "" || target.Host == "" {
		return false
	}
	return strings.EqualFold(base.Scheme, target.Scheme) && strings.EqualFold(base.Host, target.Host) && strings.HasPrefix(target.Path, "/zajuna/")
}

func readChecklistActivities(ctx context.Context, store checklistCaptureStore, activityStore checklistActivityStore, fichaID string) (sqlite.FichaRecord, coursemaps.Record, map[string]bool, error) {
	ficha, err := store.GetFicha(ctx, fichaID)
	if err != nil {
		return sqlite.FichaRecord{}, coursemaps.Record{}, nil, err
	}
	record, err := store.GetCourseMap(ctx, ficha.CourseID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ficha, coursemaps.Record{}, nil, errChecklistCourseMapMissing
		}
		return sqlite.FichaRecord{}, coursemaps.Record{}, nil, err
	}
	selected, err := activityStore.ListSelectedActivityIDs(ctx, ficha.ID)
	if err != nil {
		return sqlite.FichaRecord{}, coursemaps.Record{}, nil, err
	}
	return ficha, record, selected, nil
}

func writeChecklistActivitiesError(w http.ResponseWriter, err error) {
	if errors.Is(err, errChecklistCourseMapMissing) {
		writeJSON(w, http.StatusConflict, map[string]string{
			"code":   "course_map_required",
			"error":  "la ficha todavía no tiene un mapa de rutas; busca las rutas antes de seleccionar actividades",
			"action": "discover-course-maps",
		})
		return
	}
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, errors.New("la ficha o su mapa de curso no existe"))
		return
	}
	writeError(w, http.StatusInternalServerError, err)
}

type checklistActivityView struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	URL          string `json:"url"`
	PhaseName    string `json:"phaseName,omitempty"`
	PhaseSection int    `json:"phaseSection,omitempty"`
	Subsection   string `json:"subsection,omitempty"`
	Technical    bool   `json:"technical"`
	Selected     bool   `json:"selected"`
	// Selectable is false for transversal competencies (another instructor's
	// evidence); BlockedReason explains it in the UI.
	Selectable    bool   `json:"selectable"`
	BlockedReason string `json:"blockedReason,omitempty"`
}

func checklistActivitiesView(fichaID, courseID string, record coursemaps.Record, selected map[string]bool) map[string]any {
	activities := coursemaps.Activities(record)
	views := make([]checklistActivityView, 0, len(activities))
	selectedCount := 0
	for _, activity := range activities {
		view := checklistActivityView{
			ID: activity.ID, Title: activity.Title, URL: activity.URL, PhaseName: activity.PhaseName,
			PhaseSection: activity.PhaseSection, Subsection: activity.Subsection,
			Technical: activity.Technical, Selectable: activity.Technical,
			// A transversal activity saved by an older version is ignored.
			Selected: selected[activity.ID] && activity.Technical,
		}
		if !activity.Technical {
			view.BlockedReason = "Competencia transversal: la orienta otro instructor, así que sus fechas, calificaciones y retroalimentación no son evidencia tuya."
		}
		if view.Selected {
			selectedCount++
		}
		views = append(views, view)
	}
	return map[string]any{
		"fichaId": fichaID, "courseId": courseID, "activities": views,
		"mapReady": true, "selectedCount": selectedCount, "selectionConfigured": selectedCount > 0,
		"slotsPerItem": checklist.ActivityEvidenceSlots(),
	}
}

func checklistActivitiesEmptyView(fichaID, courseID string) map[string]any {
	return map[string]any{
		"fichaId": fichaID, "courseId": courseID, "mapReady": false,
		"activities": []checklistActivityView{}, "selectedCount": 0,
		"selectionConfigured": false,
		"discovery": map[string]string{
			"status":  "required",
			"action":  "discover-course-maps",
			"message": "Busca las rutas del curso antes de seleccionar actividades.",
		},
	}
}
