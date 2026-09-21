package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/ElatDev/vinspect/vin"
)

// checkRecord is the --json form of one check result.
type checkRecord struct {
	Input    string `json:"input"`
	Valid    bool   `json:"valid"`
	Error    string `json:"error,omitempty"` // "length", "character" or "check_digit"
	Message  string `json:"message,omitempty"`
	Length   int    `json:"length,omitempty"`
	Position int    `json:"position,omitempty"`
	Char     string `json:"char,omitempty"`
	Have     string `json:"have,omitempty"`
	Want     string `json:"want,omitempty"`
}

func newCheckRecord(input string, err error) checkRecord {
	rec := checkRecord{Input: input, Valid: err == nil}
	if err == nil {
		return rec
	}
	rec.Message = strings.TrimPrefix(err.Error(), "vin: ")
	var (
		le  *vin.LengthError
		ce  *vin.CharError
		cde *vin.CheckDigitError
	)
	switch {
	case errors.As(err, &le):
		rec.Error, rec.Length = "length", le.Len
	case errors.As(err, &ce):
		rec.Error, rec.Position, rec.Char = "character", ce.Pos, string(ce.Char)
	case errors.As(err, &cde):
		rec.Error, rec.Have, rec.Want = "check_digit", string(cde.Have), string(cde.Want)
		rec.Message += checkDigitNote(input)
	}
	return rec
}

// checkDigitNote explains, for a VIN from outside the regions that require
// a check digit, that a mismatch does not prove the VIN is fake.
func checkDigitNote(s string) string {
	region, country := vin.Origin(s)
	if region == "North America" || country == "China" {
		return ""
	}
	return " (a check digit is mandatory only for vehicles sold in North America and China)"
}

func runCheck(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	var cf commonFlags
	fs := newFlagSet("check", stderr, &cf)
	rest, err := parseArgs(fs, args)
	if err != nil {
		return flagError(err)
	}
	st, err := newStyle(cf.color, stdout)
	if err != nil {
		fmt.Fprintln(stderr, "vinspect:", err)
		return 2
	}

	enc := json.NewEncoder(stdout)
	valid, invalid := 0, 0
	err = readInputs(rest, stdin, func(input string) {
		s := vin.Normalize(input)
		rec := newCheckRecord(s, vin.Validate(s))
		if rec.Valid {
			valid++
		} else {
			invalid++
		}
		if cf.json {
			enc.Encode(rec)
			return
		}
		if valid+invalid == 1 {
			fmt.Fprintln(stdout, st.dim(pad("VIN", 19)+pad("RESULT", 9)+"DETAIL"))
		}
		result := st.green(pad("valid", 7))
		if !rec.Valid {
			result = st.red(pad("invalid", 7))
		}
		line := pad(display(s, 17), 17) + "  " + result + "  " + rec.Message
		fmt.Fprintln(stdout, strings.TrimRight(line, " "))
	})
	if err != nil {
		fmt.Fprintln(stderr, "vinspect: reading input:", err)
		return 2
	}
	switch total := valid + invalid; {
	case total == 0:
		fmt.Fprintln(stderr, "vinspect: no VINs given")
		return 2
	case total > 1 && !cf.json:
		fmt.Fprintf(stderr, "%d checked: %d valid, %d invalid\n", total, valid, invalid)
	}
	if invalid > 0 {
		return 1
	}
	return 0
}
