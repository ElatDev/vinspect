// Package vin validates and decodes 17-character Vehicle Identification
// Numbers (ISO 3779; 49 CFR 565 in the United States) without touching the
// network.
//
// A VIN is laid out as:
//
//	positions  1-3   WMI   world manufacturer identifier
//	positions  4-8   VDS   vehicle descriptor, manufacturer-specific
//	position   9           check digit
//	position   10          model year
//	position   11          assembly plant
//	positions 12-17        serial number
//
// Validate answers "could this be a real VIN?": length, alphabet and check
// digit. Decode answers "what does it say?": manufacturer, region, model
// year and plant. Both accept lower-case letters and treat them as upper case.
//
// The package is read-only by design. It checks and explains VINs; it has
// no way to construct one.
package vin

import (
	"strings"
	"unicode/utf8"
)

// Length is the number of characters in a VIN.
const Length = 17

// checkDigitPos is the 1-based position of the check digit.
const checkDigitPos = 9

// weights are the per-position multipliers from 49 CFR 565.15(c), Table IV.
// Position 9 holds the check digit itself, so it is weighted 0.
var weights = [Length]int{8, 7, 6, 5, 4, 3, 2, 10, 0, 9, 8, 7, 6, 5, 4, 3, 2}

// values maps each byte to its transliterated value (49 CFR 565.15(c),
// Table III), or -1 for bytes that cannot appear in a VIN. The letters run
// 1-9 in three rows, A-H, J-R and S-Z, the layout inherited from EBCDIC; I,
// O and Q are left out because they read as 1 and 0.
var values = func() (t [256]int8) {
	for i := range t {
		t[i] = -1
	}
	for c := byte('0'); c <= '9'; c++ {
		t[c] = int8(c - '0')
	}
	// Blanks hold the places of the excluded letters (and of the 1 that S
	// skips), so each letter's value is its column number.
	for _, row := range []string{"ABCDEFGH", "JKLMN P R", " STUVWXYZ"} {
		for j := range len(row) {
			if c := row[j]; c != ' ' {
				t[c] = int8(j + 1)
				t[c+('a'-'A')] = int8(j + 1)
			}
		}
	}
	return t
}()

// Normalize trims surrounding white space and upper-cases s. It is how the
// command-line tool cleans up pasted or piped input before validating it.
func Normalize(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

// Validate reports whether s is a well-formed VIN with a correct check digit.
// It returns nil for a valid VIN, or a *LengthError, *CharError or
// *CheckDigitError describing the first problem found, in that order.
//
// Position 9 must hold a digit or an X; anything else makes the VIN
// malformed, and Validate reports a *CharError for it.
//
// The check digit itself is mandatory for every vehicle sold in North America
// and China, and optional elsewhere, so a European-market VIN can fail here
// and still be genuine. Decode reports the check digit without failing on it.
func Validate(s string) error {
	if err := validateSyntax(s); err != nil {
		return err
	}
	have, want := upper(s[8]), checkDigit(s)
	// A letter other than X can never be a check digit, so such a VIN is
	// malformed rather than merely mismatched. NHTSA's own decoder treats it
	// the same way.
	if !isCheckDigitChar(have) {
		return &CharError{Pos: checkDigitPos, Char: rune(have)}
	}
	if have != want {
		return &CheckDigitError{Have: have, Want: want}
	}
	return nil
}

// isCheckDigitChar reports whether c can appear in position 9: a digit, or X
// for a remainder of 10 (49 CFR 565.15(c), Table V).
func isCheckDigitChar(c byte) bool { return isDigit(c) || c == 'X' }

// CheckDigit returns the check digit ('0'-'9' or 'X') that position 9 of s
// should hold, computed from the other sixteen characters. The character
// currently in position 9 is ignored. It fails only if s is not 17 valid VIN
// characters.
func CheckDigit(s string) (byte, error) {
	if err := validateSyntax(s); err != nil {
		return 0, err
	}
	return checkDigit(s), nil
}

// validateSyntax checks length and alphabet. On success s is exactly 17
// bytes, all of them VIN characters.
func validateSyntax(s string) error {
	if n := utf8.RuneCountInString(s); n != Length {
		return &LengthError{Len: n}
	}
	pos := 0
	for _, r := range s {
		pos++
		if r >= utf8.RuneSelf || values[r] < 0 {
			return &CharError{Pos: pos, Char: r}
		}
	}
	return nil
}

// checkDigit implements 49 CFR 565.15(c): transliterate, weight, sum, take
// the remainder mod 11, and write a remainder of 10 as X. s must already
// have passed validateSyntax.
func checkDigit(s string) byte {
	sum := 0
	for i := range Length {
		sum += int(values[s[i]]) * weights[i]
	}
	if r := sum % 11; r < 10 {
		return byte('0' + r)
	}
	return 'X'
}

func upper(c byte) byte {
	if 'a' <= c && c <= 'z' {
		return c - ('a' - 'A')
	}
	return c
}
