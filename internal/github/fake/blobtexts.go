package fake

import (
	"context"
	"fmt"
	"slices"

	"github.com/MikePors/strigidae/internal/github"
)

type blobTextKey struct {
	repository       github.Repository
	objectIdentifier string
}

// SetBlobText sets the text BlobTexts returns for the blob with the object
// identifier, spelled exactly as it will be asked for. A blob with none set
// answers ErrNotFound.
func (client *Client) SetBlobText(repository github.Repository, objectIdentifier, text string) {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	client.blobTexts[blobTextKey{repository, objectIdentifier}] = fileTextEntry{text: text}
}

// SetBlobTextUnavailable makes BlobTexts answer github.ErrNoFileText for the
// blob, as for a binary or oversized file.
func (client *Client) SetBlobTextUnavailable(repository github.Repository, objectIdentifier string) {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	client.blobTexts[blobTextKey{repository, objectIdentifier}] = fileTextEntry{err: github.ErrNoFileText}
}

// BlobTexts implements github.Client. The call is recorded with the
// identifiers, so a test can assert one call asked for several.
func (client *Client) BlobTexts(ctx context.Context, repository github.Repository, objectIdentifiers []string) ([]github.BlobText, error) {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	call := Call{Method: MethodBlobTexts, Repository: repository, ObjectIdentifiers: slices.Clone(objectIdentifiers)}
	if err := client.begin(ctx, call); err != nil {
		return nil, err
	}
	texts := make([]github.BlobText, len(objectIdentifiers))
	for i, identifier := range objectIdentifiers {
		entry, ok := client.blobTexts[blobTextKey{repository, identifier}]
		switch {
		case !ok:
			texts[i].Err = fmt.Errorf("read blob %s in %s: %w", identifier, repository, github.ErrNotFound)
		case entry.err != nil:
			texts[i].Err = fmt.Errorf("read blob %s in %s: %w", identifier, repository, entry.err)
		default:
			texts[i].Text = entry.text
		}
	}
	return texts, nil
}
