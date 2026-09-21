package corpus

import (
	"fmt"
	"strconv"
	"strings"
)

// Markers delimit the generated block in README.md, so tools/corpus report
// can rewrite the numbers and a test can check they are current.
const (
	MarkerStart = "<!-- corpus-report -->"
	MarkerEnd   = "<!-- /corpus-report -->"
)

// Markdown renders the report as the block between the README markers.
func (r Report) Markdown(crashRetrieved, wmiRetrieved string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", MarkerStart)
	fmt.Fprintf(&b, "**%s distinct 17-character VINs**, every one NHTSA recorded for a vehicle in its\n", n(r.Candidates))
	fmt.Fprintf(&b, "crash-test database (%s test vehicles, snapshot %s), each checked here and against\n", n(r.Vehicles), crashRetrieved)
	fmt.Fprintf(&b, "NHTSA's own vPIC decoder, with the WMI registry snapshot of %s.\n\n", wmiRetrieved)

	fmt.Fprintf(&b, "| Measure | Result |\n| --- | ---: |\n")
	fmt.Fprintf(&b, "| **Validation verdict matches vPIC** | **%s** |\n", ratio(r.VerdictAgree, r.Candidates))
	fmt.Fprintf(&b, "| &nbsp;&nbsp;valid | %s |\n", n(r.Valid))
	fmt.Fprintf(&b, "| &nbsp;&nbsp;check digit wrong | %s |\n", n(r.BadCheckDigit))
	fmt.Fprintf(&b, "| &nbsp;&nbsp;illegal character | %s |\n", n(r.BadCharacter))
	fmt.Fprintf(&b, "| Accepted by both decoders, so measured below | %s |\n", ratio(r.Clean, r.Candidates))
	fmt.Fprintf(&b, "| **Manufacturer matches vPIC** | **%s** |\n", ratio(r.MfrAgree, r.MfrCompared))
	fmt.Fprintf(&b, "| &nbsp;&nbsp;WMI absent from the snapshot | %s |\n", n(len(r.MfrUnknownToUs)))
	fmt.Fprintf(&b, "| **Model year: vPIC's year is one we offered** | **%s** |\n",
		ratio(r.YearCertain+r.YearLikely+r.YearAmbiguous+len(r.YearLikelyWrong), r.YearCompared))
	fmt.Fprintf(&b, "| &nbsp;&nbsp;year the regulation guarantees, and vPIC agreed | %s |\n", ratio(r.YearCertain, r.YearCertain+len(r.YearCertainWrong)))
	fmt.Fprintf(&b, "| &nbsp;&nbsp;year position 7 favours, and vPIC agreed | %s |\n", ratio(r.YearLikely, r.YearLikely+len(r.YearLikelyWrong)))
	fmt.Fprintf(&b, "| &nbsp;&nbsp;two candidates offered, vPIC's year among them | %s |\n", n(r.YearAmbiguous))
	if r.YearNoCode > 0 {
		fmt.Fprintf(&b, "| &nbsp;&nbsp;no model-year code in position 10 | %s |\n", n(r.YearNoCode))
	}
	if len(r.YearDisagree) > 0 {
		fmt.Fprintf(&b, "| &nbsp;&nbsp;vPIC's year is neither candidate | %s |\n", n(len(r.YearDisagree)))
	}
	fmt.Fprintf(&b, "%s\n", MarkerEnd)
	return b.String()
}

// Detail lists the individual disagreements, for the report command.
func (r Report) Detail() string {
	var b strings.Builder
	section := func(title string, ms []Mismatch) {
		fmt.Fprintf(&b, "\n%s: %d\n", title, len(ms))
		for i, m := range ms {
			if i == 25 {
				fmt.Fprintf(&b, "  ... and %d more\n", len(ms)-i)
				break
			}
			fmt.Fprintf(&b, "  %s  vinspect: %-42s vPIC: %s\n", m.VIN, m.Ours, m.NHTSA)
		}
	}
	section("Validation verdicts that differ", r.VerdictDisagree)
	section("Manufacturers that differ", r.MfrDisagree)
	section("WMIs missing from the snapshot", r.MfrUnknownToUs)
	section("Guaranteed model years that did not hold", r.YearCertainWrong)
	section("Likely model years that were the other candidate", r.YearLikelyWrong)
	section("Model years vPIC decoded outside both candidates", r.YearDisagree)
	return b.String()
}

func ratio(got, total int) string {
	if total == 0 {
		return "n/a"
	}
	pct := 100 * float64(got) / float64(total)
	digits := 0
	if got != total && pct > 99 {
		digits = 2 // don't round a real failure up to a clean 100%
	}
	return fmt.Sprintf("%s / %s (%s%%)", n(got), n(total), strconv.FormatFloat(pct, 'f', digits, 64))
}

// n formats an integer with thousands separators.
func n(v int) string {
	s := strconv.Itoa(v)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
