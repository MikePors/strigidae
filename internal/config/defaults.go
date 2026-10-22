package config

import "strings"

// Limits and defaults for section options.
const (
	DefaultLimit = 30
	MaxLimit     = 100
	// Me is the user value meaning the signed-in viewer.
	Me = "@me"
)

// Sort is a GitHub search sort field.
type Sort string

// Supported sort fields.
const (
	SortUpdated      Sort = "updated"
	SortCreated      Sort = "created"
	SortComments     Sort = "comments"
	SortReactions    Sort = "reactions"
	SortInteractions Sort = "interactions"
)

var sorts = []Sort{SortUpdated, SortCreated, SortComments, SortReactions, SortInteractions}

// Order is the sort direction.
type Order string

// Supported orders.
const (
	OrderDesc Order = "desc"
	OrderAsc  Order = "asc"
)

// defaultSections are the sections every tab gets unless it lists its own.
// {user} is replaced by the tab's user.
func defaultSections() []Section {
	return []Section{
		{Name: "Review requested", Query: "is:pr is:open review-requested:{user}"},
		{Name: "My PRs", Query: "is:pr is:open author:{user}"},
		{Name: "Involved", Query: "is:pr is:open involves:{user} -author:{user}"},
	}
}

// Default is the configuration used when no file exists: one "Me" tab with the
// default sections.
func Default() Config {
	t := Tab{Name: "Me", User: Me, Sections: defaultSections()}
	t.resolve()
	return Config{Tabs: []Tab{t}}
}

// resolve fills section defaults and substitutes the tab's user into queries.
func (t *Tab) resolve() {
	for i := range t.Sections {
		s := &t.Sections[i]
		s.Query = strings.ReplaceAll(s.Query, "{user}", t.User)
		if s.Limit == 0 {
			s.Limit = DefaultLimit
		}
		if s.Sort == "" {
			s.Sort = SortUpdated
		}
		if s.Order == "" {
			s.Order = OrderDesc
		}
	}
}
