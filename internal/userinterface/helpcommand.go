package userinterface

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/MikePors/strigidae/internal/options"
	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

// helpContent is what the help overlay shows.
type helpContent int

const (
	// helpKeys is the key help that `?` opens.
	helpKeys helpContent = iota
	// helpTopics is the list of help topics.
	helpTopics
	// helpConfig is the list of options.
	helpConfig
)

// helpTopic is one subject of `:help`.
type helpTopic struct {
	name        string
	description string
	content     helpContent
}

// helpTopicTable lists the topics: bare `:help` shows it and Tab completes
// from it. Add a topic here and nowhere else.
func helpTopicTable() []helpTopic {
	return []helpTopic{
		{name: "keys", description: "the key bindings of the current view (same as ?)", content: helpKeys},
		{name: "config", description: "every option, with its value, default and meaning", content: helpConfig},
	}
}

// helpRequestedMessage asks the root to open a help topic.
type helpRequestedMessage struct{ topic string }

// helpCommand is `:help [topic]`: it reaches the root as a message because
// the root owns the overlay and the option values.
func helpCommand(arguments string) tea.Cmd {
	return messageCommand(helpRequestedMessage{topic: arguments})
}

// help opens the overlay for topic, or the topic list when it is empty.
func (model *Model) help(topic string) tea.Cmd {
	if topic == "" {
		model.openHelp(helpTopics)
		return nil
	}
	for _, candidate := range helpTopicTable() {
		if candidate.name == topic {
			model.openHelp(candidate.content)
			return nil
		}
	}
	return Error(fmt.Errorf("unknown help topic: %s", terminaltext.Clean(topic)))
}

func (model *Model) openHelp(content helpContent) {
	model.showHelp = true
	model.helpContent = content
	model.helpOffset = 0
}

// helpTitle names the open overlay on its title line.
func helpTitle(content helpContent) string {
	switch content {
	case helpTopics:
		return "Help topics"
	case helpConfig:
		return "Help: config"
	case helpKeys:
	}
	return "Help"
}

// helpTopicLines lays out the topic list: names, then descriptions.
func helpTopicLines(activeTheme theme.Theme) []string {
	topics := helpTopicTable()
	width := 0
	for _, topic := range topics {
		width = max(width, lipgloss.Width(topic.name))
	}
	lines := []string{activeTheme.HelpSection.Render("Topics (:help <topic>)")}
	for _, topic := range topics {
		pad := strings.Repeat(" ", width-lipgloss.Width(topic.name)+2)
		lines = append(lines, activeTheme.HelpKey.Render(topic.name)+pad+activeTheme.HelpDescription.Render(topic.description))
	}
	return lines
}

// helpConfigLines lays out every option in four columns, then a footer. Values
// are written the way `:set` shows them, and a value that differs from its
// default is followed by an asterisk.
func helpConfigLines(activeTheme theme.Theme, values options.Values) []string {
	const gap = "  "
	var defaults options.Values
	rows := [][4]string{{"Option", "Value", "Default", "Description"}}
	for _, option := range options.All() {
		value := values.Describe(option.Name)
		if value != defaults.Describe(option.Name) {
			value += "*"
		}
		rows = append(rows, [4]string{option.Name, value, defaults.Describe(option.Name), option.Description})
	}
	var widths [3]int
	for _, row := range rows {
		for i := range widths {
			widths[i] = max(widths[i], lipgloss.Width(row[i]))
		}
	}
	lines := make([]string, len(rows))
	for index, row := range rows {
		style := activeTheme.HelpDescription
		if index == 0 {
			style = activeTheme.HelpSection
		}
		var line strings.Builder
		for i := range widths {
			cellStyle := style
			if index > 0 && i == 0 {
				cellStyle = activeTheme.HelpKey
			}
			line.WriteString(cellStyle.Render(row[i]) + strings.Repeat(" ", widths[i]-lipgloss.Width(row[i])) + gap)
		}
		line.WriteString(style.Render(row[3]))
		lines[index] = line.String()
	}
	return append(lines, "", activeTheme.Muted.Render(":set name / :set noname to change    * changed from the default"))
}
