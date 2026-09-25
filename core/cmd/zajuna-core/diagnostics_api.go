package main

import (
	"context"
	"database/sql"
	"errors"
	"github.com/zajuna-app/core/internal/storage/sqlite"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/zajuna-app/core/internal/capture"
	"github.com/zajuna-app/core/internal/jobs"
	"github.com/zajuna-app/core/internal/secrets"
)

type diagnosticsDB interface {
	DB() *sql.DB
}

type diagnosticCheck struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Detail      string `json:"detail,omitempty"`
	CheckedAt   string `json:"checkedAt"`
}

type diagnosticIncident struct {
	JobID     string `json:"jobId"`
	Type      string `json:"type"`
	ErrorCode string `json:"errorCode,omitempty"`
	UpdatedAt string `json:"updatedAt"`
}

type diagnosticsView struct {
	GeneratedAt string               `json:"generatedAt"`
	Checks      []diagnosticCheck    `json:"checks"`
	Incidents   []diagnosticIncident `json:"incidents"`
}

func registerDiagnosticsRoutes(mux *http.ServeMux, store any, dataDir string, _ secrets.Store) {
	mux.HandleFunc("GET /api/diagnostics", func(w http.ResponseWriter, r *http.Request) {
		view := collectDiagnostics(r.Context(), store, dataDir)
		writeJSON(w, http.StatusOK, view)
	})
}

func collectDiagnostics(ctx context.Context, store any, dataDir string) diagnosticsView {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	view := diagnosticsView{GeneratedAt: now, Checks: make([]diagnosticCheck, 0, 6), Incidents: make([]diagnosticIncident, 0)}
	view.Checks = append(view.Checks, diagnosticCheck{
		ID: "core", Title: "Aplicación local", Description: "La aplicación responde y mantiene la interfaz en este equipo.",
		Status: "ok", Detail: "Servicio local activo", CheckedAt: now,
	})

	if dbStore, ok := store.(diagnosticsDB); ok && dbStore.DB() != nil {
		view.Checks = append(view.Checks, checkSQLite(ctx, dbStore.DB(), now))
	} else {
		view.Checks = append(view.Checks, diagnosticCheck{
			ID: "sqlite", Title: "Base local", Description: "Comprobación rápida de la base de datos local.",
			Status: "error", Detail: "El almacenamiento no está disponible", CheckedAt: now,
		})
	}

	config, err := readConfig(dataDir)
	credentialCheck := diagnosticCheck{
		ID: "credentials", Title: "Credencial protegida", Description: "Solo se comprueba la presencia de la credencial; nunca se devuelve su valor.",
		Status: "warn", Detail: "Configura la conexión de Zajuna", CheckedAt: now,
	}
	if err != nil {
		credentialCheck.Status = "error"
		credentialCheck.Detail = "No se pudo leer la configuración local"
	} else if config.CredentialsStored {
		credentialCheck.Status = "ok"
		credentialCheck.Detail = "Presente en el almacén seguro del sistema"
	}
	view.Checks = append(view.Checks, credentialCheck)

	browser := capture.Resolve("")
	browserCheck := diagnosticCheck{
		ID: "chromium", Title: "Navegador incluido", Description: "El navegador incluido permite realizar capturas autenticadas sin depender de un navegador externo.",
		Status: "warn", Detail: "Instala el runtime local antes de capturar", CheckedAt: now,
	}
	if browser.Installed() {
		browserCheck.Status = "ok"
		browserCheck.Detail = "Driver y navegador instalados"
	}
	view.Checks = append(view.Checks, browserCheck)

	view.Checks = append(view.Checks, checkStorage(dataDir, now))

	if lister, ok := store.(jobLister); ok {
		jobsList, err := lister.ListJobs(ctx, 100)
		if err == nil {
			failed := 0
			for _, item := range jobsList {
				if item.Status != jobs.StatusFailed {
					continue
				}
				failed++
				if len(view.Incidents) < 5 {
					view.Incidents = append(view.Incidents, diagnosticIncident{
						JobID: item.ID, Type: item.Type, ErrorCode: item.ErrorCode, UpdatedAt: item.UpdatedAt.Format(time.RFC3339Nano),
					})
				}
			}
			jobsCheck := diagnosticCheck{
				ID: "jobs", Title: "Trabajos recientes", Description: "Los fallos recientes se muestran como incidencias sin incluir mensajes o secretos.",
				Status: "ok", Detail: "Sin fallos recientes", CheckedAt: now,
			}
			if failed > 0 {
				jobsCheck.Status = "warn"
				jobsCheck.Detail = formatCount(failed, "trabajo necesita atención", "trabajos necesitan atención")
			}
			view.Checks = append(view.Checks, jobsCheck)
		}
	}

	return view
}

func checkSQLite(ctx context.Context, db *sql.DB, checkedAt string) diagnosticCheck {
	check := diagnosticCheck{
		ID: "sqlite", Title: "Base local", Description: "Comprobación rápida de integridad de la base de datos local y su esquema.",
		Status: "error", Detail: "No se pudo comprobar la base local", CheckedAt: checkedAt,
	}
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		check.Detail = "La base de datos local no responde"
		return check
	}
	var result string
	if err := db.QueryRowContext(pingCtx, `PRAGMA quick_check`).Scan(&result); err != nil {
		check.Detail = "La comprobación de la base de datos local falló"
		return check
	}
	if strings.EqualFold(strings.TrimSpace(result), "ok") {
		var version int
		if err := db.QueryRowContext(pingCtx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&version); err != nil {
			check.Status = "error"
			check.Detail = "No se pudo leer la versión del esquema"
			return check
		}
		check.Status = "ok"
		check.Detail = formatSchemaDetail(version)
	} else {
		check.Status = "error"
		check.Detail = "La base de datos local reportó una inconsistencia"
	}
	return check
}

func formatSchemaDetail(version int) string {
	if version >= sqlite.CurrentSchemaVersion() {
		return "Integridad correcta · esquema v" + strconv.Itoa(version)
	}
	return "Integridad correcta · esquema antiguo v" + strconv.Itoa(version)
}

func checkStorage(dataDir, checkedAt string) diagnosticCheck {
	check := diagnosticCheck{
		ID: "storage", Title: "Almacenamiento local", Description: "La carpeta de datos conserva fichas, evidencias y reportes en este equipo.",
		Status: "error", Detail: "La carpeta de datos no está disponible", CheckedAt: checkedAt,
	}
	if _, err := os.Stat(dataDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			check.Detail = "La carpeta aún no se ha creado"
		}
		return check
	}
	backupCount := 0
	if entries, err := os.ReadDir(filepath.Join(dataDir, "backups")); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".zip") {
				backupCount++
			}
		}
	}
	check.Status = "ok"
	check.Detail = formatCount(backupCount, "copia local disponible", "copias locales disponibles")
	return check
}

func formatCount(count int, singular, plural string) string {
	if count == 1 {
		return "1 " + singular
	}
	return formatInt(count) + " " + plural
}

func formatInt(value int) string {
	if value < 0 {
		return "0"
	}
	return strconv.Itoa(value)
}
