package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/zajuna-app/core/internal/scheduler"
)

type createScheduleRequest struct {
	ID              string          `json:"id"`
	WorkerType      string          `json:"workerType"`
	Input           json.RawMessage `json:"input"`
	IntervalSeconds int             `json:"intervalSeconds"`
	Enabled         *bool           `json:"enabled"`
	NextRunAt       string          `json:"nextRunAt"`
}

type setScheduleEnabledRequest struct {
	Enabled bool `json:"enabled"`
}

type scheduleView struct {
	ID              string          `json:"id"`
	WorkerType      string          `json:"workerType"`
	Input           json.RawMessage `json:"input"`
	IntervalSeconds int64           `json:"intervalSeconds"`
	Enabled         bool            `json:"enabled"`
	NextRunAt       string          `json:"nextRunAt"`
	LastRunAt       *string         `json:"lastRunAt,omitempty"`
	LastJobID       string          `json:"lastJobId,omitempty"`
	CreatedAt       string          `json:"createdAt"`
	UpdatedAt       string          `json:"updatedAt"`
}

func registerScheduleRoutes(mux *http.ServeMux, store scheduler.Store) {
	mux.HandleFunc("GET /api/schedules", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeError(w, http.StatusServiceUnavailable, errors.New("el scheduler local no está disponible"))
			return
		}
		items, err := store.ListSchedules(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		views := make([]scheduleView, 0, len(items))
		for _, item := range items {
			views = append(views, toScheduleView(item))
		}
		writeJSON(w, http.StatusOK, views)
	})

	mux.HandleFunc("POST /api/schedules", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeError(w, http.StatusServiceUnavailable, errors.New("el scheduler local no está disponible"))
			return
		}
		var request createScheduleRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("el JSON del scheduler es inválido"))
			return
		}
		request.WorkerType = strings.TrimSpace(request.WorkerType)
		if request.WorkerType == "" || request.IntervalSeconds <= 0 {
			writeError(w, http.StatusBadRequest, errors.New("workerType e intervalSeconds son obligatorios"))
			return
		}
		if len(request.Input) == 0 {
			request.Input = json.RawMessage(`{}`)
		}
		var input any
		if err := json.Unmarshal(request.Input, &input); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("input de scheduler inválido"))
			return
		}
		enabled := true
		if request.Enabled != nil {
			enabled = *request.Enabled
		}
		now := time.Now().UTC()
		nextRunAt := now.Add(time.Duration(request.IntervalSeconds) * time.Second)
		if request.NextRunAt != "" {
			parsed, err := time.Parse(time.RFC3339, request.NextRunAt)
			if err != nil {
				writeError(w, http.StatusBadRequest, errors.New("nextRunAt debe estar en formato RFC3339"))
				return
			}
			nextRunAt = parsed.UTC()
		}
		id := strings.TrimSpace(request.ID)
		if id == "" {
			id = scheduler.NewID()
		}
		item := scheduler.Schedule{ID: id, WorkerType: request.WorkerType, Input: request.Input, Interval: time.Duration(request.IntervalSeconds) * time.Second, Enabled: enabled, NextRunAt: nextRunAt, CreatedAt: now, UpdatedAt: now}
		if err := store.CreateSchedule(r.Context(), item); err != nil {
			writeError(w, http.StatusConflict, err)
			return
		}
		writeJSON(w, http.StatusCreated, toScheduleView(item))
	})

	mux.HandleFunc("POST /api/schedules/{id}/enabled", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeError(w, http.StatusServiceUnavailable, errors.New("el scheduler local no está disponible"))
			return
		}
		var request setScheduleEnabledRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("el JSON de enabled es inválido"))
			return
		}
		if err := store.SetScheduleEnabled(r.Context(), r.PathValue("id"), request.Enabled); err != nil {
			writeError(w, http.StatusNotFound, errors.New("schedule no encontrado"))
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"enabled": request.Enabled})
	})
}

func toScheduleView(item scheduler.Schedule) scheduleView {
	view := scheduleView{
		ID: item.ID, WorkerType: item.WorkerType, Input: item.Input,
		IntervalSeconds: int64(item.Interval / time.Second), Enabled: item.Enabled,
		NextRunAt: item.NextRunAt.Format(time.RFC3339Nano),
		LastJobID: item.LastJobID,
		CreatedAt: item.CreatedAt.Format(time.RFC3339Nano), UpdatedAt: item.UpdatedAt.Format(time.RFC3339Nano),
	}
	if item.LastRunAt != nil {
		value := item.LastRunAt.Format(time.RFC3339Nano)
		view.LastRunAt = &value
	}
	return view
}
