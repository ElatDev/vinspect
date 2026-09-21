// Command demo renders the README's terminal picture by running vinspect
// with colors forced on and turning the ANSI output into an SVG.
//
//	go run ./tools/demo -o docs/demo.svg
//
// Keeping the picture generated means it cannot drift from what the tool
// actually prints: rerun this after changing the output format.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// The commands the picture shows, in order.
var script = [][]string{
	{"check", "1HGCM82633A004352", "1HGCM82633A004353", "1FABP39XZDG107202"},
	{"decode", "1HGCM82633A004352"},
}

// A GitHub-dark-ish palette for the eight ANSI colors used.
var palette = map[string]string{
	"30": "#484f58", "31": "#ff7b72", "32": "#3fb950", "33": "#d29922",
	"34": "#58a6ff", "35": "#bc8cff", "36": "#39c5cf", "37": "#b1bac4",
}

const (
	foreground = "#c9d1d9"
	background = "#0d1117"
	chrome     = "#161b22"
	promptFG   = "#3fb950"
	charWidth  = 8.4
	lineHeight = 20.0
	fontSize   = 14.0
	padX       = 18.0
	titleBar   = 34.0
)

// span is a run of characters sharing one style.
type span struct {
	text        string
	color       string
	bold, faint bool
	prompt      bool
}

func main() {
	out := flag.String("o", "docs/demo.svg", "output SVG path")
	flag.Parse()
	log.SetFlags(0)

	var lines [][]span
	for i, args := range script {
		if i > 0 {
			lines = append(lines, nil)
		}
		lines = append(lines, []span{{text: "$ vinspect " + strings.Join(args, " "), prompt: true}})
		output, err := runCommand(args)
		if err != nil {
			log.Fatal(err)
		}
		lines = append(lines, parseANSI(output)...)
	}
	if err := os.WriteFile(*out, []byte(renderSVG(lines)), 0o644); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %s (%d lines)", *out, len(lines))
}

// runCommand runs the CLI with color forced on and returns what it printed.
func runCommand(args []string) (string, error) {
	cmd := exec.Command("go", append([]string{"run", "./cmd/vinspect"}, append(args, "--color=always")...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	// check exits 1 when a VIN is invalid, which the demo expects.
	if err := cmd.Run(); err != nil && stdout.Len() == 0 {
		return "", fmt.Errorf("%v: %s", err, &stderr)
	}
	return strings.TrimRight(stdout.String(), "\n"), nil
}

// parseANSI turns text with SGR escape sequences into styled spans.
func parseANSI(text string) [][]span {
	var lines [][]span
	var cur span
	var line []span
	flush := func() {
		if cur.text != "" {
			line = append(line, cur)
			cur.text = ""
		}
	}
	for i := 0; i < len(text); {
		switch {
		case strings.HasPrefix(text[i:], "\x1b["):
			end := strings.IndexByte(text[i:], 'm')
			if end < 0 {
				i += 2
				continue
			}
			flush()
			for _, code := range strings.Split(text[i+2:i+end], ";") {
				switch code {
				case "", "0":
					cur = span{}
				case "1":
					cur.bold = true
				case "2":
					cur.faint = true
				default:
					if _, ok := palette[code]; ok {
						cur.color = code
					}
				}
			}
			i += end + 1
		case text[i] == '\n':
			flush()
			lines = append(lines, line)
			line = nil
			i++
		default:
			j := i
			for j < len(text) && text[j] != '\n' && text[j] != '\x1b' {
				j++
			}
			cur.text += text[i:j]
			i = j
		}
	}
	flush()
	return append(lines, line)
}

func renderSVG(lines [][]span) string {
	cols := 0
	for _, line := range lines {
		n := 0
		for _, s := range line {
			n += len([]rune(s.text))
		}
		cols = max(cols, n)
	}
	width := 2*padX + float64(cols)*charWidth
	height := titleBar + lineHeight*float64(len(lines)) + 16

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f" font-family="ui-monospace, SFMono-Regular, Menlo, Consolas, monospace" font-size="%.0f">`+"\n",
		width, height, width, height, fontSize)
	fmt.Fprintf(&b, `<rect width="%.0f" height="%.0f" rx="10" fill="%s"/>`+"\n", width, height, background)
	fmt.Fprintf(&b, `<path d="M0 10a10 10 0 0 1 10-10h%.0fa10 10 0 0 1 10 10v%.0fH0z" fill="%s"/>`+"\n", width-20, titleBar-10, chrome)
	for i, c := range []string{"#ff5f57", "#febc2e", "#28c840"} {
		fmt.Fprintf(&b, `<circle cx="%.0f" cy="17" r="6" fill="%s"/>`+"\n", 20+float64(i)*20, c)
	}
	fmt.Fprintf(&b, `<text x="%.0f" y="22" fill="#8b949e" font-size="12">vinspect</text>`+"\n", width/2-28)

	for row, line := range lines {
		y := titleBar + lineHeight*float64(row) + 15
		col := 0.0
		for _, s := range line {
			if strings.TrimSpace(s.text) == "" {
				col += float64(len([]rune(s.text)))
				continue
			}
			fill := foreground
			if s.prompt {
				fill = promptFG
			} else if c, ok := palette[s.color]; ok {
				fill = c
			}
			attrs := ""
			if s.bold {
				attrs += ` font-weight="600"`
			}
			if s.faint {
				attrs += ` opacity="0.62"`
			}
			// textLength pins each run to the character grid, so box-drawing
			// glyphs taken from a fallback font cannot shift the columns.
			runes := float64(len([]rune(s.text)))
			fmt.Fprintf(&b, `<text x="%s" y="%.0f" fill="%s"%s textLength="%s" lengthAdjust="spacingAndGlyphs" xml:space="preserve">%s</text>`+"\n",
				f(padX+col*charWidth), y, fill, attrs, f(runes*charWidth), escape(s.text))
			col += runes
		}
	}
	b.WriteString("</svg>\n")
	return b.String()
}

// f formats a coordinate with one decimal place.
func f(v float64) string { return strconv.FormatFloat(v, 'f', 1, 64) }

func escape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}
