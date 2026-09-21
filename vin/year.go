package vin

import (
	"fmt"
	"strings"
)

// yearCodes is the model-year cycle for position 10 (49 CFR 565.15(d)(1),
// Table VII): the letters A-Y without I, O, Q, U and Z, then the digits 1-9.
// It has 30 entries, so every code comes round again after 30 years.
const yearCodes = "ABCDEFGHJKLMNPRSTVWXY123456789"

// ModelYears returns every model year a position-10 code can stand for, one
// per 30-year cycle, oldest first: 'A' gives [1980 2010] and '3' gives
// [2003 2033]. The cycles are the two defined by regulation, 1980-2009 and
// 2010-2039.
//
// It returns nil for characters that are not year codes: '0', 'U' and 'Z'
// (legal VIN characters that position 10 never uses) and anything else that
// is not a VIN character. VINs from outside North America often put '0'
// there.
func ModelYears(code byte) []int {
	i := strings.IndexByte(yearCodes, upper(code))
	if i < 0 {
		return nil
	}
	return []int{1980 + i, 2010 + i}
}

// ModelYear is the decoded model year of a VIN.
//
// The code in position 10 repeats every 30 years, so on its own it names two
// years, and Candidates always lists both. Year holds the answer when
// something in the VIN chooses between them, and Certain says whether the
// regulation guarantees that choice or merely favours it. Basis explains
// either way.
type ModelYear struct {
	Code       string `json:"code"`           // the character in position 10
	Candidates []int  `json:"candidates"`     // every year Code can mean, oldest first
	Year       int    `json:"year,omitempty"` // the chosen year, or 0 if nothing chooses
	Certain    bool   `json:"certain"`        // the regulation settles it, rather than favouring it
	Basis      string `json:"basis"`          // how Year was chosen, or why it could not be
}

// Ambiguous reports whether the year could not be narrowed to one candidate.
func (y ModelYear) Ambiguous() bool { return y.Year == 0 && len(y.Candidates) > 1 }

// String renders the year for display: "2003", "2017 (likely)", "2001 or
// 2031", or "unknown".
func (y ModelYear) String() string {
	switch {
	case y.Year != 0 && y.Certain:
		return fmt.Sprint(y.Year)
	case y.Year != 0:
		return fmt.Sprint(y.Year, " (likely)")
	case len(y.Candidates) == 0:
		return "unknown"
	default:
		parts := make([]string, len(y.Candidates))
		for i, c := range y.Candidates {
			parts[i] = fmt.Sprint(c)
		}
		return strings.Join(parts, " or ")
	}
}

// Vehicle types, as NHTSA registers them, for which 49 CFR 565.15 ties
// position 7 to the 30-year cycle. The regulation covers passenger cars, and
// MPVs and trucks up to 10,000 lb GVWR. A WMI registered as "Truck" can cover
// heavier trucks too, and the VIN alone does not say which, so trucks are
// left ambiguous rather than guessed.
var positionSevenTypes = map[string]string{
	"Passenger Car":                        "a passenger car",
	"Multipurpose Passenger Vehicle (MPV)": "a multipurpose passenger vehicle",
}

// decodeYear decodes position 10 of s, using position 7 to choose the cycle
// when vehicleType is one the regulation covers. vehicleType is NHTSA's type
// for the VIN's WMI, or "" if the WMI is not registered.
//
// The position-7 rule is only conclusive in one direction. Since model year
// 2010 a car, MPV or light truck must carry a letter in position 7, so a
// digit there rules the newer cycle out. A letter does not rule the older
// cycle in the same way: before the 2008 rulemaking manufacturers could put
// a letter in position 7, and a few did — a 2001 Chrysler PT Cruiser
// (3C4FY4BB71T283350) carries a B there. So a digit gives a certain year and
// a letter gives a likely one.
func decodeYear(s string, vehicleType string) ModelYear {
	code := s[9]
	y := ModelYear{Code: string(code), Candidates: ModelYears(code)}
	if y.Candidates == nil {
		y.Candidates = []int{}
		y.Basis = fmt.Sprintf("position 10 is %q, which is not a model-year code; VINs built for markets outside North America often leave it unused", code)
		return y
	}

	what, covered := positionSevenTypes[vehicleType]
	switch {
	case vehicleType == "":
		y.Basis = "the WMI is not in NHTSA's registry, so the vehicle type is unknown, and position 7 settles the year only for cars, MPVs and light trucks"
	case vehicleType == "Truck":
		y.Basis = fmt.Sprintf("NHTSA registers this WMI for trucks, and position 7 settles the year only up to 10,000 lb GVWR; if this is a light truck it is %d", y.Candidates[cycleFromPosition7(s[6])])
	case !covered:
		y.Basis = fmt.Sprintf("NHTSA registers this WMI as %s, and position 7 settles the year only for cars, MPVs and light trucks", vehicleType)
	case isDigit(s[6]):
		y.Year, y.Certain = y.Candidates[0], true
		y.Basis = fmt.Sprintf("position 7 is a digit, and since model year 2010 %s must carry a letter there, so this is the 1980-2009 cycle (49 CFR 565.15)", what)
	default:
		y.Year = y.Candidates[1]
		y.Basis = fmt.Sprintf("position 7 is a letter, which for %s means 2010-2039 (49 CFR 565.15); a few vehicles built before that rule also used a letter there, so %d is not impossible", what, y.Candidates[0])
	}
	return y
}

// cycleFromPosition7 returns 0 (1980-2009) for a digit and 1 (2010-2039)
// for a letter.
func cycleFromPosition7(c byte) int {
	if isDigit(c) {
		return 0
	}
	return 1
}

func isDigit(c byte) bool { return '0' <= c && c <= '9' }
