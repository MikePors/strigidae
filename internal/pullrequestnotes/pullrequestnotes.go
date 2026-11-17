// Package pullrequestnotes keeps one private, free-form, multi-line note per
// pull request and saves them between runs of strig. Nothing is sent to GitHub.
//
// It knows nothing of the user interface or Bubble Tea. The notes are a plain
// value, [Notes], that the root model holds and changes only in Update. To save
// them it passes the bytes from [Encode] to a command that calls [Write]; [Read]
// loads them before the program starts. Nothing here is shared between Update and
// a command, so there are no locks, and putting saves in order is the caller's job.
package pullrequestnotes

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
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
	// MaxNoteSize is the largest note, in bytes. It is far above what anyone
	// writes by hand; it exists so that one note can't make the whole file too
	// large to read back.
	MaxNoteSize = 1 << 20
	// MaxFileSize is the largest file that is read. A bigger one is treated
	// as corrupt.
	MaxFileSize = 64 << 20

	directoryMode = 0o700
	fileMode      = 0o600
)

// Errors that say why a note was refused or a file could not be used. None of
// them carries any of the note text or the file's content, so they are safe to log.
var (
	ErrEmpty          = errors.New("note is empty")
	ErrTooLarge       = errors.New("note is too large")
	ErrControl        = errors.New("note contains control or bidirectional characters")
	ErrInvalidUTF8    = errors.New("note is not valid UTF-8")
	ErrInvalidKey     = errors.New("pull request key needs a host, an owner, a repository name and a positive number")
	ErrCorrupt        = errors.New("notes file can't be decoded")
	ErrUnknownVersion = errors.New("notes file has an unknown version")
	ErrFileTooLarge   = errors.New("notes file is too large")
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

// Notes is the set of notes, at most one per pull request. It is a value: Set
// and Clear return a changed copy and never change the receiver, so a copy
// handed to a command can't be affected by later changes in Update. The zero
// value holds no notes and is ready to use.
type Notes struct {
	notes map[Key]string // keyed by the normalised key; never changed once shared
}

// Note returns the note of a pull request, or false when it has none.
func (notes Notes) Note(key Key) (string, bool) {
	text, ok := notes.notes[key.normalised()]
	return text, ok
}

// Set returns the notes with a pull request given a note, replacing any it had.
// Line endings become LF and the text is trimmed of surrounding white space. It
// is refused, and the notes returned unchanged, when the key is incomplete, or
// the text is empty, too large, not valid UTF-8, or holds control characters
// other than newlines and tabs, or bidirectional ones.
func (notes Notes) Set(key Key, text string) (Notes, error) {
	if !key.valid() {
		return notes, ErrInvalidKey
	}
	text = Clean(text)
	if err := validate(text); err != nil {
		return notes, err
	}
	changed := notes.copy(1)
	changed.notes[key.normalised()] = text
	return changed, nil
}

// Clear returns the notes without the pull request's note. A pull request
// without one leaves the notes as they were.
func (notes Notes) Clear(key Key) Notes {
	if _, ok := notes.notes[key.normalised()]; !ok {
		return notes
	}
	changed := notes.copy(0)
	delete(changed.notes, key.normalised())
	return changed
}

// Empty reports whether there are no notes.
func (notes Notes) Empty() bool { return len(notes.notes) == 0 }

// Clean is the text as Set stores it: LF line endings (a CRLF pair or a lone
// CR each become one LF), no surrounding white space. The editor's text is
// compared with the stored note through it.
func Clean(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.TrimSpace(text)
}

// Validate says whether Set would keep the text, so a caller can refuse it
// while the user still has it. It returns the same errors as Set, except for
// the key's.
func Validate(text string) error { return validate(Clean(text)) }

// copy returns notes with its own map, with room for extra more entries.
func (notes Notes) copy(extra int) Notes {
	copied := make(map[Key]string, len(notes.notes)+extra)
	maps.Copy(copied, notes.notes)
	return Notes{notes: copied}
}

func validate(text string) error {
	if text == "" {
		return ErrEmpty
	}
	if len(text) > MaxNoteSize {
		return ErrTooLarge
	}
	// Invalid bytes would be written as U+FFFD, so the note would change on
	// the next load.
	if !utf8.ValidString(text) {
		return ErrInvalidUTF8
	}
	for _, character := range text {
		if character == '\n' || character == '\t' {
			continue
		}
		if unicode.IsControl(character) || unicode.Is(unicode.Bidi_Control, character) {
			return ErrControl
		}
	}
	return nil
}

// entry is one note as it is kept in the file.
type entry struct {
	Host       string `json:"host"`
	Owner      string `json:"owner"`
	Repository string `json:"repository"`
	Number     int    `json:"number"`
	Text       string `json:"text"`
}

type file struct {
	Version int     `json:"version"`
	Notes   []entry `json:"notes"`
}

// Encode returns the file content for the notes. The entries are in a stable
// order, so equal notes give equal bytes. It can fail only if the entry types
// stop being encodable; a caller treats an error like a failed save.
func Encode(notes Notes) ([]byte, error) {
	content := file{Version: Version, Notes: make([]entry, 0, len(notes.notes))}
	for key, text := range notes.notes {
		content.Notes = append(content.Notes, entry{Host: key.Host, Owner: key.Repository.Owner, Repository: key.Repository.Name, Number: key.Number, Text: text})
	}
	slices.SortFunc(content.Notes, func(first, second entry) int {
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
		return nil, fmt.Errorf("encode pull request notes: %w", err)
	}
	// Never write a file that Read would refuse: that would lose every note.
	if buffer.Len() > MaxFileSize {
		return nil, fmt.Errorf("encode pull request notes: %w", ErrFileTooLarge)
	}
	return buffer.Bytes(), nil
}

// Decode reads the notes from file content. Any problem returns empty notes
// together with the error, so a half-read file is never used.
func Decode(data []byte) (Notes, error) {
	var header struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return Notes{}, ErrCorrupt
	}
	if header.Version != Version {
		return Notes{}, ErrUnknownVersion
	}
	var content file
	if err := json.Unmarshal(data, &content); err != nil {
		return Notes{}, ErrCorrupt
	}
	decoded := make(map[Key]string, len(content.Notes))
	for _, saved := range content.Notes {
		key := Key{Host: saved.Host, Repository: github.Repository{Owner: saved.Owner, Name: saved.Repository}, Number: saved.Number}
		// Refuse what Set would never have written: unclean text, and two
		// entries for one pull request (spellings that differ only in case).
		_, duplicate := decoded[key.normalised()]
		if !key.valid() || validate(saved.Text) != nil || saved.Text != Clean(saved.Text) || duplicate {
			return Notes{}, ErrCorrupt
		}
		decoded[key.normalised()] = saved.Text
	}
	return Notes{notes: decoded}, nil
}

// Path returns where the notes file lives under the state directory.
func Path(stateDirectory string) string {
	return filepath.Join(stateDirectory, "strig", "pull-request-notes.json")
}

// Read loads the notes from the file at path. A file that doesn't exist holds
// no notes. Any other problem returns empty notes together with the error.
func Read(path string) (Notes, error) {
	handle, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Notes{}, nil
	}
	if err != nil {
		return Notes{}, fmt.Errorf("open pull request notes: %w", err)
	}
	defer func() { _ = handle.Close() }() // read only
	// The limit applies to the reader, in case the file grows meanwhile.
	data, err := io.ReadAll(io.LimitReader(handle, MaxFileSize+1))
	if err != nil {
		return Notes{}, fmt.Errorf("read pull request notes: %w", err)
	}
	if len(data) > MaxFileSize {
		return Notes{}, fmt.Errorf("read pull request notes %s: %w", path, ErrFileTooLarge)
	}
	notes, err := Decode(data)
	if err != nil {
		// The path tells the user which file to fix or remove.
		return Notes{}, fmt.Errorf("read pull request notes %s: %w", path, err)
	}
	return notes, nil
}

// Write replaces the file at path with data, which should come from Encode: it
// writes a temporary file in the same directory and renames it over path, so a
// reader sees the old file or the new one and never half of one.
func Write(path string, data []byte) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, directoryMode); err != nil {
		return fmt.Errorf("create pull request notes directory: %w", err)
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
		return fmt.Errorf("write pull request notes: %w", err)
	}
	syncDirectory(directory)
	return nil
}

// Saver writes the notes file for the root and for the commands it starts. It
// is the one place where saves meet, so it orders them: each save carries the
// generation of the notes it holds, and one older than a save that has already
// succeeded is skipped, so an old copy can never land after a newer one, even
// when a save made at quit overtakes one still running.
type Saver struct {
	path    string
	mutex   sync.Mutex
	written int // the newest generation written so far
}

// NewSaver returns a saver for the file at path.
func NewSaver(path string) *Saver { return &Saver{path: path} }

// Save writes data, which holds the notes of the given generation (a number
// that grows with every change), unless a newer generation was written already.
func (saver *Saver) Save(generation int, data []byte) error {
	saver.mutex.Lock()
	defer saver.mutex.Unlock()
	if generation <= saver.written {
		return nil
	}
	if err := Write(saver.path, data); err != nil {
		return err
	}
	saver.written = generation
	return nil
}

// syncDirectory makes the rename durable, as far as the file system allows. It
// is best effort: the new file is already in place.
func syncDirectory(directory string) {
	handle, err := os.Open(directory)
	if err != nil {
		return
	}
	_ = handle.Sync()
	_ = handle.Close()
}
