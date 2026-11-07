package network

import (
	"context"
	"fmt"

	"github.com/MikePors/strigidae/internal/github"
)

const operationFileText = "FileText"

// The expression "<commit>:<path>" names the file's blob in that commit.
// GitHub answers a missing path or commit with a null object, not an error.
const fileTextQuery = `
query ` + operationFileText + `($owner: String!, $name: String!, $expression: String!) {
  repository(owner: $owner, name: $name) {
    object(expression: $expression) {
      __typename
      ... on Blob { text isBinary isTruncated byteSize }
    }
  }
}`

// FileText implements github.Client. It never logs or puts the file's text in
// an error.
func (client *Client) FileText(ctx context.Context, repository github.Repository, commitObjectIdentifier, path string) (string, error) {
	var response struct {
		Repository *struct {
			Object *struct {
				Typename    string `json:"__typename"`
				Text        *string
				IsBinary    bool
				IsTruncated bool
				ByteSize    int
			}
		}
	}
	variables := map[string]any{"owner": repository.Owner, "name": repository.Name, "expression": commitObjectIdentifier + ":" + path}
	if err := client.query(ctx, fileTextQuery, variables, &response); err != nil {
		return "", fmt.Errorf("read %s at %s in %s: %w", path, commitObjectIdentifier, repository, err)
	}
	if response.Repository == nil || response.Repository.Object == nil {
		return "", fmt.Errorf("read %s at %s in %s: %w", path, commitObjectIdentifier, repository, github.ErrNotFound)
	}
	object := response.Repository.Object
	switch {
	case object.Typename != "Blob":
		return "", fmt.Errorf("read %s at %s in %s: not a file: %w", path, commitObjectIdentifier, repository, github.ErrNotFound)
	case object.IsBinary:
		return "", fmt.Errorf("read %s at %s in %s: binary file: %w", path, commitObjectIdentifier, repository, github.ErrNoFileText)
	case object.IsTruncated:
		return "", fmt.Errorf("read %s at %s in %s: truncated at %d bytes: %w", path, commitObjectIdentifier, repository, object.ByteSize, github.ErrNoFileText)
	case object.Text == nil:
		return "", fmt.Errorf("read %s at %s in %s: no text returned: %w", path, commitObjectIdentifier, repository, github.ErrNoFileText)
	}
	return *object.Text, nil
}
