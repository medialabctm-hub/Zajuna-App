package backup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PendingResetFile is the marker that requests a clean local workspace on
// the next start. The local API writes it ("Restablecer datos") and the
// Windows installer writes it (content FullResetMarker) on every install,
// because the core is not running at install time.
const PendingResetFile = ".reset-pending"

// resetTargets are the user-data entries removed by a reset. backups/ is kept
// by the in-app reset (it is the only way back and the endpoint can create
// one first); a new-version install removes it too (see FullResetMarker).
var resetTargets = []string{
	"zajuna.db", "zajuna.db-wal", "zajuna.db-shm",
	"config.json", "evidences", "reports", "exports", "thumbnails",
	pendingRestoreDir, appliedRestoreFile,
	// Copies left by a restore that was applied but never committed.
	"zajuna.db.restore-old", "config.json.restore-old", "evidences.restore-old",
	"reports.restore-old", "exports.restore-old",
}

// StageReset records that the local data must be wiped on the next start.
func StageReset(dataDir string) error {
	if strings.TrimSpace(dataDir) == "" {
		return errors.New("la carpeta de datos es obligatoria")
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return fmt.Errorf("prepare data dir for reset: %w", err)
	}
	stamp := time.Now().UTC().Format(time.RFC3339) + "\n"
	if err := os.WriteFile(filepath.Join(dataDir, PendingResetFile), []byte(stamp), 0o600); err != nil {
		return fmt.Errorf("stage reset: %w", err)
	}
	return nil
}

// ResetPending reports whether a reset is staged.
func ResetPending(dataDir string) bool {
	_, err := os.Stat(filepath.Join(dataDir, PendingResetFile))
	return err == nil
}

// FullResetMarker is the marker content written by the Windows installer:
// a new version must start completely clean, backups included.
const FullResetMarker = "full"

// AppVersionFile records which app version created the local data.
const AppVersionFile = ".app-version"

// EnforceVersion makes every new version start from a clean workspace, as an
// installed desktop app should: when the data on disk was created by a
// different version (or by a release older than this marker, which never
// wrote it), everything is wiped, backups included, before SQLite opens.
// Development builds ("dev" or empty) never wipe.
func EnforceVersion(dataDir, version string) (bool, error) {
	version = strings.TrimSpace(version)
	if version == "" || version == "dev" {
		return false, nil
	}
	markerPath := filepath.Join(dataDir, AppVersionFile)
	previous, err := os.ReadFile(markerPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("read app version marker: %w", err)
	}
	stale := strings.TrimSpace(string(previous)) != version
	wiped := false
	if stale && (len(previous) > 0 || hasUserData(dataDir)) {
		var partial *partialWipeError
		if err := wipe(dataDir, true); err != nil && !errors.As(err, &partial) {
			return false, err
		}
		wiped = true
	}
	if stale {
		if err := os.WriteFile(markerPath, []byte(version+"\n"), 0o600); err != nil {
			return wiped, fmt.Errorf("write app version marker: %w", err)
		}
	}
	return wiped, nil
}

// hasUserData reports whether anything a reset would remove is on disk, so
// data from a release without the version marker is never left behind.
func hasUserData(dataDir string) bool {
	for _, name := range append(append([]string{}, resetTargets...), "backups") {
		if _, err := os.Stat(filepath.Join(dataDir, name)); err == nil {
			return true
		}
	}
	return false
}

// wipe removes the user data. The database is mandatory: if it cannot be
// removed the error is returned so the caller keeps the marker and retries.
func wipe(dataDir string, includeBackups bool) error {
	targets := append([]string{}, resetTargets...)
	if includeBackups {
		targets = append(targets, "backups")
	}
	var failures []string
	for _, name := range targets {
		if err := removeWithRetry(filepath.Join(dataDir, name)); err != nil {
			if strings.HasPrefix(name, "zajuna.db") {
				return fmt.Errorf("no se pudo borrar la base local para restablecer los datos: %w", err)
			}
			failures = append(failures, name)
		}
	}
	if len(failures) > 0 {
		// The database is gone, so the workspace is already clean for the UI;
		// orphaned files are collected by the evidence GC after migration.
		return &partialWipeError{names: failures}
	}
	return nil
}

type partialWipeError struct{ names []string }

func (e *partialWipeError) Error() string {
	return "datos restablecidos, pero quedaron archivos en uso: " + strings.Join(e.names, ", ")
}

// ApplyPendingReset wipes the staged user data before SQLite is opened. It
// returns false when no reset is pending. If the database cannot be removed
// (for example, a leftover process still holds it on Windows) the marker is
// kept so the next start retries, and the caller keeps using the old data.
func ApplyPendingReset(dataDir string) (bool, error) {
	if !ResetPending(dataDir) {
		return false, nil
	}
	contents, _ := os.ReadFile(filepath.Join(dataDir, PendingResetFile))
	includeBackups := strings.TrimSpace(string(contents)) == FullResetMarker
	wipeErr := wipe(dataDir, includeBackups)
	var partial *partialWipeError
	if wipeErr != nil && !errors.As(wipeErr, &partial) {
		return false, wipeErr
	}
	if err := os.Remove(filepath.Join(dataDir, PendingResetFile)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return true, fmt.Errorf("remove reset marker: %w", err)
	}
	return true, wipeErr
}

func removeWithRetry(target string) error {
	var err error
	for attempt := 0; attempt < 10; attempt++ {
		err = os.RemoveAll(target)
		if err == nil {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return err
}
