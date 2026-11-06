package userinterface

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

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
		return model.openHelp(helpTopics)
	}
	for _, candidate := range helpTopicTable() {
		if candidate.name == topic {
			return model.openHelp(candidate.content)
		}
	}
	return Error(fmt.Errorf("unknown help topic: %s", terminaltext.Clean(topic)))
}

// openHelp shows a page. A prompt that was open (the top view's, or the help's
// own) is cancelled first, so its results cannot reach the wrong receiver; the
// receiver hears about it in the returned command.
func (model *Model) openHelp(content helpContent) tea.Cmd {
	var command tea.Cmd
	if model.commandLine.active && model.commandLine.prompt != "" {
		result := PromptResultMessage{Prefix: model.commandLine.prompt, Text: model.commandLine.input.Value(), State: PromptCancelled}
		model.commandLine.close()
		command = model.deliverPrompt(result) // the prompt may be the help's own
	}
	model.showHelp = true
	model.helpContent = content
	model.helpOffset = 0
	model.helpSearch = helpSearch{last: model.helpSearch.last} // the query belongs to the page; last is kept so an empty enter repeats it
	return command
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

// minimumDescriptionWidth is the narrowest the description column may be
// before the table stacks each description under its option.
const minimumDescriptionWidth = 20

// stackedIndent is how far a stacked description is indented.
const stackedIndent = 4

// wrapWords breaks text into lines of at most width columns, at spaces where
// it can and inside a word only when the word alone is wider than width.
func wrapWords(text string, width int) []string {
	return strings.Split(ansi.Wrap(text, max(1, width), ""), "\n")
}

// helpConfigLines lays out every option, then a footer. Values are written the
// way `:set` shows them, and a value that differs from its default is followed
// by an asterisk. With room for a description column of at least
// minimumDescriptionWidth the table has four columns and a long description
// wraps onto the following lines, under its column. Otherwise each
// description is stacked on the lines under its option.
func helpConfigLines(activeTheme theme.Theme, values options.Values, width int) []string {
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
	leftWidth := widths[0] + widths[1] + widths[2] + 3*len(gap)
	stacked := width-leftWidth < minimumDescriptionWidth
	columns := len(widths) + 1
	descriptionWidth := width - leftWidth
	indent := strings.Repeat(" ", leftWidth)
	if stacked {
		columns = len(widths)
		descriptionWidth = width - stackedIndent
		indent = strings.Repeat(" ", stackedIndent)
	}
	var lines []string
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
			line.WriteString(cellStyle.Render(row[i]))
			if i < columns-1 {
				line.WriteString(strings.Repeat(" ", widths[i]-lipgloss.Width(row[i])) + gap)
			}
		}
		descriptionLines := wrapWords(row[3], descriptionWidth)
		if stacked && index == 0 {
			descriptionLines = nil
		}
		if !stacked {
			line.WriteString(style.Render(descriptionLines[0]))
			descriptionLines = descriptionLines[1:]
		}
		lines = append(lines, line.String())
		for _, continuation := range descriptionLines {
			lines = append(lines, indent+style.Render(continuation))
		}
	}
	lines = append(lines, "")
	for _, footerLine := range wrapWords(":set name / :set noname to change    * changed from the default", width) {
		lines = append(lines, activeTheme.Muted.Render(footerLine))
	}
	return lines
}
