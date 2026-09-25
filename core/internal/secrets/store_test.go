package secrets

import (
	"errors"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestSystemStoreRoundTripAndIdempotentDelete(t *testing.T) {
	keyring.MockInit()
	var store Store = SystemStore{}

	if err := store.Set("123456789", "secreto"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := store.Get("123456789")
	if err != nil || got != "secreto" {
		t.Fatalf("Get = %q, %v; want secreto", got, err)
	}
	if _, err := keyring.Get(ServiceName, "123456789"); err != nil {
		t.Fatalf("password must be stored under service %q: %v", ServiceName, err)
	}

	deleter := SystemStore{}
	if err := deleter.Delete("123456789"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Get("123456789"); !errors.Is(err, keyring.ErrNotFound) {
		t.Fatalf("Get after Delete = %v; want keyring.ErrNotFound", err)
	}
	if err := deleter.Delete("123456789"); err != nil {
		t.Fatalf("Delete of a missing entry must succeed so a data reset can call it unconditionally: %v", err)
	}
}

func TestSystemStoreDeletePropagatesBackendErrors(t *testing.T) {
	backendErr := errors.New("keyring locked")
	keyring.MockInitWithError(backendErr)
	t.Cleanup(keyring.MockInit)

	if err := (SystemStore{}).Delete("123456789"); !errors.Is(err, backendErr) {
		t.Fatalf("Delete = %v; want backend error", err)
	}
}
