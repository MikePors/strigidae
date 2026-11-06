package main

import (
	"encoding/base64"
	"image/png"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestScrollPastImage(test *testing.T) {
	var probe tea.Model = model{protocol: "kitty", width: 80, height: 24}
	down := tea.KeyPressMsg{Code: 'j', Text: "j"}
	up := tea.KeyPressMsg{Code: 'k', Text: "k"}
	for range 8 {
		probe, _ = probe.Update(down)
	}
	if !strings.Contains(probe.View().Content, "\U0010eeee") {
		test.Fatal("last image row should still be visible before scrolling past it")
	}
	probe, _ = probe.Update(down)
	content := probe.View().Content
	if strings.Contains(content, "\U0010eeee") {
		test.Fatal("image remains visible after scrolling past its last row")
	}
	if !strings.Contains(content, "After image") {
		test.Fatal("text following the image should remain visible")
	}
	probe, _ = probe.Update(down)
	if probe.View().Content != content {
		test.Fatal("scrolling beyond the limit should leave the view unchanged")
	}
	probe, _ = probe.Update(up)
	if !strings.Contains(probe.View().Content, "\U0010eeee") {
		test.Fatal("scrolling back should reveal the last image row")
	}
}

func TestPlaceholderCells(test *testing.T) {
	for row := 0; row < 6; row++ {
		line := placeholderRow(row, 32)
		if ansi.StringWidth(line) != 32 || lipgloss.Width(line) != 32 {
			test.Fatalf("row %d: placeholder width is not 32 cells", row)
		}
		clipped := ansi.Truncate(line, 1, "")
		if ansi.StringWidth(clipped) != 1 || len([]rune(ansi.Strip(clipped))) != 2 {
			test.Fatalf("row %d: clipping lost the row marker: %q", row, clipped)
		}
	}
}

func TestSampleImage(test *testing.T) {
	payload, err := sampleImage()
	if err != nil {
		test.Fatal(err)
	}
	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		test.Fatal(err)
	}
	configuration, err := png.DecodeConfig(strings.NewReader(string(decoded)))
	if err != nil || configuration.Width != 64 || configuration.Height != 32 {
		test.Fatalf("sample dimensions: %+v, %v", configuration, err)
	}
	if len(payload) > 4096 {
		test.Fatal("sample exceeds one Kitty transmission chunk")
	}
}

func TestInlineVisibility(test *testing.T) {
	for _, testCase := range []struct {
		name             string
		offset, height   int
		covered, visible bool
	}{
		{name: "full", height: 24, visible: true},
		{name: "top clipped", offset: 4, height: 24},
		{name: "bottom clipped", height: 10},
		{name: "overlay", height: 24, covered: true},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			probe := model{protocol: "iterm2", width: 80, height: testCase.height, offset: testCase.offset, covered: testCase.covered}
			if visible := probe.transmission() != ""; visible != testCase.visible {
				test.Errorf("draw = %v, want %v", visible, testCase.visible)
			}
		})
	}
}
