package vin

import "testing"

const sample = "1HGCM82633A004352"

func BenchmarkValidate(b *testing.B) {
	for b.Loop() {
		if err := Validate(sample); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCheckDigit(b *testing.B) {
	for b.Loop() {
		if _, err := CheckDigit(sample); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecode(b *testing.B) {
	for b.Loop() {
		if _, err := Decode(sample); err != nil {
			b.Fatal(err)
		}
	}
}
