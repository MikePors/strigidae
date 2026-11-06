// Command imageprobe exercises image placement through Bubble Tea before the
// production markdown renderer adopts a graphics protocol. It uses generated
// pixels only: no network, credentials, or files are read.
package main

import (
	"bytes"
	"encoding/base64"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

const imageIdentifier = 424242

type model struct {
	protocol      string
	payload       string
	width, height int
	offset        int
	covered       bool
}

func main() {
	protocol := flag.String("protocol", "kitty", "kitty or iterm2")
	flag.Parse()
	if *protocol != "kitty" && *protocol != "iterm2" {
		_, _ = fmt.Fprintln(os.Stderr, "protocol must be kitty or iterm2")
		os.Exit(2)
	}
	payload, err := sampleImage()
	if err == nil {
		_, err = tea.NewProgram(model{protocol: *protocol, payload: payload}).Run()
	}
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func sampleImage() (string, error) {
	pixels := image.NewNRGBA(image.Rect(0, 0, 64, 32))
	for row := 0; row < 32; row++ {
		for column := 0; column < 64; column++ {
			pixels.SetNRGBA(column, row, color.NRGBA{R: uint8(column * 4), G: uint8(row * 8), B: 128, A: 255})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, pixels); err != nil {
		return "", fmt.Errorf("encode sample: %w", err)
	}
	return base64.StdEncoding.EncodeToString(encoded.Bytes()), nil
}

func (model) Init() tea.Cmd { return nil }

func (model model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		model.width, model.height = message.Width, message.Height
	case tea.KeyPressMsg:
		switch message.String() {
		case "q", "ctrl+c":
			if model.protocol == "kitty" {
				return model, tea.Sequence(tea.Raw(fmt.Sprintf("\x1b_Ga=d,d=I,i=%d,q=2\x1b\\", imageIdentifier)), tea.Quit)
			}
			return model, tea.Quit
		case "j":
			// Allow the first row after the image to reach the top, so the
			// probe can check cleanup when every image cell is off screen.
			model.offset = min(9, model.offset+1)
		case "k":
			model.offset = max(0, model.offset-1)
		case "h":
			model.covered = !model.covered
		case "r":
			return model, tea.Raw(model.transmission())
		}
	}
	return model, nil
}

func (model model) dimensions() (int, int) {
	return min(32, max(1, model.width-4)), 6
}

func (model model) transmission() string {
	columns, rows := model.dimensions()
	if model.protocol == "kitty" {
		return fmt.Sprintf("\x1b_Ga=T,f=100,t=d,U=1,q=2,i=%d,c=%d,r=%d;%s\x1b\\", imageIdentifier, columns, rows, model.payload)
	}
	// This intentionally tests Raw separately from View. Press r after a
	// repaint; there is no documented post-frame graphics hook in tea.View.
	top := 3 - model.offset
	if model.covered || model.width < 5 || top < 0 || top+rows > model.height-2 {
		return ""
	}
	return fmt.Sprintf("\x1b7\x1b[%d;3H\x1b]1337;File=inline=1;width=%d;height=%d;preserveAspectRatio=1:%s\a\x1b8", top+2, columns, rows, model.payload)
}

func (model model) View() tea.View {
	columns, rows := model.dimensions()
	lines := []string{"Image placement probe", "", "Before image"}
	for row := 0; row < rows; row++ {
		line := strings.Repeat(" ", columns)
		if model.protocol == "kitty" {
			line = placeholderRow(row, columns)
		}
		lines = append(lines, "  "+line)
	}
	lines = append(lines, "After image", "", "End of document")
	var visible []string
	for row := 0; row < max(0, model.height-2); row++ {
		line := ""
		if index := row + model.offset; index < len(lines) {
			line = lines[index]
		}
		if model.covered {
			line = "Overlay: image must be hidden"
		}
		visible = append(visible, ansi.Truncate(line, max(0, model.width), ""))
	}
	frame := []string{ansi.Truncate("r draw | j/k scroll | h overlay | q quit", max(0, model.width), "")}
	frame = append(frame, visible...)
	frame = append(frame, ansi.Truncate(model.protocol+": draw again after resize; inspect stale pixels", max(0, model.width), ""))
	frame = frame[:min(len(frame), max(0, model.height))]
	content := strings.Join(frame, "\n")
	view := tea.NewView(content)
	view.AltScreen = true
	return view
}

func placeholderRow(row, columns int) string {
	// The first six row diacritics from Kitty's protocol table. Explicit
	// row markers survive vertical clipping; columns inherit left to right.
	diacritics := []rune{0x0305, 0x030d, 0x030e, 0x0310, 0x0312, 0x033d}
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", imageIdentifier>>16, (imageIdentifier>>8)&255, imageIdentifier&255) +
		"\U0010eeee" + string(diacritics[row]) + strings.Repeat("\U0010eeee", columns-1) + "\x1b[39m"
}
