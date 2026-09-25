package workers

import (
	"context"
	"sort"
	"strconv"
	"sync"

	"github.com/zajuna-app/core/internal/checklist"
)

// keyedLocks serializes work per key (one checklist evidence slot) across
// every job running in this process. Entries are dropped once no holder or
// waiter remains, so the map does not grow with old slots.
type keyedLocks struct {
	mu    sync.Mutex
	slots map[string]*keyedLock
}

type keyedLock struct {
	token chan struct{}
	refs  int
}

var checklistSlotLocks = &keyedLocks{slots: make(map[string]*keyedLock)}

// checklistSlotLockKeys lists every evidence slot a target writes: its own
// file and the evidence rows of each covered item share the same slot.
func checklistSlotLockKeys(fichaID string, target checklist.CaptureTarget) []string {
	keys := make([]string, 0, len(target.CoveredItemCodes)+1)
	for _, itemCode := range append([]string{target.ItemCode}, target.CoveredItemCodes...) {
		keys = append(keys, fichaID+"\x1f"+itemCode+"\x1f"+strconv.Itoa(normalizedSlot(target.SlotNumber)))
	}
	return keys
}

// lockAll acquires every key in a fixed order (so two holders of
// overlapping sets cannot deadlock) or none of them when ctx ends first.
// The returned function releases them and is safe to call more than once.
func (l *keyedLocks) lockAll(ctx context.Context, keys []string) (func(), error) {
	unique := make([]string, 0, len(keys))
	seen := make(map[string]bool, len(keys))
	for _, key := range keys {
		if !seen[key] {
			seen[key] = true
			unique = append(unique, key)
		}
	}
	sort.Strings(unique)
	releases := make([]func(), 0, len(unique))
	releaseAll := func() {
		for index := len(releases) - 1; index >= 0; index-- {
			releases[index]()
		}
	}
	for _, key := range unique {
		release, err := l.lock(ctx, key)
		if err != nil {
			releaseAll()
			return nil, err
		}
		releases = append(releases, release)
	}
	var once sync.Once
	return func() { once.Do(releaseAll) }, nil
}

func (l *keyedLocks) lock(ctx context.Context, key string) (func(), error) {
	l.mu.Lock()
	entry, ok := l.slots[key]
	if !ok {
		entry = &keyedLock{token: make(chan struct{}, 1)}
		l.slots[key] = entry
	}
	entry.refs++
	l.mu.Unlock()

	select {
	case entry.token <- struct{}{}:
	case <-ctx.Done():
		l.forget(key, entry)
		return nil, ctx.Err()
	}
	return func() {
		<-entry.token
		l.forget(key, entry)
	}, nil
}

func (l *keyedLocks) forget(key string, entry *keyedLock) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry.refs--
	if entry.refs == 0 && l.slots[key] == entry {
		delete(l.slots, key)
	}
}
