package network

import (
	"context"
	"fmt"
	"strings"

	"github.com/MikePors/strigidae/internal/github"
)

const operationBlobTexts = "BlobTexts"

// blobFields is read for each blob alias. An abbreviated object identifier
// resolves to the object with that prefix; GitHub answers an unknown one with
// a null object, not an error.
const blobFields = `{ __typename oid ... on Blob { text isBinary isTruncated byteSize } }`

// blobTextsQuery builds one query with an alias per identifier. The
// identifiers travel as variables, never in the query text.
func blobTextsQuery(count int) string {
	var query strings.Builder
	query.WriteString("query " + operationBlobTexts + "($owner: String!, $name: String!")
	for i := range count {
		fmt.Fprintf(&query, ", $expression%d: String!", i)
	}
	query.WriteString(") {\n  repository(owner: $owner, name: $name) {\n")
	for i := range count {
		fmt.Fprintf(&query, "    blob%d: object(expression: $expression%d) %s\n", i, i, blobFields)
	}
	query.WriteString("  }\n}")
	return query.String()
}

type graphqlBlob struct {
	Typename    string `json:"__typename"`
	OID         string `json:"oid"`
	Text        *string
	IsBinary    bool
	IsTruncated bool
	ByteSize    int
}

// BlobTexts implements github.Client. It never logs or puts a file's text in
// an error.
func (client *Client) BlobTexts(ctx context.Context, repository github.Repository, objectIdentifiers []string) ([]github.BlobText, error) {
	if len(objectIdentifiers) == 0 {
		return nil, nil
	}
	var response struct {
		Repository map[string]*graphqlBlob
	}
	variables := map[string]any{"owner": repository.Owner, "name": repository.Name}
	for i, identifier := range objectIdentifiers {
		variables[fmt.Sprintf("expression%d", i)] = identifier
	}
	if err := client.query(ctx, blobTextsQuery(len(objectIdentifiers)), variables, &response); err != nil {
		return nil, fmt.Errorf("read %d blobs in %s: %w", len(objectIdentifiers), repository, err)
	}
	if response.Repository == nil {
		return nil, fmt.Errorf("read %d blobs in %s: %w", len(objectIdentifiers), repository, github.ErrNotFound)
	}
	texts := make([]github.BlobText, len(objectIdentifiers))
	for i, identifier := range objectIdentifiers {
		text, err := blobText(response.Repository[fmt.Sprintf("blob%d", i)], identifier)
		if err != nil {
			err = fmt.Errorf("read blob %s in %s: %w", identifier, repository, err)
		}
		texts[i] = github.BlobText{Text: text, Err: err}
	}
	return texts, nil
}

// blobText is the text of the object found for identifier, or the reason it
// is not usable. The returned identifier must start with the requested one:
// a ref named like a short identifier resolves to another object.
func blobText(object *graphqlBlob, identifier string) (string, error) {
	switch {
	case object == nil:
		return "", github.ErrNotFound
	case object.Typename != "Blob":
		return "", fmt.Errorf("not a file: %w", github.ErrNotFound)
	case identifier == "" || !strings.HasPrefix(strings.ToLower(object.OID), strings.ToLower(identifier)):
		return "", fmt.Errorf("found another object: %w", github.ErrNotFound)
	case object.IsBinary:
		return "", fmt.Errorf("binary file: %w", github.ErrNoFileText)
	case object.IsTruncated:
		return "", fmt.Errorf("truncated at %d bytes: %w", object.ByteSize, github.ErrNoFileText)
	case object.Text == nil:
		return "", fmt.Errorf("no text returned: %w", github.ErrNoFileText)
	}
	return *object.Text, nil
}
