// Package pullrequestmarks keeps one private, free-form mark per pull request
// ("waiting on author", "left for Sam") and saves them between runs of strig.
// Nothing is sent to GitHub.
//
// It knows nothing of the user interface or Bubble Tea. The root model owns a
// Store, changes it from Update, and runs Save in a command.
package pullrequestmarks

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/MikePors/strigidae/internal/github"
)

const (
	// Version is the file format this package reads and writes.
	Version = 1
	// MaxLength is the longest mark, counted in characters.
	MaxLength = 40
	// MaxFileSize is the largest file that is read. A bigger one is treated
	// as corrupt.
	MaxFileSize = 4 << 20

	directoryMode = 0o700
	fileMode      = 0o600
)

// Errors that say why a mark was refused or a file could not be used. None of
// them carries any of the mark text or the file's content, so they are safe to log.
var (
	ErrEmpty          = errors.New("mark is empty")
	ErrTooLong        = errors.New("mark is too long")
	ErrControl        = errors.New("mark contains control or bidirectional characters")
	ErrInvalidKey     = errors.New("pull request key needs a host, an owner, a repository name and a positive number")
	ErrCorrupt        = errors.New("marks file can't be decoded")
	ErrUnknownVersion = errors.New("marks file has an unknown version")
	ErrTooLarge       = errors.New("marks file is too large")
)

// Key identifies a pull request: the host, the repository and the number.
// Owner and repository name are compared without regard to case, as GitHub does.
type Key struct {
	Host       string
	Repository github.Repository
	Number     int
}

func (key Key) normalised() Key {
	key.Host = strings.ToLower(key.Host)
	key.Repository.Owner = strings.ToLower(key.Repository.Owner)
	key.Repository.Name = strings.ToLower(key.Repository.Name)
	return key
}

func (key Key) valid() bool {
	return key.Host != "" && key.Repository.Owner != "" && key.Repository.Name != "" && key.Number > 0
}

// Store holds the marks and the file they are kept in.
//
// It is safe for concurrent use, because the interface changes it from Update
// while a command saves it. Two mutexes keep that cheap for Update: the state
// mutex guards the marks and is held only for in-memory work, never across
// disk access; the save mutex serialises writes and is held through the disk
// write. Save takes its snapshot after it holds the save mutex, so a copy made
// earlier can never be written after a newer one, and a stuck disk can hold
// Save but never Set, Clear, Mark or Names.
type Store struct {
	path string

	saveMutex sync.Mutex

	stateMutex sync.Mutex
	marks      map[Key]string
	loadErr    error // why the file could not be used; Save refuses to overwrite it
}

// Open returns a store holding the marks saved under stateDirectory. An empty
// directory keeps the marks for this run only. A missing file means no marks.
// When the file can't be used, Open returns the error together with a store
// that works in memory; its Save refuses to write, so the file is never
// overwritten silently.
func Open(stateDirectory string) (*Store, error) {
	store := &Store{marks: map[Key]string{}}
	if stateDirectory == "" {
		return store, nil
	}
	store.path = filepath.Join(stateDirectory, "strig", "pull-request-marks.json")
	content, err := read(store.path)
	if err != nil {
		store.loadErr = err
		return store, err
	}
	for _, saved := range content.Marks {
		key := Key{Host: saved.Host, Repository: github.Repository{Owner: saved.Owner, Name: saved.Repository}, Number: saved.Number}
		if !key.valid() || validate(saved.Text) != nil {
			store.loadErr = ErrCorrupt
			return store, ErrCorrupt
		}
		store.marks[key.normalised()] = saved.Text
	}
	return store, nil
}

// Mark returns the mark of a pull request, or false when it has none.
func (store *Store) Mark(key Key) (string, bool) {
	store.stateMutex.Lock()
	defer store.stateMutex.Unlock()
	text, ok := store.marks[key.normalised()]
	return text, ok
}

// Set gives a pull request a mark, replacing any it had. The text is trimmed
// of surrounding white space. It is refused when empty, too long, or holding
// control or bidirectional characters.
func (store *Store) Set(key Key, text string) error {
	if !key.valid() {
		return ErrInvalidKey
	}
	text = strings.TrimSpace(text)
	if err := validate(text); err != nil {
		return err
	}
	store.stateMutex.Lock()
	defer store.stateMutex.Unlock()
	store.marks[key.normalised()] = text
	return nil
}

// Clear removes a pull request's mark. A pull request without one is left alone.
func (store *Store) Clear(key Key) {
	store.stateMutex.Lock()
	defer store.stateMutex.Unlock()
	delete(store.marks, key.normalised())
}

// Names returns the distinct marks in use, sorted, for completion.
func (store *Store) Names() []string {
	store.stateMutex.Lock()
	defer store.stateMutex.Unlock()
	names := make([]string, 0, len(store.marks))
	for _, text := range store.marks {
		names = append(names, text)
	}
	slices.Sort(names)
	return slices.Compact(names)
}

func validate(text string) error {
	if strings.TrimSpace(text) == "" {
		return ErrEmpty
	}
	for _, character := range text {
		if unicode.IsControl(character) || unicode.Is(unicode.Bidi_Control, character) {
			return ErrControl
		}
	}
	if utf8.RuneCountInString(text) > MaxLength {
		return ErrTooLong
	}
	return nil
}

// entry is one mark as it is kept in the file.
type entry struct {
	Host       string `json:"host"`
	Owner      string `json:"owner"`
	Repository string `json:"repository"`
	Number     int    `json:"number"`
	Text       string `json:"text"`
}

type file struct {
	Version int     `json:"version"`
	Marks   []entry `json:"marks"`
}

// Save writes the marks to the file, atomically. It does nothing for a store
// without a state directory, and fails without writing when Open could not use
// the file.
func (store *Store) Save() error {
	if store.path == "" {
		return nil
	}
	store.saveMutex.Lock()
	defer store.saveMutex.Unlock()
	data, err := store.encode()
	if err != nil {
		return err
	}
	return write(store.path, data)
}

func (store *Store) encode() ([]byte, error) {
	store.stateMutex.Lock()
	defer store.stateMutex.Unlock()
	if store.loadErr != nil {
		return nil, fmt.Errorf("marks not saved, the file was not used: %w", store.loadErr)
	}
	content := file{Version: Version, Marks: make([]entry, 0, len(store.marks))}
	for key, text := range store.marks {
		content.Marks = append(content.Marks, entry{Host: key.Host, Owner: key.Repository.Owner, Repository: key.Repository.Name, Number: key.Number, Text: text})
	}
	// A stable order keeps the file the same when the marks are.
	slices.SortFunc(content.Marks, func(first, second entry) int {
		return cmp.Or(
			cmp.Compare(first.Host, second.Host),
			cmp.Compare(first.Owner, second.Owner),
			cmp.Compare(first.Repository, second.Repository),
			cmp.Compare(first.Number, second.Number),
		)
	})
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(content); err != nil {
		return nil, fmt.Errorf("encode pull request marks: %w", err)
	}
	return buffer.Bytes(), nil
}

// read loads the file at path. A file that doesn't exist holds no marks.
func read(path string) (file, error) {
	handle, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return file{Version: Version}, nil
	}
	if err != nil {
		return file{}, fmt.Errorf("open pull request marks: %w", err)
	}
	defer func() { _ = handle.Close() }() // read only
	// The limit applies to the reader, in case the file grows meanwhile.
	data, err := io.ReadAll(io.LimitReader(handle, MaxFileSize+1))
	if err != nil {
		return file{}, fmt.Errorf("read pull request marks: %w", err)
	}
	if len(data) > MaxFileSize {
		return file{}, ErrTooLarge
	}
	var header struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return file{}, ErrCorrupt
	}
	if header.Version != Version {
		return file{}, ErrUnknownVersion
	}
	var content file
	if err := json.Unmarshal(data, &content); err != nil {
		return file{}, ErrCorrupt
	}
	return content, nil
}

// write replaces the file at path with data: it writes a temporary file in the
// same directory and renames it over path, so a reader sees the old file or the
// new one and never half of one.
func write(path string, data []byte) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, directoryMode); err != nil {
		return fmt.Errorf("create pull request marks directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".write-*")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	name := temporary.Name()
	_, err = temporary.Write(data)
	if err == nil {
		err = temporary.Chmod(fileMode)
	}
	if err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(name, path)
	}
	if err != nil {
		_ = os.Remove(name) // best effort: the write already failed
		return fmt.Errorf("write pull request marks: %w", err)
	}
	return nil
}
