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

	mutex   sync.Mutex          // guards locks and fetched
	locks   map[int]*sync.Mutex // one per pull request, held through its fetch
	fetched map[int]bool        // pull requests whose head fetch ran
}

// NewBlobReader reads blobs in the checkout containing directory, fetching
// from the git remote remoteName.
func NewBlobReader(directory, remoteName string) *BlobReader {
	return &BlobReader{
		directory: directory, remoteName: remoteName,
		readBlob: ReadBlob, fetchHead: FetchPullRequestHead,
		locks: map[int]*sync.Mutex{}, fetched: map[int]bool{},
	}
}

// ReadBlob returns the contents of blob blobObjectIdentifier of pull request
// number. When the checkout lacks it and the pull request's head has not been
// fetched yet, it fetches the head and reads again. It fails with
// ErrBlobNotFound when the blob is still missing; a failed fetch is returned
// as it is, and is not tried again. A read whose ctx ends before its fetch
// starts returns ctx's error and leaves the fetch to a later read.
func (reader *BlobReader) ReadBlob(ctx context.Context, number int, blobObjectIdentifier string) ([]byte, error) {
	contents, err := reader.readBlob(ctx, reader.directory, blobObjectIdentifier)
	if !errors.Is(err, ErrBlobNotFound) {
		return contents, err
	}
	// The pull request's lock is held through its fetch, so a second read
	// waiting for it finds the objects instead of fetching again, and a slow
	// fetch holds up no other pull request.
	lock := reader.lockFor(number)
	lock.Lock()
	defer lock.Unlock()
	if ctx.Err() != nil {
		return nil, ctx.Err() // the wait used up the deadline; no fetch ran
	}
	if !reader.markFetched(number) {
		if err := reader.fetchHead(ctx, reader.directory, reader.remoteName, number); err != nil {
			return nil, err
		}
	}
	return reader.readBlob(ctx, reader.directory, blobObjectIdentifier)
}

// lockFor returns the lock of pull request number.
func (reader *BlobReader) lockFor(number int) *sync.Mutex {
	reader.mutex.Lock()
	defer reader.mutex.Unlock()
	lock, ok := reader.locks[number]
	if !ok {
		lock = &sync.Mutex{}
		reader.locks[number] = lock
	}
	return lock
}

// markFetched records that pull request number's head fetch runs, and
// reports whether it had already.
func (reader *BlobReader) markFetched(number int) bool {
	reader.mutex.Lock()
	defer reader.mutex.Unlock()
	already := reader.fetched[number]
	reader.fetched[number] = true
	return already
}
