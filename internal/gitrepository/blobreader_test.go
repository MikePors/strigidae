package gitrepository

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// fakeCheckout is a checkout whose blobs appear once their pull request's head
// is fetched.
type fakeCheckout struct {
	present   map[string]string
	afterHead map[int]map[string]string
	fetches   []int
	fetchErr  error
}

func (checkout *fakeCheckout) reader() *BlobReader {
	reader := NewBlobReader("/checkout", "origin")
	reader.readBlob = func(_ context.Context, directory, identifier string) ([]byte, error) {
		if directory != "/checkout" {
			return nil, fmt.Errorf("directory %q", directory)
		}
		if text, ok := checkout.present[identifier]; ok {
			return []byte(text), nil
		}
		return nil, fmt.Errorf("%w: %s", ErrBlobNotFound, identifier)
	}
	reader.fetchHead = func(_ context.Context, _, remoteName string, number int) error {
		if remoteName != "origin" {
			return fmt.Errorf("remote %q", remoteName)
		}
		checkout.fetches = append(checkout.fetches, number)
		if checkout.fetchErr != nil {
			return checkout.fetchErr
		}
		for identifier, text := range checkout.afterHead[number] {
			checkout.present[identifier] = text
		}
		return nil
	}
	return reader
}

func TestBlobReaderFetchesOncePerPullRequest(test *testing.T) {
	test.Parallel()
	checkout := &fakeCheckout{
		present:   map[string]string{"aaaa": "base"},
		afterHead: map[int]map[string]string{7: {"bbbb": "head"}},
	}
	reader := checkout.reader()
	ctx := context.Background()

	if text, err := reader.ReadBlob(ctx, 7, "aaaa"); err != nil || string(text) != "base" {
		test.Fatalf("present blob = %q, %v", text, err)
	}
	if len(checkout.fetches) != 0 {
		test.Fatalf("a present blob fetched: %v", checkout.fetches)
	}
	if text, err := reader.ReadBlob(ctx, 7, "bbbb"); err != nil || string(text) != "head" {
		test.Fatalf("blob after fetch = %q, %v", text, err)
	}
	if _, err := reader.ReadBlob(ctx, 7, "cccc"); !errors.Is(err, ErrBlobNotFound) {
		test.Fatalf("blob missing after fetch: err = %v, want ErrBlobNotFound", err)
	}
	if _, err := reader.ReadBlob(ctx, 8, "cccc"); !errors.Is(err, ErrBlobNotFound) {
		test.Fatalf("other pull request: err = %v, want ErrBlobNotFound", err)
	}
	if diff := cmp.Diff([]int{7, 8}, checkout.fetches); diff != "" {
		test.Errorf("fetches (-want +got):\n%s", diff)
	}
}

func TestBlobReaderDoesNotRetryAFailedFetch(test *testing.T) {
	test.Parallel()
	fetchErr := errors.New("offline")
	checkout := &fakeCheckout{present: map[string]string{}, fetchErr: fetchErr}
	reader := checkout.reader()
	if _, err := reader.ReadBlob(context.Background(), 3, "aaaa"); !errors.Is(err, fetchErr) {
		test.Fatalf("err = %v, want the fetch error", err)
	}
	if _, err := reader.ReadBlob(context.Background(), 3, "bbbb"); !errors.Is(err, ErrBlobNotFound) {
		test.Fatalf("second read: err = %v, want ErrBlobNotFound", err)
	}
	if diff := cmp.Diff([]int{3}, checkout.fetches); diff != "" {
		test.Errorf("fetches (-want +got):\n%s", diff)
	}
}

func TestBlobReaderLeavesTheFetchToALaterReadWhenTheDeadlineHasPassed(test *testing.T) {
	test.Parallel()
	checkout := &fakeCheckout{
		present:   map[string]string{},
		afterHead: map[int]map[string]string{5: {"bbbb": "head"}},
	}
	reader := checkout.reader()
	expired, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := reader.ReadBlob(expired, 5, "bbbb"); !errors.Is(err, context.Canceled) {
		test.Fatalf("err = %v, want context.Canceled", err)
	}
	if len(checkout.fetches) != 0 {
		test.Fatalf("fetched with an expired context: %v", checkout.fetches)
	}
	if text, err := reader.ReadBlob(context.Background(), 5, "bbbb"); err != nil || string(text) != "head" {
		test.Fatalf("later read = %q, %v", text, err)
	}
}

func TestRedactCredentials(test *testing.T) {
	test.Parallel()
	tests := []struct{ diagnostics, want string }{
		{"fatal: unable to access 'https://user:ghp_secret@example.test/o/r.git/': 403", "fatal: unable to access 'https://[redacted]@example.test/o/r.git/': 403"},
		{"fatal: could not read from https://token@a.test/x and https://b.test/y", "fatal: could not read from https://[redacted]@a.test/x and https://b.test/y"},
		{"fatal: unable to access 'https://u:p@ss@example.test/o/r.git/'", "fatal: unable to access 'https://[redacted]@example.test/o/r.git/'"},
		{"git@github.com: Permission denied (publickey).", "git@github.com: Permission denied (publickey)."},
	}
	for _, testCase := range tests {
		if got := redactCredentials(testCase.diagnostics); got != testCase.want {
			test.Errorf("redactCredentials(%q) = %q, want %q", testCase.diagnostics, got, testCase.want)
		}
	}
}
