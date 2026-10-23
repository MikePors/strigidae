package github

// DefaultPageSize is used when PageRequest.Limit is zero.
const DefaultPageSize = 30

// MaxPageSize is the largest page the API accepts.
const MaximumPageSize = 100

// PageRequest asks for one page of a list. The zero value requests the first
// page with the default size.
type PageRequest struct {
	// After is the Next cursor of the previous Page, or empty for the first page.
	After string
	// Limit is the page size; zero means DefaultPageSize, values above
	// MaxPageSize are clamped.
	Limit int
}

// Size returns the effective page size.
func (pageRequest PageRequest) Size() int {
	switch {
	case pageRequest.Limit <= 0:
		return DefaultPageSize
	case pageRequest.Limit > MaximumPageSize:
		return MaximumPageSize
	default:
		return pageRequest.Limit
	}
}

// Page is one page of results. Pass Next as PageRequest.After to load the
// following page; the cursor is opaque.
type Page[Value any] struct {
	Items []Value
	// Next is the cursor for the following page, empty when there is none.
	Next string
	// Total is the total number of items across all pages, if known.
	Total int
}

// HasMore reports whether another page can be requested.
func (page Page[Value]) HasMore() bool { return page.Next != "" }
