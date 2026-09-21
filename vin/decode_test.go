package vin

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"
)

func TestDecode(t *testing.T) {
	tests := []struct {
		vin          string
		manufacturer string // "" when the WMI is not registered
		region       string
		country      string
		checkValid   bool
		year         int
		candidates   []int
		plant        string
		serial       string
	}{
		{"1HGCM82633A004352", "AMERICAN HONDA MOTOR CO., INC.", "North America", "United States", true, 2003, []int{2003, 2033}, "A", "004352"},
		{"JM1BN1U70H1100922", "MAZDA MOTOR CORPORATION", "Asia", "Japan", true, 2017, []int{1987, 2017}, "1", "100922"},
		// A European-market Porsche 911: no check digit, but it still decodes,
		// and position 7 dates it to 1996.
		{"WP0ZZZ99ZTS392124", "DR. ING. H.C.F. PORSCHE AG", "Europe", "Germany", false, 1996, []int{1996, 2026}, "S", "392124"},
		// Chinese-market Model 3: Tesla's Shanghai WMI is not registered with
		// NHTSA, so the year stays ambiguous.
		{"LRW3E7EA6MC000000", "", "Asia", "China", true, 0, []int{1991, 2021}, "C", "000000"},
		{"1hgcm82633a004352", "AMERICAN HONDA MOTOR CO., INC.", "North America", "United States", true, 2003, []int{2003, 2033}, "A", "004352"},
	}
	for _, tt := range tests {
		t.Run(tt.vin, func(t *testing.T) {
			info, err := Decode(tt.vin)
			if err != nil {
				t.Fatal(err)
			}
			if info.VIN != Normalize(tt.vin) {
				t.Errorf("VIN %q", info.VIN)
			}
			if info.WMI != info.VIN[:3] || info.VDS != info.VIN[3:8] || info.VIS != info.VIN[9:] {
				t.Errorf("sections %q %q %q", info.WMI, info.VDS, info.VIS)
			}
			gotMfr := ""
			if info.Manufacturer != nil {
				gotMfr = info.Manufacturer.Manufacturer
			}
			if gotMfr != tt.manufacturer {
				t.Errorf("manufacturer %q, want %q", gotMfr, tt.manufacturer)
			}
			if info.Region != tt.region || info.Country != tt.country {
				t.Errorf("origin %q %q, want %q %q", info.Region, info.Country, tt.region, tt.country)
			}
			if info.CheckDigit.Valid != tt.checkValid || info.CheckDigit.Have != info.VIN[8:9] {
				t.Errorf("check digit %+v, want valid=%v", info.CheckDigit, tt.checkValid)
			}
			if info.ModelYear.Year != tt.year || !slices.Equal(info.ModelYear.Candidates, tt.candidates) {
				t.Errorf("model year %+v, want %d %v", info.ModelYear, tt.year, tt.candidates)
			}
			if info.PlantCode != tt.plant || info.Serial != tt.serial {
				t.Errorf("plant %q serial %q, want %q %q", info.PlantCode, info.Serial, tt.plant, tt.serial)
			}
		})
	}
}

func TestDecodeRejectsBadSyntax(t *testing.T) {
	for in, want := range map[string]error{
		"1HGCM82633A00435":  ErrLength,
		"1HGCM82633A00435O": ErrCharacter,
	} {
		if _, err := Decode(in); !errors.Is(err, want) {
			t.Errorf("Decode(%q) error = %v, want %v", in, err, want)
		}
	}
}

// TestDecodeLowVolume checks that a '9' in position 3 makes Decode look up
// the six-character identifier formed with positions 12-14.
func TestDecodeLowVolume(t *testing.T) {
	// 1A9 + 841 is AC Propulsion's six-character WMI.
	info, err := Decode("1A91234591A841000")
	if err != nil {
		t.Fatal(err)
	}
	if info.Manufacturer == nil || info.Manufacturer.Code != "1A9841" {
		t.Fatalf("manufacturer %+v, want WMI 1A9841", info.Manufacturer)
	}
}

func TestInfoJSON(t *testing.T) {
	info, err := Decode("1HGCM82633A004352")
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	var back map[string]any
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"vin", "wmi", "vds", "vis", "check_digit", "region", "country", "manufacturer", "model_year", "plant_code", "serial"} {
		if _, ok := back[key]; !ok {
			t.Errorf("JSON is missing %q: %s", key, b)
		}
	}
}
