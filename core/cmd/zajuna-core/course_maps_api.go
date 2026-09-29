package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/zajuna-app/core/internal/coursemaps"
	"github.com/zajuna-app/core/internal/jobs"
	"github.com/zajuna-app/core/internal/storage/sqlite"
	"github.com/zajuna-app/core/internal/zajuna"
)

type discoverCourseMapsRequest struct {
	Username     string   `json:"username"`
	DocumentType string   `json:"documentType"`
	CourseIDs    []string `json:"courseIds"`
	// FichaID elige la ficha cuyas rutas se buscan. Sin courseIds ni fichaId se
	// usa la ficha activa. AllFichas recorre todas las fichas locales y solo lo
	// pide la preparación del primer arranque: ninguna acción normal lo usa.
	FichaID         string `json:"fichaId,omitempty"`
	AllFichas       bool   `json:"allFichas,omitempty"`
	MaxDepth        int    `json:"maxDepth"`
	MaxPages        int    `json:"maxPages"`
	MaxLinksPerPage int    `json:"maxLinksPerPage"`
}

// courseMapFichaStore resuelve la ficha objetivo del descubrimiento.
type courseMapFichaStore interface {
	GetFicha(ctx context.Context, fichaID string) (sqlite.FichaRecord, error)
	GetActiveFichaID(ctx context.Context) (string, error)
}

type importCourseActivitiesRequest struct {
	CourseID   string                `json:"courseId"`
	ProfileURL string                `json:"profileUrl,omitempty"`
	PageLinks  []zajuna.ActivityLink `json:"pageLinks"`
	Jump       []zajuna.ActivityLink `json:"jump"`
}

func registerCourseMapRoutes(mux *http.ServeMux, store coursemaps.Store, fichas fichaLister, runtime *jobs.Runtime, dataDir string) {
	mux.HandleFunc("GET /api/course-maps", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeError(w, http.StatusServiceUnavailable, errors.New("el almacenamiento de mapas no está disponible"))
			return
		}
		limit := 50
		if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
			parsed, err := strconv.Atoi(rawLimit)
			if err != nil || parsed < 1 || parsed > 100 {
				writeError(w, http.StatusBadRequest, errors.New("limit debe ser un número entre 1 y 100"))
				return
			}
			limit = parsed
		}
		items, err := store.ListCourseMaps(r.Context(), limit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, items)
	})

	mux.HandleFunc("GET /api/course-maps/{courseId}", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeError(w, http.StatusServiceUnavailable, errors.New("el almacenamiento de mapas no está disponible"))
			return
		}
		courseID := strings.TrimSpace(r.PathValue("courseId"))
		if courseID == "" {
			writeError(w, http.StatusBadRequest, errors.New("el curso es obligatorio"))
			return
		}
		item, err := store.GetCourseMap(r.Context(), courseID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusNotFound, errors.New("no existe un mapa local para ese curso"))
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
	})

	mux.HandleFunc("POST /api/course-maps/import-activities", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeError(w, http.StatusServiceUnavailable, errors.New("el almacenamiento de mapas no está disponible"))
			return
		}
		var request importCourseActivitiesRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("el archivo de actividades es inválido"))
			return
		}
		if len(request.PageLinks)+len(request.Jump) > 5000 {
			writeError(w, http.StatusBadRequest, errors.New("el mapa importado supera el límite de 5000 enlaces"))
			return
		}
		activities := make([]zajuna.ActivityLink, 0, len(request.PageLinks)+len(request.Jump))
		seen := make(map[string]bool, cap(activities))
		for _, activity := range append(request.PageLinks, request.Jump...) {
			activity.URL = strings.TrimSpace(activity.URL)
			activity.Label = strings.TrimSpace(activity.Label)
			if activity.URL == "" || seen[activity.URL] {
				continue
			}
			seen[activity.URL] = true
			activities = append(activities, activity)
		}
		if len(activities) == 0 {
			writeError(w, http.StatusBadRequest, errors.New("el mapa importado no contiene enlaces de actividades"))
			return
		}
		record, err := zajuna.BuildCourseMapFromActivities(request.CourseID, request.ProfileURL, activities)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if err := store.CreateOrReplaceCourseMap(r.Context(), record); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusCreated, record)
	})

	mux.HandleFunc("POST /api/course-maps/discover", func(w http.ResponseWriter, r *http.Request) {
		if runtime == nil {
			writeError(w, http.StatusServiceUnavailable, errors.New("el runtime de jobs no está disponible"))
			return
		}
		var request discoverCourseMapsRequest
		if r.Body != nil {
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil && !errors.Is(err, io.EOF) {
				writeError(w, http.StatusBadRequest, errors.New("el input de descubrimiento es inválido"))
				return
			}
		}
		request.Username = strings.TrimSpace(request.Username)
		if request.Username == "" {
			config, err := readConfig(dataDir)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			request.Username = config.ZajunaUsername
		}
		if request.Username == "" {
			writeError(w, http.StatusBadRequest, errors.New("configura primero el usuario de Zajuna"))
			return
		}
		if request.DocumentType == "" {
			request.DocumentType = "CC"
		}
		courseIDs, fichaID, status, err := resolveDiscoveryCourses(r.Context(), request, fichas)
		if err != nil {
			writeError(w, status, err)
			return
		}
		// El job guarda la ficha resuelta, no la pedida: así un reintento busca
		// las rutas de la misma ficha aunque después cambie la ficha activa.
		request.CourseIDs, request.FichaID = courseIDs, fichaID
		if request.MaxDepth < 0 || request.MaxDepth > 6 || request.MaxPages < 0 || request.MaxPages > 500 || request.MaxLinksPerPage < 0 || request.MaxLinksPerPage > 1000 {
			writeError(w, http.StatusBadRequest, errors.New("los límites de descubrimiento están fuera de rango"))
			return
		}
		job, err := runtime.Submit(r.Context(), "discover-course-maps", request)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusAccepted, toJobView(job))
	})
}

// resolveDiscoveryCourses decide qué cursos se descubren. Buscar rutas es una
// acción sobre una ficha: courseIds explícitos, si no la ficha indicada y, si
// tampoco, la ficha activa. Recorrer todas las fichas exige allFichas.
func resolveDiscoveryCourses(ctx context.Context, request discoverCourseMapsRequest, fichas fichaLister) ([]string, string, int, error) {
	if courseIDs := uniqueNonEmpty(request.CourseIDs); len(courseIDs) > 0 {
		return courseIDs, strings.TrimSpace(request.FichaID), http.StatusOK, nil
	}
	if request.AllFichas {
		if fichas == nil {
			return nil, "", http.StatusServiceUnavailable, errors.New("el almacenamiento de fichas no está disponible")
		}
		items, err := fichas.ListFichas(ctx, 100)
		if err != nil {
			return nil, "", http.StatusInternalServerError, err
		}
		courseIDs := make([]string, 0, len(items))
		for _, item := range items {
			courseIDs = append(courseIDs, item.CourseID)
		}
		if courseIDs = uniqueNonEmpty(courseIDs); len(courseIDs) == 0 {
			return nil, "", http.StatusBadRequest, errors.New("sin cursos locales; sincroniza primero Mis cursos")
		}
		return courseIDs, "", http.StatusOK, nil
	}
	store, ok := fichas.(courseMapFichaStore)
	if !ok {
		return nil, "", http.StatusServiceUnavailable, errors.New("el almacenamiento de fichas no está disponible")
	}
	fichaID := strings.TrimSpace(request.FichaID)
	if fichaID == "" {
		active, err := store.GetActiveFichaID(ctx)
		if err != nil {
			return nil, "", http.StatusInternalServerError, err
		}
		if fichaID = strings.TrimSpace(active); fichaID == "" {
			return nil, "", http.StatusBadRequest, errors.New("selecciona una ficha antes de buscar rutas")
		}
	}
	ficha, err := store.GetFicha(ctx, fichaID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, "", http.StatusNotFound, errors.New("la ficha seleccionada no existe")
		}
		return nil, "", http.StatusInternalServerError, err
	}
	courseID := strings.TrimSpace(ficha.CourseID)
	if courseID == "" {
		return nil, "", http.StatusBadRequest, errors.New("la ficha no tiene un curso asociado; sincroniza tus fichas otra vez")
	}
	return []string{courseID}, ficha.ID, http.StatusOK, nil
}

func uniqueNonEmpty(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}
