package vin

import (
	"errors"
	"fmt"
	"strings"
)

// Sentinel errors, for use with errors.Is. Every error returned by this
// package matches exactly one of them; errors.As recovers the details.
var (
	// ErrLength means the input is not 17 characters long.
	ErrLength = errors.New("vin: wrong length")
	// ErrCharacter means the input contains a character VINs never use.
	ErrCharacter = errors.New("vin: invalid character")
	// ErrCheckDigit means position 9 does not match the computed check digit.
	ErrCheckDigit = errors.New("vin: check digit mismatch")
)

// LengthError reports input that is not 17 characters long.
type LengthError struct {
	Len int // length of the input, in characters
}

func (e *LengthError) Error() string {
	return fmt.Sprintf("vin: %d characters, want %d", e.Len, Length)
}

func (e *LengthError) Is(target error) bool { return target == ErrLength }

// CharError reports the first character that cannot appear in a VIN.
type CharError struct {
	Pos  int  // 1-based position in the VIN
	Char rune // the offending character
}

func (e *CharError) Error() string {
	switch {
	case strings.ContainsRune("IOQioq", e.Char):
		return fmt.Sprintf("vin: position %d is %q; I, O and Q are never used because they read as 1 and 0", e.Pos, e.Char)
	case e.Pos == checkDigitPos:
		// Reached only for characters the VIN alphabet allows elsewhere.
		return fmt.Sprintf("vin: position %d is %q; the check digit must be a digit or X", e.Pos, e.Char)
	}
	return fmt.Sprintf("vin: position %d is %q, which is not a VIN character (A-Z except I, O, Q, and 0-9)", e.Pos, e.Char)
}

func (e *CharError) Is(target error) bool { return target == ErrCharacter }

// CheckDigitError reports a VIN whose position 9 disagrees with the check
// digit computed from the other sixteen characters.
type CheckDigitError struct {
	Have byte // the character in position 9
	Want byte // the computed check digit, '0'-'9' or 'X'
}

func (e *CheckDigitError) Error() string {
	return fmt.Sprintf("vin: check digit is %q, computed %q", e.Have, e.Want)
}

func (e *CheckDigitError) Is(target error) bool { return target == ErrCheckDigit }
