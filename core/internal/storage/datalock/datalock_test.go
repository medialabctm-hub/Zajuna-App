package datalock

import (
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"
)

// The lock is per process (flock/LockFileEx): the second holder is a child
// process of this test binary.
func TestSecondProcessCannotTakeTheDataDirectory(t *testing.T) {
	if dir := os.Getenv("DATALOCK_CHILD_DIR"); dir != "" {
		if _, err := Acquire(dir, 0); errors.Is(err, ErrLocked) {
			os.Exit(3)
		} else if err != nil {
			os.Exit(2)
		}
		os.Exit(0)
	}
	dir := t.TempDir()
	lock, err := Acquire(dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	run := func() int {
		cmd := exec.Command(os.Args[0], "-test.run", "^TestSecondProcessCannotTakeTheDataDirectory$")
		cmd.Env = append(os.Environ(), "DATALOCK_CHILD_DIR="+dir)
		err := cmd.Run()
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return exit.ExitCode()
		}
		if err != nil {
			t.Fatal(err)
		}
		return 0
	}
	if code := run(); code != 3 {
		t.Fatalf("a second process must be refused while the lock is held, exit %d", code)
	}
	lock.Release()
	if code := run(); code != 0 {
		t.Fatalf("the lock must be free after Release, exit %d", code)
	}
}

func TestAcquireWaitsForTheHolderToLeave(t *testing.T) {
	dir := t.TempDir()
	lock, err := Acquire(dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	lock.Release()
	start := time.Now()
	again, err := Acquire(dir, time.Second)
	if err != nil || time.Since(start) > time.Second {
		t.Fatalf("a free lock must be taken at once: %v", err)
	}
	again.Release()
}
