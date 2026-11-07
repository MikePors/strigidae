package fake

import (
	"context"
	"fmt"

	"github.com/MikePors/strigidae/internal/github"
)

type fileTextKey struct {
	repository             github.Repository
	commitObjectIdentifier string
	path                   string
}

// fileTextEntry is the text of a file, or the error reading it yields.
type fileTextEntry struct {
	text string
	err  error
}

// SetFileText sets the text FileText returns for the file at path in the
// commit. A file with none set answers ErrNotFound.
func (client *Client) SetFileText(repository github.Repository, commitObjectIdentifier, path, text string) {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	client.fileTexts[fileTextKey{repository, commitObjectIdentifier, path}] = fileTextEntry{text: text}
}

// SetFileTextUnavailable makes FileText answer github.ErrNoFileText for the
// file at path in the commit, as for a binary or oversized file.
func (client *Client) SetFileTextUnavailable(repository github.Repository, commitObjectIdentifier, path string) {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	client.fileTexts[fileTextKey{repository, commitObjectIdentifier, path}] = fileTextEntry{err: github.ErrNoFileText}
}

// FileText implements github.Client.
func (client *Client) FileText(ctx context.Context, repository github.Repository, commitObjectIdentifier, path string) (string, error) {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	call := Call{Method: MethodFileText, Repository: repository, CommitObjectIdentifier: commitObjectIdentifier, Path: path}
	if err := client.begin(ctx, call); err != nil {
		return "", err
	}
	entry, ok := client.fileTexts[fileTextKey{repository, commitObjectIdentifier, path}]
	if !ok {
		return "", fmt.Errorf("read %s at %s in %s: %w", path, commitObjectIdentifier, repository, github.ErrNotFound)
	}
	if entry.err != nil {
		return "", fmt.Errorf("read %s at %s in %s: %w", path, commitObjectIdentifier, repository, entry.err)
	}
	return entry.text, nil
}
