// Package corpus loads the committed verification corpus in testdata/corpus
// and measures how vinspect's offline answers compare with NHTSA's.
//
// The corpus has two halves. crash-test-vehicles.csv lists every vehicle in
// NHTSA's crash-test database with the VIN NHTSA recorded for it.
// vpic-decodes.csv holds NHTSA vPIC's decode of each distinct 17-character VIN
// from that list. Both are produced by tools/corpus and committed, so the
// comparison is reproducible offline.
package corpus

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Vehicle is one row of crash-test-vehicles.csv.
type Vehicle struct {
	TestNo    int
	VehicleNo int
	Make      string
	Model     string
	ModelYear int // 0 when NHTSA recorded none
	VIN       string
}

// Decode is one row of vpic-decodes.csv: what NHTSA vPIC said about a VIN.
type Decode struct {
	VIN            string
	ErrorCodes     []string // vPIC's comma-separated ErrorCode, split
	Manufacturer   string
	ManufacturerID int
	Make           string
	Model          string
	ModelYear      int // 0 when vPIC could not decode one
	VehicleType    string
	PlantCountry   string
}

// HasError reports whether vPIC raised the given error code.
func (d Decode) HasError(code string) bool { return slices.Contains(d.ErrorCodes, code) }

// Clean reports whether vPIC decoded the VIN with no complaint at all, which
// is error code 0 on its own.
func (d Decode) Clean() bool { return slices.Equal(d.ErrorCodes, []string{"0"}) }

// LoadVehicles reads crash-test-vehicles.csv.
func LoadVehicles(path string) ([]Vehicle, error) {
	var out []Vehicle
	err := readCSV(path, 6, func(rec []string) error {
		var v Vehicle
		var err error
		if v.TestNo, err = strconv.Atoi(rec[0]); err != nil {
			return err
		}
		if v.VehicleNo, err = strconv.Atoi(rec[1]); err != nil {
			return err
		}
		v.Make, v.Model = rec[2], rec[3]
		if rec[4] != "" {
			if v.ModelYear, err = strconv.Atoi(rec[4]); err != nil {
				return err
			}
		}
		v.VIN = rec[5]
		out = append(out, v)
		return nil
	})
	return out, err
}

// LoadDecodes reads vpic-decodes.csv, keyed by VIN.
func LoadDecodes(path string) (map[string]Decode, error) {
	out := map[string]Decode{}
	err := readCSV(path, 9, func(rec []string) error {
		d := Decode{
			VIN:          rec[0],
			Manufacturer: rec[2],
			Make:         rec[4],
			Model:        rec[5],
			VehicleType:  rec[7],
			PlantCountry: rec[8],
		}
		for code := range strings.SplitSeq(rec[1], ",") {
			if code = strings.TrimSpace(code); code != "" {
				d.ErrorCodes = append(d.ErrorCodes, code)
			}
		}
		d.ManufacturerID, _ = strconv.Atoi(rec[3])
		d.ModelYear, _ = strconv.Atoi(rec[6])
		out[d.VIN] = d
		return nil
	})
	return out, err
}

// CandidateVINs returns the distinct VINs in vs that are 17 characters long
// once surrounding space is trimmed and letters are upper-cased, sorted.
// These are the VINs the corpus claims are measured over. Length is the only
// filter: VINs with illegal characters or bad check digits stay in, because
// rejecting them correctly is half of what is being measured.
func CandidateVINs(vs []Vehicle) []string {
	var out []string
	for _, v := range vs {
		if s := Normalize(v.VIN); utf8.RuneCountInString(s) == 17 {
			out = append(out, s)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// Normalize trims surrounding space and upper-cases s.
func Normalize(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }

func readCSV(path string, fields int, row func([]string) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Comment = '#'
	r.FieldsPerRecord = fields
	if _, err := r.Read(); err != nil { // header
		return fmt.Errorf("%s: %w", path, err)
	}
	for {
		rec, err := r.Read()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if err := row(rec); err != nil {
			line, _ := r.FieldPos(0)
			return fmt.Errorf("%s:%d: %w", path, line, err)
		}
	}
}

// Load reads both corpus files, compares vinspect with NHTSA over them, and
// returns the report along with the date the crash-test snapshot was taken.
func Load(crashPath, decodePath string) (Report, string, error) {
	vehicles, err := LoadVehicles(crashPath)
	if err != nil {
		return Report{}, "", err
	}
	decodes, err := LoadDecodes(decodePath)
	if err != nil {
		return Report{}, "", err
	}
	rep, err := Compare(vehicles, decodes)
	return rep, Retrieved(crashPath), err
}

// Retrieved reads the snapshot date out of a corpus file's header comment.
func Retrieved(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if !strings.HasPrefix(sc.Text(), "#") {
			break
		}
		if _, rest, ok := strings.Cut(sc.Text(), "retrieved "); ok {
			date, _, _ := strings.Cut(rest, ".")
			return date
		}
	}
	return ""
}
