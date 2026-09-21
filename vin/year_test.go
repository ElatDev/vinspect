package vin

import (
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestModelYears(t *testing.T) {
	tests := []struct {
		code byte
		want []int
	}{
		{'A', []int{1980, 2010}},
		{'B', []int{1981, 2011}},
		{'H', []int{1987, 2017}},
		{'J', []int{1988, 2018}}, // I is skipped
		{'N', []int{1992, 2022}},
		{'P', []int{1993, 2023}}, // O is skipped
		{'R', []int{1994, 2024}}, // Q is skipped
		{'S', []int{1995, 2025}},
		{'T', []int{1996, 2026}},
		{'V', []int{1997, 2027}}, // U is skipped
		{'Y', []int{2000, 2030}}, // Z is skipped
		{'1', []int{2001, 2031}},
		{'9', []int{2009, 2039}},
		{'a', []int{1980, 2010}},
		{'0', nil},
		{'U', nil},
		{'Z', nil},
		{'I', nil},
		{'O', nil},
		{'Q', nil},
		{'-', nil},
	}
	for _, tt := range tests {
		if got := ModelYears(tt.code); !slices.Equal(got, tt.want) {
			t.Errorf("ModelYears(%q) = %v, want %v", tt.code, got, tt.want)
		}
	}
}

// TestModelYearTable checks the whole 2010-2039 cycle against 49 CFR
// 565.15(d)(1), Table VII, as printed in the regulation.
func TestModelYearTable(t *testing.T) {
	const tableVII = "2010A 2011B 2012C 2013D 2014E 2015F 2016G 2017H 2018J 2019K " +
		"2020L 2021M 2022N 2023P 2024R 2025S 2026T 2027V 2028W 2029X " +
		"2030Y 20311 20322 20333 20344 20355 20366 20377 20388 20399"
	seen := 0
	for entry := range strings.FieldsSeq(tableVII) {
		year, _ := strconv.Atoi(entry[:4])
		code := entry[4]
		if got, want := ModelYears(code), []int{year - 30, year}; !slices.Equal(got, want) {
			t.Errorf("ModelYears(%q) = %v, want %v", code, got, want)
		}
		seen++
	}
	if seen != 30 {
		t.Fatalf("table has %d entries, want 30", seen)
	}
}

func TestDecodeYear(t *testing.T) {
	const (
		car   = "Passenger Car"
		mpv   = "Multipurpose Passenger Vehicle (MPV)"
		truck = "Truck"
		moto  = "Motorcycle"
	)
	tests := []struct {
		name        string
		vin         string
		vehicleType string
		wantYear    int
		wantCertain bool
		wantCands   []int
		basis       string // substring of Basis
	}{
		// A digit in position 7 rules out the 2010-2039 cycle, because a car
		// or MPV of model year 2010 or later must carry a letter there.
		{"car, digit in 7", "1HGCM82633A004352", car, 2003, true, []int{2003, 2033}, "1980-2009"},
		// A letter only favours the newer cycle: older vehicles were allowed
		// letters too, so the answer is likely rather than certain.
		{"car, letter in 7", "JM1BN1U70H1100922", car, 2017, false, []int{1987, 2017}, "2010-2039"},
		{"mpv, letter in 7", "5FNYF6H59KB000000", mpv, 2019, false, []int{1989, 2019}, "multipurpose"},
		// A real 2001 PT Cruiser, which carries a letter in position 7.
		{"pre-2010 car with a letter in 7", "3C4FY4BB71T283350", mpv, 2031, false, []int{2001, 2031}, "2001 is not impossible"},
		{"truck stays ambiguous", "1FTFW1ET9DFC10312", truck, 0, false, []int{1983, 2013}, "if this is a light truck it is 2013"},
		{"truck, digit in 7", "1GCEK19T54E000000", truck, 0, false, []int{2004, 2034}, "if this is a light truck it is 2004"},
		{"motorcycle stays ambiguous", "1HD1KB4197Y000000", moto, 0, false, []int{2007, 2037}, "registers this WMI as Motorcycle"},
		{"unregistered WMI", "LRW3E7EA6MC000000", "", 0, false, []int{1991, 2021}, "not in NHTSA's registry"},
		{"no year code", "WP0ZZZ99Z0S392124", car, 0, false, []int{}, "not a model-year code"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			y := decodeYear(tt.vin, tt.vehicleType)
			if y.Year != tt.wantYear || y.Certain != tt.wantCertain || !slices.Equal(y.Candidates, tt.wantCands) {
				t.Errorf("year %d (certain %v) candidates %v, want %d (certain %v) %v",
					y.Year, y.Certain, y.Candidates, tt.wantYear, tt.wantCertain, tt.wantCands)
			}
			if !strings.Contains(y.Basis, tt.basis) {
				t.Errorf("basis %q does not mention %q", y.Basis, tt.basis)
			}
			if y.Code != tt.vin[9:10] {
				t.Errorf("code %q, want %q", y.Code, tt.vin[9:10])
			}
			if y.Ambiguous() != (tt.wantYear == 0 && len(tt.wantCands) > 1) {
				t.Errorf("Ambiguous() = %v", y.Ambiguous())
			}
		})
	}
}

func TestModelYearString(t *testing.T) {
	tests := []struct {
		y    ModelYear
		want string
	}{
		{ModelYear{Candidates: []int{2003, 2033}, Year: 2003, Certain: true}, "2003"},
		{ModelYear{Candidates: []int{1987, 2017}, Year: 2017}, "2017 (likely)"},
		{ModelYear{Candidates: []int{2001, 2031}}, "2001 or 2031"},
		{ModelYear{Candidates: []int{}}, "unknown"},
	}
	for _, tt := range tests {
		if got := tt.y.String(); got != tt.want {
			t.Errorf("%+v.String() = %q, want %q", tt.y, got, tt.want)
		}
	}
}
