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

func TestDeleteImageProtocol(test *testing.T) {
	message, ok := deleteImage()().(tea.RawMsg)
	if !ok || message.Msg != "\x1b_Ga=d,d=I,i=424242,q=2\x1b\\" {
		test.Fatalf("delete command = %#v, want targeted image-data deletion", message)
	}
}

func TestTransmissionProtocol(test *testing.T) {
	for _, testCase := range []struct {
		name, protocol        string
		width, height, offset int
		covered               bool
		want                  string
	}{
		{
			name: "kitty virtual placement", protocol: "kitty", width: 80, height: 24,
			want: "\x1b_Ga=T,f=100,t=d,U=1,q=2,i=424242,c=32,r=6;YWJj\x1b\\",
		},
		{
			name: "inline origin", protocol: "iterm2", width: 80, height: 24,
			want: "\x1b7\x1b[5;3H\x1b]1337;File=inline=1;width=32;height=6;preserveAspectRatio=1:YWJj\a\x1b8",
		},
		{
			name: "top edge fits", protocol: "iterm2", width: 80, height: 24, offset: 3,
			want: "\x1b7\x1b[2;3H\x1b]1337;File=inline=1;width=32;height=6;preserveAspectRatio=1:YWJj\a\x1b8",
		},
		{name: "top edge clipped", protocol: "iterm2", width: 80, height: 24, offset: 4},
		{
			name: "bottom edge fits", protocol: "iterm2", width: 80, height: 11,
			want: "\x1b7\x1b[5;3H\x1b]1337;File=inline=1;width=32;height=6;preserveAspectRatio=1:YWJj\a\x1b8",
		},
		{name: "bottom edge clipped", protocol: "iterm2", width: 80, height: 10},
		{
			name: "width threshold accepted", protocol: "iterm2", width: 5, height: 24,
			want: "\x1b7\x1b[5;3H\x1b]1337;File=inline=1;width=1;height=6;preserveAspectRatio=1:YWJj\a\x1b8",
		},
		{name: "below width threshold", protocol: "iterm2", width: 4, height: 24},
		{name: "overlay", protocol: "iterm2", width: 80, height: 24, covered: true},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			probe := model{protocol: testCase.protocol, payload: "YWJj", width: testCase.width, height: testCase.height, offset: testCase.offset, covered: testCase.covered}
			if got := probe.transmission(); got != testCase.want {
				test.Errorf("transmission = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestPlaceholderProtocol(test *testing.T) {
	for row, marker := range []string{"\u0305", "\u030d", "\u030e", "\u0310", "\u0312", "\u033d"} {
		// These bytes are protocol data: 6*65536 + 121*256 + 50 = 424242.
		want := "\x1b[38;2;6;121;50m\U0010eeee" + marker + "\U0010eeee\x1b[39m"
		if got := placeholderRow(row, 2); got != want {
			test.Errorf("row %d = %q, want %q", row, got, want)
		}
	}
}
