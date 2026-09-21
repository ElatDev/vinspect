package main

import (
	"fmt"
	"io"
	"os"
)

// style applies ANSI colors when they are wanted, and is a no-op otherwise.
type style struct{ on bool }

// newStyle decides whether to color output written to w. "auto" colors only
// a terminal, and only when NO_COLOR is unset (https://no-color.org).
func newStyle(when string, w io.Writer) (style, error) {
	switch when {
	case "always":
		// A Windows console still needs escape processing switched on, even
		// when the user insists on color.
		if f, ok := w.(*os.File); ok {
			enableANSI(f)
		}
		return style{on: true}, nil
	case "never":
		return style{}, nil
	case "auto", "":
		if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
			return style{}, nil
		}
		f, ok := w.(*os.File)
		return style{on: ok && isTerminal(f) && enableANSI(f)}, nil
	default:
		return style{}, fmt.Errorf("--color must be auto, always or never, not %q", when)
	}
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func (s style) wrap(code, text string) string {
	if !s.on || text == "" {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

func (s style) bold(t string) string    { return s.wrap("1", t) }
func (s style) dim(t string) string     { return s.wrap("2", t) }
func (s style) red(t string) string     { return s.wrap("31", t) }
func (s style) green(t string) string   { return s.wrap("32", t) }
func (s style) yellow(t string) string  { return s.wrap("33", t) }
func (s style) blue(t string) string    { return s.wrap("34", t) }
func (s style) magenta(t string) string { return s.wrap("35", t) }
func (s style) cyan(t string) string    { return s.wrap("36", t) }
