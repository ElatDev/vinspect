package vin

import (
	"errors"
	"strings"
	"testing"
)

// Expected check digits in these tables were computed by a separate
// implementation, not by this package.

func TestValidate(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want error // nil, or the sentinel the error must match
	}{
		{"honda accord", "1HGCM82633A004352", nil},
		{"check digit X", "1M8GDM9AXKP042788", nil},
		{"straight ones", "11111111111111111", nil},
		{"49 CFR 565.15 Table VI sample", "1G4AH59H45G118341", nil},
		{"mazda3 from the NHTSA crash-test corpus", "JM1BN1U70H1100922", nil},
		{"lower case", "1hgcm82633a004352", nil},
		{"mixed case", "1HgCm82633a004352", nil},

		{"empty", "", ErrLength},
		{"16 characters", "1HGCM82633A00435", ErrLength},
		{"18 characters", "1HGCM82633A0043521", ErrLength},
		{"leading space", " 1HGCM82633A004352", ErrLength},
		{"trailing newline", "1HGCM82633A004352\n", ErrLength},
		{"hyphenated", "1HGCM8-2633A004352", ErrLength},

		{"letter I", "IHGCM82633A004352", ErrCharacter},
		{"letter O", "1HGCM82633A0O4352", ErrCharacter},
		{"letter Q", "1HGCM82633Q004352", ErrCharacter},
		{"lower-case o", "1HGCM82633A0o4352", ErrCharacter},
		{"punctuation", "1HGCM82633A00435-", ErrCharacter},
		{"inner space", "1HGCM826 3A004352", ErrCharacter},
		{"non-ASCII", "1HGCM82633A00435é", ErrCharacter},
		// Z is a legal VIN character but cannot be a check digit, so this
		// European-market Porsche is malformed rather than mismatched.
		{"letter in the check digit", "WP0ZZZ99ZTS392124", ErrCharacter},
		{"fullwidth digit", "1HGCM82633A00435２", ErrCharacter},

		{"wrong check digit", "1HGCM82643A004352", ErrCheckDigit},
		{"last digit changed", "1HGCM82633A004353", ErrCheckDigit},
		{"last two swapped", "1HGCM82633A004325", ErrCheckDigit},
		{"European VIN whose check digit does not compute", "WBA3A5C5XDF358910", ErrCheckDigit},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.in)
			if tt.want == nil {
				if err != nil {
					t.Fatalf("Validate(%q) = %v, want nil", tt.in, err)
				}
				return
			}
			if !errors.Is(err, tt.want) {
				t.Fatalf("Validate(%q) = %v, want an error matching %v", tt.in, err, tt.want)
			}
			// Each error matches exactly one sentinel.
			for _, other := range []error{ErrLength, ErrCharacter, ErrCheckDigit} {
				if other != tt.want && errors.Is(err, other) {
					t.Errorf("Validate(%q) = %v also matches %v", tt.in, err, other)
				}
			}
		})
	}
}

func TestValidateErrorDetails(t *testing.T) {
	t.Run("length counts characters, not bytes", func(t *testing.T) {
		var le *LengthError
		if err := Validate("1HGCM82633A00435éé"); !errors.As(err, &le) || le.Len != 18 {
			t.Fatalf("got %v, want *LengthError{Len: 18}", err)
		}
	})
	t.Run("first bad character wins", func(t *testing.T) {
		var ce *CharError
		err := Validate("1HGCM82633AO0435Q")
		if !errors.As(err, &ce) || ce.Pos != 12 || ce.Char != 'O' {
			t.Fatalf("got %v, want *CharError{Pos: 12, Char: 'O'}", err)
		}
		if !strings.Contains(err.Error(), "read as 1 and 0") {
			t.Errorf("message %q does not explain why O is excluded", err)
		}
	})
	t.Run("non-ASCII position", func(t *testing.T) {
		var ce *CharError
		if err := Validate("1HGCM82633A00435é"); !errors.As(err, &ce) || ce.Pos != 17 || ce.Char != 'é' {
			t.Fatalf("got %v, want *CharError{Pos: 17, Char: 'é'}", err)
		}
	})
	t.Run("check digit have and want", func(t *testing.T) {
		var cde *CheckDigitError
		if err := Validate("1HGCM82643A004352"); !errors.As(err, &cde) || cde.Have != '4' || cde.Want != '3' {
			t.Fatalf("got %v, want *CheckDigitError{Have: '4', Want: '3'}", err)
		}
	})
	t.Run("check digit reported upper case", func(t *testing.T) {
		var cde *CheckDigitError
		if err := Validate("1m8gdm9a3kp042788"); !errors.As(err, &cde) || cde.Have != '3' || cde.Want != 'X' {
			t.Fatalf("got %v, want *CheckDigitError{Have: '3', Want: 'X'}", err)
		}
		if err := Validate("1m8gdm9axkp042788"); err != nil {
			t.Fatalf("lower-case x check digit: %v", err)
		}
	})
}

func TestCheckDigit(t *testing.T) {
	tests := []struct {
		in   string
		want byte
	}{
		{"1HGCM82633A004352", '3'},
		{"1M8GDM9AXKP042788", 'X'},
		{"1M8GDM9A_KP042788", 0}, // '_' is not a VIN character
		{"1M8GDM9A0KP042788", 'X'},
		{"11111111111111111", '1'},
		{"1G4AH59H05G118341", '4'}, // position 9 is ignored
		{"WP0ZZZ99ZTS392124", '8'},
		{"KLATF08Y1VB363636", '4'},
		{"5YJ3E1EA7KF317000", '2'},
		{"WBA3A5C5XDF358910", '9'},
		{"JH4KA7561PC008269", '1'},
	}
	for _, tt := range tests {
		got, err := CheckDigit(tt.in)
		if tt.want == 0 {
			if err == nil {
				t.Errorf("CheckDigit(%q) = %q, want an error", tt.in, got)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("CheckDigit(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
		}
	}
}

// TestTransliteration pins every letter's value against 49 CFR 565.15(c),
// Table III, and checks that I, O and Q have none.
func TestTransliteration(t *testing.T) {
	table := "A1 B2 C3 D4 E5 F6 G7 H8 J1 K2 L3 M4 N5 P7 R9 S2 T3 U4 V5 W6 X7 Y8 Z9"
	for pair := range strings.FieldsSeq(table) {
		c, want := pair[0], int8(pair[1]-'0')
		if values[c] != want || values[c+32] != want {
			t.Errorf("value of %c = %d (lower %d), want %d", c, values[c], values[c+32], want)
		}
	}
	for c := byte('0'); c <= '9'; c++ {
		if values[c] != int8(c-'0') {
			t.Errorf("value of %c = %d", c, values[c])
		}
	}
	for _, c := range []byte("IOQioq") {
		if values[c] != -1 {
			t.Errorf("%c has value %d, want none", c, values[c])
		}
	}
	allowed := 0
	for _, v := range values {
		if v >= 0 {
			allowed++
		}
	}
	if allowed != 10+2*23 {
		t.Errorf("%d byte values allowed, want 56 (10 digits, 23 letters in two cases)", allowed)
	}
}

// TestSubstitutionDetection checks the property that makes the check digit
// worth having. The weights of every position except 9 are non-zero and 11
// is prime, so changing one character changes the sum mod 11 unless the new
// character transliterates to the same value (A and J are both 1). Every
// such single-character error must therefore be caught.
func TestSubstitutionDetection(t *testing.T) {
	const alphabet = "0123456789ABCDEFGHJKLMNPRSTUVWXYZ"
	for _, good := range []string{"1HGCM82633A004352", "1M8GDM9AXKP042788", "JM1BN1U70H1100922"} {
		caught, blind := 0, 0
		for pos := range Length {
			if pos == 8 {
				continue // the check digit itself; changing it is caught trivially
			}
			for i := range len(alphabet) {
				c := alphabet[i]
				if c == good[pos] {
					continue
				}
				typo := good[:pos] + string(c) + good[pos+1:]
				err := Validate(typo)
				sameValue := values[c] == values[good[pos]]
				switch {
				case sameValue && err == nil:
					blind++
				case !sameValue && errors.Is(err, ErrCheckDigit):
					caught++
				default:
					t.Errorf("%s -> %s (position %d): Validate = %v", good, typo, pos+1, err)
				}
			}
		}
		t.Logf("%s: %d single-character typos caught, %d undetectable by design", good, caught, blind)
	}
}

func TestNormalize(t *testing.T) {
	if got := Normalize("  1hgcm82633a004352\r\n"); got != "1HGCM82633A004352" {
		t.Errorf("Normalize = %q", got)
	}
}

func FuzzValidate(f *testing.F) {
	for _, s := range []string{"1HGCM82633A004352", "1M8GDM9AXKP042788", "WP0ZZZ99ZTS392124", "", "IOQ", "1HGCM82633A00435é"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		err := Validate(s)
		info, derr := Decode(s)
		want, cerr := CheckDigit(s)

		// Decode and CheckDigit fail on exactly the same inputs: anything
		// that is not 17 VIN characters. Validate is stricter, so it must
		// fail on those too.
		if (derr != nil) != (cerr != nil) {
			t.Fatalf("%q: Decode=%v CheckDigit=%v", s, derr, cerr)
		}
		if derr != nil {
			if err == nil {
				t.Fatalf("%q: Decode failed with %v but Validate accepted it", s, derr)
			}
			return
		}
		// Validate accepts exactly the VINs whose check digit matches.
		if info.CheckDigit.Valid != (err == nil) || info.CheckDigit.Want != string(want) {
			t.Fatalf("%q: Validate=%v but Decode check digit = %+v", s, err, info.CheckDigit)
		}
		// Writing the computed digit into position 9 always yields a valid VIN.
		fixed := s[:8] + string(want) + s[9:]
		if err := Validate(fixed); err != nil {
			t.Fatalf("%q with computed check digit %q: %v", s, want, err)
		}
	})
}
