package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/zajuna-app/core/internal/jobs"
)

type testZajunaConnectionRequest struct {
	Username     string `json:"username"`
	DocumentType string `json:"documentType"`
}

func registerZajunaRoutes(mux *http.ServeMux, runtime *jobs.Runtime, dataDir string) {
	mux.HandleFunc("POST /api/zajuna/test-connection", func(w http.ResponseWriter, r *http.Request) {
		if runtime == nil {
			writeError(w, http.StatusServiceUnavailable, errors.New("el runtime de jobs no está disponible"))
			return
		}
		var request testZajunaConnectionRequest
		if r.Body != nil {
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil && !errors.Is(err, io.EOF) {
				writeError(w, http.StatusBadRequest, errors.New("el input de prueba de conexión es inválido"))
				return
			}
		}
		request.Username = strings.TrimSpace(request.Username)
		request.DocumentType = strings.ToUpper(strings.TrimSpace(request.DocumentType))
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
				request.DocumentType = strings.ToUpper(strings.TrimSpace(config.ZajunaDocumentType))
			}
		}
		if request.Username == "" {
			writeError(w, http.StatusBadRequest, errors.New("configura primero el usuario de Zajuna"))
			return
		}
		if request.DocumentType == "" {
			request.DocumentType = "CC"
		}
		job, err := runtime.Submit(r.Context(), "test-zajuna-connection", request)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusAccepted, toJobView(job))
	})
}
