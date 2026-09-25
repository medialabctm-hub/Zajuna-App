// Package datalock keeps a single core process per data directory. Two cores
// on the same folder would run two job runtimes and schedulers against one
// SQLite database and could apply a pending reset under the other's feet.
package datalock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// FileName is the lock file inside the data directory. It is not a reset
// target: resets must run while the lock is held.
const FileName = ".core.lock"

// ErrLocked means another core holds the data directory.
var ErrLocked = errors.New("otro proceso de Zajuna App ya usa esta carpeta de datos")

// Lock is an exclusive OS lock on the data directory, released by Release or
// automatically when the process exits.
type Lock struct{ file *os.File }

// Acquire takes the lock, retrying for up to wait (a supervisor may restart
// the core before the previous process has fully exited).
func Acquire(dataDir string, wait time.Duration) (*Lock, error) {
	path := filepath.Join(dataDir, FileName)
	deadline := time.Now().Add(wait)
	for {
		file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
		if err != nil {
			return nil, fmt.Errorf("abrir el bloqueo de la carpeta de datos: %w", err)
		}
		if err := lockFile(file); err == nil {
			_ = file.Truncate(0)
			_, _ = file.WriteAt([]byte(fmt.Sprintf("%d\n", os.Getpid())), 0)
			return &Lock{file: file}, nil
		}
		_ = file.Close()
		if time.Now().After(deadline) {
			return nil, ErrLocked
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// Release frees the lock. It is safe to call on a nil lock.
func (l *Lock) Release() {
	if l == nil || l.file == nil {
		return
	}
	_ = unlockFile(l.file)
	_ = l.file.Close()
	l.file = nil
}
