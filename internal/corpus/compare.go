package corpus

import (
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/ElatDev/vinspect/vin"
)

// Verdict is how a VIN fares under validation: "valid", "check_digit" or
// "character". (Every corpus VIN has 17 characters, so length never fails.)
type Verdict string

const (
	Valid         Verdict = "valid"
	BadCheckDigit Verdict = "check_digit"
	BadCharacter  Verdict = "character"
)

// The vPIC error codes that correspond to those verdicts.
const (
	vpicCheckDigit = "1"   // "Check Digit (9th position) does not calculate properly"
	vpicBadChars   = "400" // "Invalid Characters Present"
)

// OurVerdict classifies vinspect's validation of s.
func OurVerdict(s string) Verdict {
	switch err := vin.Validate(s); {
	case err == nil:
		return Valid
	case errors.Is(err, vin.ErrCheckDigit):
		return BadCheckDigit
	default:
		return BadCharacter
	}
}

// NHTSAVerdict classifies vPIC's decode of the same VIN from its error
// codes. vPIC reports every problem it finds; vinspect reports the first,
// checking characters before the check digit, so a bad character wins.
func NHTSAVerdict(d Decode) Verdict {
	switch {
	case d.HasError(vpicBadChars):
		return BadCharacter
	case d.HasError(vpicCheckDigit):
		return BadCheckDigit
	default:
		return Valid
	}
}

// Report is the outcome of comparing vinspect with NHTSA over the corpus.
type Report struct {
	Vehicles   int // rows in crash-test-vehicles.csv
	WithVIN    int // rows with any VIN recorded
	Candidates int // distinct 17-character VINs; everything below is over these

	// Validation, ours, and whether vPIC reached the same verdict.
	Valid, BadCheckDigit, BadCharacter int
	VerdictAgree                       int
	VerdictDisagree                    []Mismatch

	// Clean is the subset both decoders accept without complaint: vinspect
	// validates it and vPIC returns error code 0 alone. The manufacturer and
	// model-year measures below are over those VINs, because a VIN either
	// side calls malformed says nothing about whether the decoding rules are
	// right.
	Clean int

	// Manufacturer, over the clean subset.
	MfrCompared    int // vPIC named a manufacturer
	MfrAgree       int // our WMI snapshot names the same one
	MfrUnknownToUs []Mismatch
	MfrDisagree    []Mismatch

	// Model year, over the clean subset for which vPIC decoded a year. The
	// ways vinspect can answer are counted apart, because they claim
	// different things: a certain year is one the regulation guarantees, a
	// likely year is one position 7 merely favours, and an ambiguous answer
	// names both candidates.
	YearCompared  int
	YearCertain   int // guaranteed by the regulation, and vPIC agreed
	YearLikely    int // favoured by position 7, and vPIC agreed
	YearAmbiguous int // two candidates offered, vPIC's year among them
	YearNoCode    int // position 10 is not a year code, so no year is offered

	YearCertainWrong []Mismatch // a guarantee that did not hold: must stay empty
	YearLikelyWrong  []Mismatch // the favoured year was the other candidate
	YearDisagree     []Mismatch // vPIC's year is not a candidate at all
}

// Mismatch records one VIN where vinspect and vPIC differ.
type Mismatch struct {
	VIN, Ours, NHTSA string
}

// Compare runs vinspect over every candidate VIN and compares it with
// vPIC's decodes.
func Compare(vehicles []Vehicle, decodes map[string]Decode) (Report, error) {
	r := Report{Vehicles: len(vehicles)}
	for _, v := range vehicles {
		if strings.TrimSpace(v.VIN) != "" {
			r.WithVIN++
		}
	}
	vins := CandidateVINs(vehicles)
	r.Candidates = len(vins)

	for _, s := range vins {
		d, ok := decodes[s]
		if !ok {
			return r, errors.New("corpus: no vPIC decode for " + s + "; run go run ./tools/corpus vpic")
		}

		ours, theirs := OurVerdict(s), NHTSAVerdict(d)
		switch ours {
		case Valid:
			r.Valid++
		case BadCheckDigit:
			r.BadCheckDigit++
		case BadCharacter:
			r.BadCharacter++
		}
		if ours == theirs {
			r.VerdictAgree++
		} else {
			r.VerdictDisagree = append(r.VerdictDisagree, Mismatch{s, string(ours), string(theirs) + " (codes " + strings.Join(d.ErrorCodes, ",") + ")"})
		}
		if ours != Valid || !d.Clean() {
			continue // one side calls it malformed; decoding rules are not on trial
		}
		r.Clean++

		info, err := vin.Decode(s)
		if err != nil {
			return r, err
		}
		r.compareManufacturer(s, info, d)
		r.compareYear(s, info.ModelYear, d)
	}
	return r, nil
}

func (r *Report) compareManufacturer(s string, info vin.Info, d Decode) {
	if d.Manufacturer == "" {
		return
	}
	r.MfrCompared++
	switch {
	case info.Manufacturer == nil:
		r.MfrUnknownToUs = append(r.MfrUnknownToUs, Mismatch{s, "WMI " + info.WMI + " not in the snapshot", d.Manufacturer})
	case sameName(info.Manufacturer.Manufacturer, d.Manufacturer):
		r.MfrAgree++
	default:
		r.MfrDisagree = append(r.MfrDisagree, Mismatch{s, info.Manufacturer.Manufacturer, d.Manufacturer})
	}
}

func (r *Report) compareYear(s string, y vin.ModelYear, d Decode) {
	if d.ModelYear == 0 {
		return
	}
	r.YearCompared++
	theirs := strconv.Itoa(d.ModelYear)
	switch {
	case len(y.Candidates) == 0:
		r.YearNoCode++
	case y.Year == d.ModelYear && y.Certain:
		r.YearCertain++
	case y.Year == d.ModelYear:
		r.YearLikely++
	case y.Year != 0 && y.Certain:
		r.YearCertainWrong = append(r.YearCertainWrong, Mismatch{s, y.String(), theirs})
	case y.Year != 0 && slices.Contains(y.Candidates, d.ModelYear):
		// Position 7 favoured one cycle; vPIC says it is the other one.
		r.YearLikelyWrong = append(r.YearLikelyWrong, Mismatch{s, y.String(), theirs})
	case y.Year == 0 && slices.Contains(y.Candidates, d.ModelYear):
		r.YearAmbiguous++
	default:
		r.YearDisagree = append(r.YearDisagree, Mismatch{s, y.String(), theirs})
	}
}

func sameName(a, b string) bool {
	return strings.EqualFold(strings.Join(strings.Fields(a), " "), strings.Join(strings.Fields(b), " "))
}
