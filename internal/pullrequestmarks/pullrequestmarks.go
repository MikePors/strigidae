// Package pullrequestmarks keeps one private, free-form mark per pull request
// ("waiting on author", "left for Sam") and saves them between runs of strig.
// Nothing is sent to GitHub.
//
// It knows nothing of the user interface or Bubble Tea. The marks are a plain
// value, [Marks], that the root model holds and changes only in Update. To save
// them it passes the bytes from [Encode] to a command that calls [Write]; [Read]
// loads them before the program starts. Nothing here is shared between Update and
// a command, so there are no locks, and putting saves in order is the caller's job.
package pullrequestmarks

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
	ErrInvalidUTF8    = errors.New("mark is not valid UTF-8")
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

// Marks is the set of marks, at most one per pull request. It is a value: Set
// and Clear return a changed copy and never change the receiver, so a copy
// handed to a command can't be affected by later changes in Update. Each change
// copies the whole map, which stays cheap because it holds one short mark per
// pull request the user marked. The zero value holds no marks and is ready to use.
type Marks struct {
	marks map[Key]string // keyed by the normalised key; never changed once shared
}

// Mark returns the mark of a pull request, or false when it has none.
func (marks Marks) Mark(key Key) (string, bool) {
	text, ok := marks.marks[key.normalised()]
	return text, ok
}

// Set returns the marks with a pull request given a mark, replacing any it had.
// The text is trimmed of surrounding white space. It is refused, and the marks
// returned unchanged, when the key is incomplete, or the text is empty, too long,
// not valid UTF-8, or holds control or bidirectional characters.
func (marks Marks) Set(key Key, text string) (Marks, error) {
	if !key.valid() {
		return marks, ErrInvalidKey
	}
	text = strings.TrimSpace(text)
	if err := validate(text); err != nil {
		return marks, err
	}
	changed := marks.copy(1)
	changed.marks[key.normalised()] = text
	return changed, nil
}

// Clear returns the marks without the pull request's mark. A pull request
// without one leaves the marks as they were.
func (marks Marks) Clear(key Key) Marks {
	if _, ok := marks.marks[key.normalised()]; !ok {
		return marks
	}
	changed := marks.copy(0)
	delete(changed.marks, key.normalised())
	return changed
}

// Names returns the distinct marks in use, sorted, for completion.
func (marks Marks) Names() []string {
	names := make([]string, 0, len(marks.marks))
	for _, text := range marks.marks {
		names = append(names, text)
	}
	slices.Sort(names)
	return slices.Compact(names)
}

// copy returns marks with its own map, with room for extra more entries.
func (marks Marks) copy(extra int) Marks {
	copied := make(map[Key]string, len(marks.marks)+extra)
	maps.Copy(copied, marks.marks)
	return Marks{marks: copied}
}

func validate(text string) error {
	if strings.TrimSpace(text) == "" {
		return ErrEmpty
	}
	// Invalid bytes would be written as U+FFFD, so the mark would change on
	// the next load.
	if !utf8.ValidString(text) {
		return ErrInvalidUTF8
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

// Encode returns the file content for the marks. The entries are in a stable
// order, so equal marks give equal bytes. It can fail only if the entry types
// stop being encodable; a caller treats an error like a failed save.
func Encode(marks Marks) ([]byte, error) {
	content := file{Version: Version, Marks: make([]entry, 0, len(marks.marks))}
	for key, text := range marks.marks {
		content.Marks = append(content.Marks, entry{Host: key.Host, Owner: key.Repository.Owner, Repository: key.Repository.Name, Number: key.Number, Text: text})
	}
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

// Decode reads the marks from file content. Any problem returns empty marks
// together with the error, so a half-read file is never used.
func Decode(data []byte) (Marks, error) {
	var header struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return Marks{}, ErrCorrupt
	}
	if header.Version != Version {
		return Marks{}, ErrUnknownVersion
	}
	var content file
	if err := json.Unmarshal(data, &content); err != nil {
		return Marks{}, ErrCorrupt
	}
	decoded := make(map[Key]string, len(content.Marks))
	for _, saved := range content.Marks {
		key := Key{Host: saved.Host, Repository: github.Repository{Owner: saved.Owner, Name: saved.Repository}, Number: saved.Number}
		// Refuse what Set would never have written: untrimmed text, and two
		// entries for one pull request (spellings that differ only in case).
		_, duplicate := decoded[key.normalised()]
		if !key.valid() || validate(saved.Text) != nil || saved.Text != strings.TrimSpace(saved.Text) || duplicate {
			return Marks{}, ErrCorrupt
		}
		decoded[key.normalised()] = saved.Text
	}
	return Marks{marks: decoded}, nil
}

// Path returns where the marks file lives under the state directory.
func Path(stateDirectory string) string {
	return filepath.Join(stateDirectory, "strig", "pull-request-marks.json")
}

// Read loads the marks from the file at path. A file that doesn't exist holds
// no marks. Any other problem returns empty marks together with the error.
func Read(path string) (Marks, error) {
	handle, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Marks{}, nil
	}
	if err != nil {
		return Marks{}, fmt.Errorf("open pull request marks: %w", err)
	}
	defer func() { _ = handle.Close() }() // read only
	// The limit applies to the reader, in case the file grows meanwhile.
	data, err := io.ReadAll(io.LimitReader(handle, MaxFileSize+1))
	if err != nil {
		return Marks{}, fmt.Errorf("read pull request marks: %w", err)
	}
	if len(data) > MaxFileSize {
		return Marks{}, fmt.Errorf("read pull request marks %s: %w", path, ErrTooLarge)
	}
	marks, err := Decode(data)
	if err != nil {
		// The path tells the user which file to fix or remove.
		return Marks{}, fmt.Errorf("read pull request marks %s: %w", path, err)
	}
	return marks, nil
}

// Write replaces the file at path with data, which should come from Encode: it
// writes a temporary file in the same directory and renames it over path, so a
// reader sees the old file or the new one and never half of one.
func Write(path string, data []byte) error {
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
	syncDirectory(directory)
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
