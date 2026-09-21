package corpus

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The comparison logic, exercised on a handful of rows rather than the whole
// committed corpus, so each branch is reachable in a readable test.
func TestCompare(t *testing.T) {
	vehicles := []Vehicle{
		{TestNo: 1, VehicleNo: 1, VIN: "1HGCM82633A004352"},   // valid, known WMI, resolvable year
		{TestNo: 2, VehicleNo: 1, VIN: " jm1bn1u70h1100922 "}, // needs normalising
		{TestNo: 3, VehicleNo: 1, VIN: "1HGCM82633A004353"},   // bad check digit
		{TestNo: 4, VehicleNo: 1, VIN: "1B7GEO6X5LS716048"},   // letter O: illegal character
		{TestNo: 5, VehicleNo: 1, VIN: "1FTFW1ET9DFC10312"},   // truck: year stays ambiguous
		{TestNo: 6, VehicleNo: 1, VIN: "LRW3E7EA6MC000000"},   // WMI not in NHTSA's registry
		{TestNo: 7, VehicleNo: 1, VIN: "5H27H182053"},         // pre-1981, too short to count
		{TestNo: 8, VehicleNo: 1, VIN: ""},                    // no VIN recorded
		{TestNo: 9, VehicleNo: 1, VIN: "1HGCM82633A004352"},   // duplicate of the first
	}
	decodes := map[string]Decode{
		"1HGCM82633A004352": {ErrorCodes: []string{"0"}, Manufacturer: "AMERICAN HONDA MOTOR CO., INC.", ModelYear: 2003},
		"JM1BN1U70H1100922": {ErrorCodes: []string{"0"}, Manufacturer: "MAZDA MOTOR CORPORATION", ModelYear: 2017},
		"1HGCM82633A004353": {ErrorCodes: []string{"1"}, Manufacturer: "AMERICAN HONDA MOTOR CO., INC.", ModelYear: 2003},
		"1B7GEO6X5LS716048": {ErrorCodes: []string{"400"}},
		"1FTFW1ET9DFC10312": {ErrorCodes: []string{"0"}, Manufacturer: "FORD MOTOR COMPANY", ModelYear: 2013},
		"LRW3E7EA6MC000000": {ErrorCodes: []string{"0", "7"}, Manufacturer: "", ModelYear: 2021},
	}

	rep, err := Compare(vehicles, decodes)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Vehicles != 9 || rep.WithVIN != 8 || rep.Candidates != 6 {
		t.Errorf("counts: %d vehicles, %d with a VIN, %d candidates", rep.Vehicles, rep.WithVIN, rep.Candidates)
	}
	if rep.Valid != 4 || rep.BadCheckDigit != 1 || rep.BadCharacter != 1 {
		t.Errorf("verdicts: %d valid, %d check digit, %d character", rep.Valid, rep.BadCheckDigit, rep.BadCharacter)
	}
	if rep.VerdictAgree != 6 || len(rep.VerdictDisagree) != 0 {
		t.Errorf("agreement: %d agree, %v", rep.VerdictAgree, rep.VerdictDisagree)
	}
	// The manufacturer and year measures cover only VINs both decoders
	// accept, which excludes the bad check digit and the illegal character.
	// (The LRW row carries vPIC code 7 alongside 0, so it is not clean.)
	if rep.Clean != 3 || rep.MfrCompared != 3 || rep.MfrAgree != 3 || len(rep.MfrDisagree) != 0 {
		t.Errorf("manufacturer: %d clean, %d compared, %d agree, %v", rep.Clean, rep.MfrCompared, rep.MfrAgree, rep.MfrDisagree)
	}
	// The clean Honda has a digit in position 7 (certain), the Mazda has a
	// letter (likely), and the truck stays ambiguous.
	if rep.YearCertain != 1 || rep.YearLikely != 1 || rep.YearAmbiguous != 1 || len(rep.YearDisagree) != 0 {
		t.Errorf("year: %d certain, %d likely, %d ambiguous, %v", rep.YearCertain, rep.YearLikely, rep.YearAmbiguous, rep.YearDisagree)
	}

	t.Run("mismatches are recorded", func(t *testing.T) {
		wrong := map[string]Decode{}
		for k, v := range decodes {
			wrong[k] = v
		}
		// A clean vPIC decode that names a different manufacturer and year.
		wrong["1HGCM82633A004352"] = Decode{ErrorCodes: []string{"0"}, Manufacturer: "SOME OTHER COMPANY", ModelYear: 1996}
		rep, err := Compare(vehicles, wrong)
		if err != nil {
			t.Fatal(err)
		}
		if len(rep.MfrDisagree) != 1 || len(rep.YearCertainWrong) != 1 {
			t.Fatalf("expected a manufacturer and a year mismatch, got %d and %d", len(rep.MfrDisagree), len(rep.YearCertainWrong))
		}
		if m := rep.YearCertainWrong[0]; m.Ours != "2003" || m.NHTSA != "1996" {
			t.Errorf("year mismatch recorded as %+v", m)
		}
	})

	t.Run("a missing vPIC decode is an error", func(t *testing.T) {
		if _, err := Compare(vehicles, map[string]Decode{}); err == nil {
			t.Error("want an error when the vPIC file is incomplete")
		}
	})

	t.Run("markdown reports the numbers", func(t *testing.T) {
		md := rep.Markdown("2026-09-20", "2026-09-20")
		for _, want := range []string{MarkerStart, MarkerEnd, "6 / 6 (100%)", "2026-09-20"} {
			if !strings.Contains(md, want) {
				t.Errorf("markdown is missing %q:\n%s", want, md)
			}
		}
	})
}

func TestLoadCSV(t *testing.T) {
	dir := t.TempDir()
	crash := filepath.Join(dir, "crash.csv")
	os.WriteFile(crash, []byte(`# NHTSA Vehicle Crash Test Database, retrieved 2026-09-20.
test_no,vehicle_no,make,model,model_year,vin
10,1,HONDA,ACCORD,2003,1HGCM82633A004352
11,2,FORD,,,
`), 0o644)
	vs, err := LoadVehicles(crash)
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 2 || vs[0].Make != "HONDA" || vs[0].ModelYear != 2003 || vs[1].ModelYear != 0 {
		t.Fatalf("parsed %+v", vs)
	}
	if got := Retrieved(crash); got != "2026-09-20" {
		t.Errorf("Retrieved = %q", got)
	}

	decodes := filepath.Join(dir, "vpic.csv")
	os.WriteFile(decodes, []byte(`# retrieved 2026-09-20.
vin,error_code,manufacturer,manufacturer_id,make,model,model_year,vehicle_type,plant_country
1HGCM82633A004352,"1,11",AMERICAN HONDA MOTOR CO.,988,HONDA,Accord,2003,PASSENGER CAR,UNITED STATES (USA)
`), 0o644)
	ds, err := LoadDecodes(decodes)
	if err != nil {
		t.Fatal(err)
	}
	d := ds["1HGCM82633A004352"]
	if !d.HasError("1") || !d.HasError("11") || d.HasError("0") || d.ManufacturerID != 988 || d.ModelYear != 2003 {
		t.Errorf("parsed %+v", d)
	}
	if v := NHTSAVerdict(d); v != BadCheckDigit {
		t.Errorf("verdict %q", v)
	}
}
