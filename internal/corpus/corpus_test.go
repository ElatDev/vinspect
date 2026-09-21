package corpus

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/ElatDev/vinspect/wmi"
)

const (
	crashPath  = "../../testdata/corpus/crash-test-vehicles.csv"
	decodePath = "../../testdata/corpus/vpic-decodes.csv"
	readmePath = "../../README.md"
)

func load(t *testing.T) (Report, string) {
	t.Helper()
	rep, retrieved, err := Load(crashPath, decodePath)
	if err != nil {
		t.Fatal(err)
	}
	return rep, retrieved
}

// TestCorpus is the claim in the README, re-checked: every VIN NHTSA has
// recorded for a crash-tested vehicle, validated and decoded here and
// compared with NHTSA's own vPIC decode of the same VIN. Both files are
// committed, so this runs offline.
func TestCorpus(t *testing.T) {
	rep, retrieved := load(t)
	t.Logf("%d vehicles (%s), %d distinct 17-character VINs", rep.Vehicles, retrieved, rep.Candidates)
	t.Logf("verdicts: %d valid, %d bad check digit, %d illegal character", rep.Valid, rep.BadCheckDigit, rep.BadCharacter)
	t.Logf("manufacturer: %d/%d agree, %d WMIs absent from the snapshot", rep.MfrAgree, rep.MfrCompared, len(rep.MfrUnknownToUs))
	t.Logf("model year: %d certain, %d likely, %d ambiguous, %d compared", rep.YearCertain, rep.YearLikely, rep.YearAmbiguous, rep.YearCompared)

	if rep.Candidates < 1000 {
		t.Fatalf("only %d VINs in the corpus; it looks truncated", rep.Candidates)
	}

	// vinspect and vPIC reach the same verdict on all but a handful of VINs,
	// and every one of those is understood. A disagreement that is not one of
	// these kinds is a bug in the validator, so it fails the test.
	t.Run("validation verdicts differ only in understood ways", func(t *testing.T) {
		// VINs NHTSA published with the serial masked out. X is a legal VIN
		// character, so vinspect sees only a check digit that does not
		// compute; vPIC also knows the last five characters of a light
		// vehicle must be numeric.
		masked := regexp.MustCompile(`X{3,}$`)
		// Placeholders and one real manufacturing error, with vPIC's reading.
		known := map[string]string{
			"00000000000000000": "seventeen zeros, whose check digit computes; vPIC rejects the characters",
			"JDA000L6000823321": "placeholder-looking VIN vPIC rejects the characters of",
			"KMTGE4S1PRU008162": "2024 Genesis G80 whose P in position 9 cannot be a check digit; " +
				"vPIC accepts it under a documented OEM production-error exception",
		}
		for _, m := range rep.VerdictDisagree {
			switch {
			case masked.MatchString(m.VIN):
			case known[m.VIN] != "":
				t.Logf("known exception %s: %s", m.VIN, known[m.VIN])
			default:
				t.Errorf("unexplained disagreement: %s vinspect %s, vPIC %s", m.VIN, m.Ours, m.NHTSA)
			}
		}
		t.Logf("%d of %d VINs disagree, all of them understood", len(rep.VerdictDisagree), rep.Candidates)
	})

	t.Run("manufacturer matches NHTSA", func(t *testing.T) {
		if n := len(rep.MfrDisagree); n > 0 {
			t.Errorf("%d of %d VINs decode to a different manufacturer:%s", n, rep.MfrCompared, listing(rep.MfrDisagree))
		}
	})

	// A "certain" year is one 49 CFR 565.15 guarantees: position 7 holds a
	// digit, and since model year 2010 a car or MPV must hold a letter there.
	// If vPIC ever disagrees with one of those, the reading of the regulation
	// is wrong, not the data.
	t.Run("guaranteed model years hold", func(t *testing.T) {
		if n := len(rep.YearCertainWrong); n > 0 {
			t.Errorf("%d of %d guaranteed years disagree with vPIC:%s", n, rep.YearCertain+n, listing(rep.YearCertainWrong))
		}
	})

	// The remaining year answers are weaker by construction, so they are
	// measured rather than asserted; the README publishes the counts and
	// TestREADMEIsCurrent keeps them honest.
	t.Run("weaker model-year answers are measured", func(t *testing.T) {
		t.Logf("%d likely years were the other candidate", len(rep.YearLikelyWrong))
		t.Logf("%d years vPIC decoded outside both candidates", len(rep.YearDisagree))
		for _, m := range rep.YearLikelyWrong {
			t.Logf("  likely wrong: %s vinspect %s, vPIC %s", m.VIN, m.Ours, m.NHTSA)
		}
		for _, m := range rep.YearDisagree {
			t.Logf("  outside both: %s vinspect %s, vPIC %s", m.VIN, m.Ours, m.NHTSA)
		}
	})
}

// TestREADMEIsCurrent keeps the published numbers honest: the table in the
// README must be the one the committed corpus produces right now.
func TestREADMEIsCurrent(t *testing.T) {
	rep, retrieved := load(t)
	src, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(src), MarkerStart)
	end := strings.Index(string(src), MarkerEnd)
	if start < 0 || end < start {
		t.Fatalf("README is missing the %s block", MarkerStart)
	}
	got := strings.ReplaceAll(string(src[start:end+len(MarkerEnd)]), "\r\n", "\n")
	want := strings.TrimSuffix(rep.Markdown(retrieved, wmi.Retrieved()), "\n")
	if got != want {
		t.Errorf("README's corpus block is out of date; run:\n\tgo run ./tools/corpus report -readme README.md\n\n--- README ---\n%s\n\n--- current ---\n%s", got, want)
	}
}

func listing(ms []Mismatch) string {
	var b strings.Builder
	for i, m := range ms {
		if i == 20 {
			b.WriteString("\n  ... and more")
			break
		}
		b.WriteString("\n  " + m.VIN + "  vinspect: " + m.Ours + "  vPIC: " + m.NHTSA)
	}
	return b.String()
}
