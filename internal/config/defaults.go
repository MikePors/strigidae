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

var sortNames = []string{
	string(SortUpdated), string(SortCreated), string(SortComments),
	string(SortReactions), string(SortInteractions),
}

var orderNames = []string{string(OrderAscending), string(OrderDescending)}

// Order is the sort direction.
type Order string

// Supported orders.
const (
	OrderDescending Order = "desc"
	OrderAscending  Order = "asc"
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
	tab := Tab{Name: "Me", User: Me, Sections: defaultSections()}
	tab.resolve()
	return Config{Tabs: []Tab{tab}}
}

// resolve fills section defaults and substitutes the tab's user into queries.
func (tab *Tab) resolve() {
	for i := range tab.Sections {
		section := &tab.Sections[i]
		section.Query = strings.ReplaceAll(section.Query, "{user}", tab.User)
		if section.Limit == 0 {
			section.Limit = DefaultLimit
		}
		if section.Sort == "" {
			section.Sort = SortUpdated
		}
		if section.Order == "" {
			section.Order = OrderDescending
		}
	}
}
