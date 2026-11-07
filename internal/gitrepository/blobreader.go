package gitrepository

import (
	"context"
	"errors"
	"sync"
)

// BlobReader reads the blobs of one repository's pull requests from the
// checkout that has the repository as its remote. A blob the checkout lacks is
// read again after fetching the pull request's head, which is tried at most
// once per pull request for the life of the value. It is safe to use from
// several goroutines.
type BlobReader struct {
	directory  string
	remoteName string
	// readBlob and fetchHead are ReadBlob and FetchPullRequestHead; tests
	// replace them so no git runs.
	readBlob  func(ctx context.Context, directory, blobObjectIdentifier string) ([]byte, error)
	fetchHead func(ctx context.Context, directory, remoteName string, number int) error

	mutex   sync.Mutex
	fetched map[int]bool // pull requests whose head was fetched, or tried
}

// NewBlobReader reads blobs in the checkout containing directory, fetching
// from the git remote remoteName.
func NewBlobReader(directory, remoteName string) *BlobReader {
	return &BlobReader{
		directory: directory, remoteName: remoteName,
		readBlob: ReadBlob, fetchHead: FetchPullRequestHead,
		fetched: map[int]bool{},
	}
}

// ReadBlob returns the contents of blob blobObjectIdentifier of pull request
// number. When the checkout lacks it and the pull request's head has not been
// fetched yet, it fetches the head and reads again. It fails with
// ErrBlobNotFound when the blob is still missing; a failed fetch is returned
// as it is, and is not tried again.
func (reader *BlobReader) ReadBlob(ctx context.Context, number int, blobObjectIdentifier string) ([]byte, error) {
	contents, err := reader.readBlob(ctx, reader.directory, blobObjectIdentifier)
	if !errors.Is(err, ErrBlobNotFound) {
		return contents, err
	}
	// The lock is held through the fetch, so a second read waiting for the
	// same pull request finds the objects instead of fetching again.
	reader.mutex.Lock()
	defer reader.mutex.Unlock()
	if !reader.fetched[number] {
		reader.fetched[number] = true
		if err := reader.fetchHead(ctx, reader.directory, reader.remoteName, number); err != nil {
			return nil, err
		}
	}
	return reader.readBlob(ctx, reader.directory, blobObjectIdentifier)
}
