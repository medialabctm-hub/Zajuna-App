package main

import (
	"net/http"
	"testing"
)

func firstRunPending(t *testing.T, base string, client *http.Client) bool {
	t.Helper()
	response := doJSON(t, client, http.MethodGet, base+"/api/setup/status", "")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("setup status = %d, body = %s", response.StatusCode, response.Body)
	}
	var status struct {
		SetupComplete   bool `json:"setupComplete"`
		FirstRunPending bool `json:"firstRunPending"`
	}
	decodeJSON(t, response.Body, &status)
	return status.FirstRunPending
}

func TestFirstRunPendingLifecycle(t *testing.T) {
	server, _ := newDiscoveryServer(t, nil)
	client := server.Client()
	setup := `{"zajunaUsername":"qa-user","zajunaDocumentType":"CC","zajunaPassword":"secret"}`

	if firstRunPending(t, server.URL, client) {
		t.Fatal("antes del Setup no hay primer arranque pendiente")
	}
	if response := doJSON(t, client, http.MethodPost, server.URL+"/api/setup", setup); response.StatusCode != http.StatusOK {
		t.Fatalf("setup = %d, body = %s", response.StatusCode, response.Body)
	}
	if !firstRunPending(t, server.URL, client) {
		t.Fatal("al completar el Setup por primera vez el primer arranque queda pendiente")
	}

	// Cambiar credenciales con el primer arranque aún pendiente no lo apaga.
	if response := doJSON(t, client, http.MethodPost, server.URL+"/api/setup", setup); response.StatusCode != http.StatusOK {
		t.Fatalf("segundo setup = %d, body = %s", response.StatusCode, response.Body)
	}
	if !firstRunPending(t, server.URL, client) {
		t.Fatal("repetir el Setup no debe apagar el primer arranque pendiente")
	}

	if response := doJSON(t, client, http.MethodPost, server.URL+"/api/setup/first-run/complete", ""); response.StatusCode != http.StatusOK {
		t.Fatalf("complete = %d, body = %s", response.StatusCode, response.Body)
	}
	if firstRunPending(t, server.URL, client) {
		t.Fatal("completar el primer arranque debe apagar la marca")
	}

	// Ya no se vuelve a marcar al cambiar las credenciales después.
	if response := doJSON(t, client, http.MethodPost, server.URL+"/api/setup", setup); response.StatusCode != http.StatusOK {
		t.Fatalf("tercer setup = %d, body = %s", response.StatusCode, response.Body)
	}
	if firstRunPending(t, server.URL, client) {
		t.Fatal("cambiar credenciales tras el primer arranque no debe volver a marcarlo")
	}
	// Completar de nuevo es inocuo.
	if response := doJSON(t, client, http.MethodPost, server.URL+"/api/setup/first-run/complete", ""); response.StatusCode != http.StatusOK {
		t.Fatalf("complete repetido = %d", response.StatusCode)
	}
}
