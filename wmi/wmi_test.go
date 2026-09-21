package wmi

import (
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestLookup(t *testing.T) {
	tests := []struct {
		code, manufacturer, vehicleType string
	}{
		{"1HG", "AMERICAN HONDA MOTOR CO., INC.", "Passenger Car"},
		{"1hg", "AMERICAN HONDA MOTOR CO., INC.", "Passenger Car"},
		{"1G1", "GENERAL MOTORS LLC", "Passenger Car"},
		{"1FT", "FORD MOTOR COMPANY", "Truck"},
		{"5YJ", "TESLA, INC.", "Passenger Car"},
		{"JM1", "MAZDA MOTOR CORPORATION", "Passenger Car"},
		{"1A9841", "AC PROPULSION, INC.", "Passenger Car"},
	}
	for _, tt := range tests {
		e, ok := Lookup(tt.code)
		if !ok || e.Manufacturer != tt.manufacturer || e.VehicleType != tt.vehicleType {
			t.Errorf("Lookup(%q) = %+v, %v; want %s / %s", tt.code, e, ok, tt.manufacturer, tt.vehicleType)
		}
	}
	for _, code := range []string{"LRW", "ZZZ", "", "1HGX", "1H", "1A9ZZZ"} {
		if e, ok := Lookup(code); ok {
			t.Errorf("Lookup(%q) = %+v, want no entry", code, e)
		}
	}
}

func TestCode(t *testing.T) {
	tests := map[string]string{
		"1HGCM82633A004352": "1HG",
		"1A91234591A841000": "1A9841",
		"1HGCM82633A00435":  "",
	}
	for vin, want := range tests {
		if got := Code(vin); got != want {
			t.Errorf("Code(%q) = %q, want %q", vin, got, want)
		}
	}
	if e, ok := ForVIN("1HGCM82633A004352"); !ok || e.Code != "1HG" {
		t.Errorf("ForVIN = %+v, %v", e, ok)
	}
	if _, ok := ForVIN("too short"); ok {
		t.Error("ForVIN accepted a short VIN")
	}
}

// TestSnapshot checks the embedded table as a whole: size, sort order (which
// Lookup's binary search depends on), the shape of every code, and that each
// six-character code has the 9 in position 3 that 49 CFR 565.15(a) requires.
func TestSnapshot(t *testing.T) {
	if n := Len(); n < 10000 {
		t.Fatalf("snapshot has %d WMIs; expected the full NHTSA table (13,000+)", n)
	}
	if !regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`).MatchString(Retrieved()) {
		t.Errorf("Retrieved() = %q, want YYYY-MM-DD", Retrieved())
	}

	valid := regexp.MustCompile(`^[A-HJ-NPR-Z0-9]{3}([A-HJ-NPR-Z0-9]{3})?$`)
	types := map[string]int{}
	seen := 0
	prev := ""
	for e := range All() {
		seen++
		if e.Code <= prev {
			t.Fatalf("snapshot is not sorted: %q follows %q, which breaks Lookup", e.Code, prev)
		}
		prev = e.Code
		if !valid.MatchString(e.Code) {
			t.Errorf("malformed WMI %q", e.Code)
		}
		if len(e.Code) == 6 && e.Code[2] != '9' {
			t.Errorf("six-character WMI %q lacks a 9 in position 3", e.Code)
		}
		if strings.TrimSpace(e.Manufacturer) == "" || e.VehicleType == "" || e.ManufacturerID <= 0 {
			t.Errorf("incomplete entry %+v", e)
		}
		types[e.VehicleType]++
	}
	if seen != Len() {
		t.Errorf("All() yielded %d entries, Len() reports %d", seen, Len())
	}
	t.Logf("%d WMIs by vehicle type: %v", Len(), types)

	// The first and last rows must be reachable through the binary search.
	for _, code := range []string{"101", prev} {
		if _, ok := Lookup(code); !ok {
			t.Errorf("Lookup(%q) missed a row that All() yielded", code)
		}
	}
}

func TestAllStopsEarly(t *testing.T) {
	var got []string
	for e := range All() {
		got = append(got, e.Code)
		if len(got) == 3 {
			break
		}
	}
	if len(got) != 3 || !slices.IsSorted(got) {
		t.Errorf("All() yielded %v", got)
	}
}

// TestFind exercises the binary search on a small sorted table, including
// the boundaries, where an off-by-one would hide in the real snapshot.
func TestFind(t *testing.T) {
	tab := table{rows: strings.Join([]string{
		"1A9841,1,AC,,Passenger Car,",
		"1HG,2,HONDA,,Passenger Car,",
		"5YJ,3,TESLA,,Passenger Car,",
		"WBA,4,BMW,,Passenger Car,",
		"ZZ9ZZZ,5,LAST,,Trailer,",
	}, "\n")}
	for _, code := range []string{"1A9841", "1HG", "5YJ", "WBA", "ZZ9ZZZ"} {
		line, ok := tab.find(code)
		if !ok || !strings.HasPrefix(line, code+",") {
			t.Errorf("find(%q) = %q, %v", code, line, ok)
		}
	}
	for _, code := range []string{"0AA", "1HGX", "1H", "JM1", "ZZZZZZ", ""} {
		if line, ok := tab.find(code); ok {
			t.Errorf("find(%q) = %q, want no match", code, line)
		}
	}
	if _, ok := (table{}).find("1HG"); ok {
		t.Error("find on an empty table returned a match")
	}
}

func TestParseRow(t *testing.T) {
	e, err := parseRow(`1HG,988,"AMERICAN HONDA MOTOR CO., INC.",HONDA,Passenger Car,UNITED STATES (USA)`)
	if err != nil {
		t.Fatal(err)
	}
	want := Entry{Code: "1HG", ManufacturerID: 988, Manufacturer: "AMERICAN HONDA MOTOR CO., INC.", Make: "HONDA", VehicleType: "Passenger Car", Country: "UNITED STATES (USA)"}
	if e != want {
		t.Errorf("parseRow = %+v, want %+v", e, want)
	}
	for name, row := range map[string]string{
		"too few fields": "1HG,988,HONDA",
		"bad id":         "1HG,x,HONDA,,Passenger Car,",
		"unclosed quote": `1HG,988,"HONDA,,Passenger Car,`,
	} {
		if _, err := parseRow(row); err == nil {
			t.Errorf("%s: parseRow succeeded", name)
		}
	}
}

func BenchmarkLookup(b *testing.B) {
	for b.Loop() {
		if _, ok := Lookup("1HG"); !ok {
			b.Fatal("missing")
		}
	}
}

// BenchmarkAll is the cost of walking the whole registry, which is what a
// single lookup used to pay before Lookup learned to binary-search.
func BenchmarkAll(b *testing.B) {
	b.SetBytes(int64(len(snapshot)))
	for b.Loop() {
		for range All() {
		}
	}
}
