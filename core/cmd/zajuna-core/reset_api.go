package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zajuna-app/core/internal/secrets"
	"github.com/zajuna-app/core/internal/storage/backup"
)

type resetRequest struct {
	BackupFirst       bool `json:"backupFirst"`
	ForgetCredentials bool `json:"forgetCredentials"`
}

type resetView struct {
	Staged     bool   `json:"staged"`
	Restarting bool   `json:"restarting"`
	BackupName string `json:"backupName,omitempty"`
}

type credentialDeleter interface {
	Delete(user string) error
}

// registerAppRoutes exposes installation info and the full local reset. The
// reset is staged (like a backup restore) and applied on the next start,
// before SQLite opens, because the live database and evidence files cannot be
// removed safely while workers and Chromium may still be using them.
func registerAppRoutes(mux *http.ServeMux, dataDir string, credentials secrets.Store, manager *backup.Manager) {
	mux.HandleFunc("GET /api/app/info", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"version": appVersion, "dataDir": displayDataDir(dataDir), "supervised": supervised(),
			"resetPending": backup.ResetPending(dataDir),
		})
	})

	mux.HandleFunc("POST /api/app/reset", func(w http.ResponseWriter, r *http.Request) {
		var request resetRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("la solicitud de restablecimiento es inválida"))
			return
		}
		view := resetView{Staged: true, Restarting: supervised()}
		if request.BackupFirst {
			if manager == nil {
				writeError(w, http.StatusServiceUnavailable, errors.New("las copias locales no están disponibles; desmarca la copia de seguridad o inténtalo de nuevo"))
				return
			}
			record, err := manager.Create(r.Context())
			if err != nil {
				writeError(w, http.StatusInternalServerError, errors.New("no se pudo crear la copia de seguridad previa; no se borró ningún dato"))
				return
			}
			view.BackupName = filepath.Base(record.Path)
		}
		if request.ForgetCredentials {
			if config, err := readConfig(dataDir); err == nil && strings.TrimSpace(config.ZajunaUsername) != "" {
				if deleter, ok := credentials.(credentialDeleter); ok {
					if err := deleter.Delete(config.ZajunaUsername); err != nil {
						log.Printf("no se pudo borrar la credencial guardada durante el restablecimiento: %v", err)
					}
				}
			}
		}
		if err := backup.StageReset(dataDir); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
		// Let the response reach the browser before the server shuts down.
		go func() {
			time.Sleep(300 * time.Millisecond)
			requestShutdown()
		}()
	})
}

// displayDataDir shortens the data directory to an environment-relative form
// (%LOCALAPPDATA%\ZajunaApp, ~/.local/share/zajuna-app) so the API never
// reveals the OS user name. Unknown locations fall back to the folder name.
func displayDataDir(dataDir string) string {
	clean := filepath.Clean(dataDir)
	bases := []struct{ root, label string }{{os.Getenv("LOCALAPPDATA"), "%LOCALAPPDATA%"}}
	if home, err := os.UserHomeDir(); err == nil {
		bases = append(bases, struct{ root, label string }{home, "~"})
	}
	for _, base := range bases {
		if strings.TrimSpace(base.root) == "" {
			continue
		}
		relative, err := filepath.Rel(filepath.Clean(base.root), clean)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
			continue
		}
		if relative == "." {
			return base.label
		}
		return base.label + string(filepath.Separator) + relative
	}
	return filepath.Base(clean)
}
