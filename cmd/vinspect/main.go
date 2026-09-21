// Command vinspect validates and decodes Vehicle Identification Numbers.
//
//	vinspect check 1HGCM82633A004352
//	vinspect decode 1HGCM82633A004352
//	vinspect decode --online 1HGCM82633A004352
//	cut -d, -f3 inventory.csv | vinspect check --json
//
// Everything works offline except decode --online, which adds model, trim,
// body and engine details from NHTSA's vPIC API.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"
)

// version is set at release time with -ldflags "-X main.version=v0.1.0".
var version = ""

const usageText = `vinspect validates and decodes Vehicle Identification Numbers.

Usage:
  vinspect check  [--json] [VIN ...]
  vinspect decode [--json] [--online] [VIN ...]
  vinspect version

With no VINs, or "-", VINs are read from standard input, one per line.
Blank lines and lines starting with # are skipped.

Commands:
  check    report whether each VIN is well formed and its check digit is right
  decode   show what each VIN says: manufacturer, region, model year, plant

Flags:
  --json          print one JSON object per VIN (JSON Lines), for scripts
  --online        decode: also ask NHTSA's vPIC API for model, trim and engine
  --color WHEN    auto (default), always or never; NO_COLOR is respected

A model year printed as "2017 (likely)", or "2017?" in a table, is the year
position 7 favours but the regulation does not guarantee; "1983 or 2013" means
the VIN alone cannot choose. decode explains which rule it used.

Exit status: 0 if every VIN passed, 1 if any did not, 2 for usage errors.
vinspect is read-only: it checks and explains VINs and never generates them.
`

// restoreConsole undoes any terminal mode change made for colors. It is
// replaced by enableANSI on platforms where one is needed.
var restoreConsole = func() {}

func main() {
	code := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
	restoreConsole()
	os.Exit(code)
}

// run is main without the process: it takes the arguments and streams and
// returns the exit status, so tests can drive the whole command.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usageText)
		return 2
	}
	switch cmd, rest := args[0], args[1:]; cmd {
	case "check":
		return runCheck(rest, stdin, stdout, stderr)
	case "decode":
		return runDecode(rest, stdin, stdout, stderr)
	case "version", "--version", "-version":
		fmt.Fprintln(stdout, "vinspect", versionString())
		return 0
	case "help", "--help", "-help", "-h":
		fmt.Fprint(stdout, usageText)
		return 0
	default:
		fmt.Fprintf(stderr, "vinspect: unknown command %q\n\n%s", cmd, usageText)
		return 2
	}
}

func versionString() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "dev"
}

// commonFlags are the flags every command accepts.
type commonFlags struct {
	json  bool
	color string
}

func newFlagSet(name string, stderr io.Writer, c *commonFlags) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.BoolVar(&c.json, "json", false, "print one JSON object per VIN")
	fs.StringVar(&c.color, "color", "auto", "colorize output: auto, always or never")
	fs.Usage = func() { fmt.Fprint(stderr, usageText) }
	return fs
}

// parseArgs parses flags wherever they appear, so both
// "vinspect decode --json VIN" and "vinspect decode VIN --json" work, and
// returns the remaining arguments. "--" ends flag parsing.
func parseArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	var flags, rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			rest = append(rest, args[i+1:]...)
			break
		}
		if len(a) < 2 || a[0] != '-' {
			rest = append(rest, a)
			continue
		}
		flags = append(flags, a)
		name := strings.TrimLeft(a, "-")
		if strings.Contains(name, "=") {
			continue
		}
		// A non-boolean flag written "--flag value" takes the next argument.
		if f := fs.Lookup(name); f != nil && i+1 < len(args) {
			if b, ok := f.Value.(interface{ IsBoolFlag() bool }); !ok || !b.IsBoolFlag() {
				i++
				flags = append(flags, args[i])
			}
		}
	}
	if err := fs.Parse(flags); err != nil {
		return nil, err
	}
	return rest, nil
}

// flagError maps a flag parsing error to an exit status.
func flagError(err error) int {
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	return 2
}

// readInputs calls fn for each VIN to process: the arguments, or the lines
// of stdin when there are no arguments or the only argument is "-". Lines
// are trimmed; blank lines and # comments are skipped.
func readInputs(args []string, stdin io.Reader, fn func(string)) error {
	if len(args) > 0 && !(len(args) == 1 && args[0] == "-") {
		for _, a := range args {
			fn(a)
		}
		return nil
	}
	sc := bufio.NewScanner(stdin)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	first := true
	for sc.Scan() {
		line := sc.Text()
		if first {
			line = strings.TrimPrefix(line, "\uFEFF") // byte-order mark from Windows editors
			first = false
		}
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fn(line)
	}
	return sc.Err()
}

// display shortens an arbitrary input for a table cell.
func display(s string, width int) string {
	s = strings.Map(func(r rune) rune {
		if r < ' ' || r == 0x7f {
			return '?'
		}
		return r
	}, s)
	if r := []rune(s); len(r) > width {
		return string(r[:width-1]) + "…"
	}
	return s
}

// pad right-pads s with spaces to width runes.
func pad(s string, width int) string {
	if n := len([]rune(s)); n < width {
		return s + strings.Repeat(" ", width-n)
	}
	return s
}
