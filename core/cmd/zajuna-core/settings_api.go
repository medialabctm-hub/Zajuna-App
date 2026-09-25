package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/zajuna-app/core/internal/workers"
)

const appSettingsKey = "ui_preferences"

type appSettingsStore interface {
	GetAppSetting(context.Context, string) (string, error)
	SetAppSetting(context.Context, string, string) error
}

type settingsView struct {
	Session       sessionSettings      `json:"session"`
	Capture       captureSettings      `json:"capture"`
	Notifications notificationSettings `json:"notifications"`
	Storage       storageSettings      `json:"storage"`
}

type sessionSettings struct {
	AutoRenew bool `json:"autoRenew"`
}

type captureSettings struct {
	FullPage     bool `json:"fullPage"`
	ReuseSession bool `json:"reuseSession"`
	Motion       bool `json:"motion"`
}

type notificationSettings struct {
	JobCompleted bool `json:"jobCompleted"`
	NeedsReview  bool `json:"needsReview"`
}

type storageSettings struct {
	RetentionKeep int `json:"retentionKeep"`
	RetentionDays int `json:"retentionDays"`
}

func defaultSettings() settingsView {
	return settingsView{
		Session:       sessionSettings{AutoRenew: true},
		Capture:       captureSettings{FullPage: true, ReuseSession: true, Motion: true},
		Notifications: notificationSettings{JobCompleted: true, NeedsReview: true},
		Storage:       storageSettings{RetentionKeep: 5, RetentionDays: 30},
	}
}

func registerSettingsRoutes(mux *http.ServeMux, store appSettingsStore) {
	mux.HandleFunc("GET /api/settings", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeError(w, http.StatusServiceUnavailable, errors.New("las preferencias locales no están disponibles"))
			return
		}
		settings, err := loadSettings(r.Context(), store)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, settings)
	})

	mux.HandleFunc("PUT /api/settings", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeError(w, http.StatusServiceUnavailable, errors.New("las preferencias locales no están disponibles"))
			return
		}
		request := defaultSettings()
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("el JSON de preferencias es inválido"))
			return
		}
		if request.Storage.RetentionKeep < 1 || request.Storage.RetentionKeep > 1000 || request.Storage.RetentionDays < 1 || request.Storage.RetentionDays > 3650 {
			writeError(w, http.StatusBadRequest, errors.New("la política de retención está fuera de rango"))
			return
		}
		if err := saveSettings(r.Context(), store, request); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, request)
	})
}

func loadSettings(ctx context.Context, store appSettingsStore) (settingsView, error) {
	settings := defaultSettings()
	contents, err := store.GetAppSetting(ctx, appSettingsKey)
	if err != nil {
		return settingsView{}, err
	}
	if contents == "" {
		return settings, nil
	}
	if err := json.Unmarshal([]byte(contents), &settings); err != nil {
		// A malformed preference must not prevent the app from opening. Keep
		// safe defaults and let the next save repair the value.
		return defaultSettings(), nil
	}
	return settings, nil
}

// capturePreferencesLoader exposes the persisted capture and session
// preferences to the checklist worker; a read error keeps the defaults so a
// capture never fails because of a preference.
func capturePreferencesLoader(store appSettingsStore) func(context.Context) workers.CapturePreferences {
	return func(ctx context.Context) workers.CapturePreferences {
		settings, err := loadSettings(ctx, store)
		if err != nil {
			return workers.DefaultCapturePreferences()
		}
		return workers.CapturePreferences{
			FullPage:     settings.Capture.FullPage,
			ReuseSession: settings.Capture.ReuseSession,
			AutoRenew:    settings.Session.AutoRenew,
		}
	}
}

func saveSettings(ctx context.Context, store appSettingsStore, settings settingsView) error {
	contents, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	return store.SetAppSetting(ctx, appSettingsKey, string(contents))
}
