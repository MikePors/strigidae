// Package listcache stores disposable, account-scoped list snapshots.
package listcache

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/MikePors/strigidae/internal/github"
)

// Key identifies the effective list request within a host and account.
type Key struct {
	Kind       string
	Repository github.Repository
	Query      string
	Limit      int
}

type snapshot struct {
	Version   int
	FetchedAt time.Time
	Items     []github.PullRequest
}

// Store keeps snapshots for one host and account. A nil Store disables caching.
// Its methods are safe for concurrent commands.
type Store struct {
	directory string
	namespace string
	now       func() time.Time
	mutex     sync.Mutex
}

// New constructs a store without performing I/O. Unknown identities disable it.
func New(directory, host, account string, now func() time.Time) *Store {
	if directory == "" || host == "" || account == "" {
		return nil
	}
	if now == nil {
		now = time.Now
	}
	return &Store{directory: directory, namespace: github.NormalizeHost(host) + "\x00" + strings.ToLower(account), now: now}
}

// DefaultDirectory returns strig's directory in the platform's cache location.
func DefaultDirectory() string {
	directory, err := os.UserCacheDir()
	if err != nil || !filepath.IsAbs(directory) {
		return ""
	}
	return filepath.Join(directory, "strig")
}

func (store *Store) path(key Key) string {
	encoded, _ := json.Marshal(key) // Key contains only strings and integers.
	digest := sha256.Sum256(append([]byte(store.namespace+"\x00"), encoded...))
	return filepath.Join(store.directory, fmt.Sprintf("%x.json", digest))
}

// Load returns a snapshot only when it is valid and at most 24 hours old.
// Cache errors are misses; live loading must always remain available.
func (store *Store) Load(key Key) ([]github.PullRequest, bool) {
	if store == nil {
		return nil, false
	}
	store.mutex.Lock()
	defer store.mutex.Unlock()
	entry, ok := store.read(key)
	age := store.now().Sub(entry.FetchedAt)
	if !ok || age < 0 || age > 24*time.Hour {
		return nil, false
	}
	return entry.Items, true
}

func (store *Store) read(key Key) (snapshot, bool) {
	var entry snapshot
	data, err := os.ReadFile(store.path(key))
	if err != nil || json.Unmarshal(data, &entry) != nil || entry.Version != 1 || entry.FetchedAt.IsZero() {
		return entry, false
	}
	return entry, true
}

// Save replaces a snapshot after a successful request. Started is captured
// before the request so an older in-flight load cannot overwrite a newer one.
// Failures leave the live list usable and, where possible, the old file intact.
func (store *Store) Save(key Key, items []github.PullRequest, started time.Time) error {
	if store == nil {
		return nil
	}
	store.mutex.Lock()
	defer store.mutex.Unlock()
	if previous, ok := store.read(key); ok && !previous.FetchedAt.After(store.now()) && previous.FetchedAt.After(started) {
		return nil
	}
	data, err := json.Marshal(snapshot{Version: 1, FetchedAt: started, Items: items})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(store.directory, 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(store.directory, ".list-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(file.Name()) }()
	_, writeError := file.Write(data)
	if err := errors.Join(writeError, file.Close()); err != nil {
		return err
	}
	return os.Rename(file.Name(), store.path(key))
}

// Denied identifies failures for which cached rows must no longer be shown.
func Denied(err error) bool {
	return errors.Is(err, github.ErrNotAuthenticated) || errors.Is(err, github.ErrForbidden) || errors.Is(err, github.ErrNotFound)
}

// Remove discards a snapshot after access is refused.
func (store *Store) Remove(key Key) error {
	if store == nil {
		return nil
	}
	store.mutex.Lock()
	defer store.mutex.Unlock()
	err := os.Remove(store.path(key))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
