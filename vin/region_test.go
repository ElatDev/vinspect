package vin

import (
	"strings"
	"testing"
)

func TestOrigin(t *testing.T) {
	tests := []struct {
		in, region, country string
	}{
		{"1HG", "North America", "United States"},
		{"4T1", "North America", "United States"},
		{"5YJ", "North America", "United States"},
		{"7SA", "North America", "United States"},
		{"2T1", "North America", "Canada"},
		{"25X", "North America", "Canada"},
		{"26X", "North America", ""}, // reserved for the region, unassigned
		{"3VW", "North America", "Mexico"},
		{"3X1", "North America", "Mexico"},
		{"3Y1", "North America", ""},
		{"38A", "North America", "Puerto Rico"},
		{"JM1", "Asia", "Japan"},
		{"KMH", "Asia", "South Korea"},
		{"K1A", "Asia", "South Korea"},
		{"LRW", "Asia", "China"},
		{"MA1", "Asia", "India"},
		{"MY1", "Asia", "India"},
		{"M01", "Asia", "India"},
		{"RU1", "Asia", "Russia"},
		{"WBA", "Europe", "Germany"},
		{"WP0", "Europe", "Germany"},
		{"SAL", "Europe", "United Kingdom"},
		{"SN1", "Europe", "Germany"},
		{"VF1", "Europe", "France"},
		{"VS1", "Europe", "Spain"},
		{"VX1", "Europe", "France"},
		{"V21", "Europe", "France"},
		{"YV1", "Europe", "Sweden"},
		{"ZFF", "Europe", "Italy"},
		{"ZU1", "Europe", "Italy"},
		{"ZV1", "Europe", ""},
		{"XTA", "Europe", "Russia"},
		{"E01", "Europe", "Russia"},
		{"6FP", "Oceania", "Australia"},
		{"6X1", "Oceania", "Australia"},
		{"6Y1", "Oceania", "New Zealand"},
		{"611", "Oceania", "New Zealand"},
		{"9BW", "South America", "Brazil"},
		{"93H", "South America", "Brazil"},
		{"8AP", "South America", "Argentina"},
		{"AAV", "Africa", "South Africa"},
		{"AZ1", "Africa", "Cyprus"},
		{"A11", "Africa", "Cyprus"},
		{"A21", "Africa", "Zimbabwe"},
		{"D11", "", ""},
		{"011", "", ""},
		{"jm1", "Asia", "Japan"},
		{"J", "", ""},
	}
	for _, tt := range tests {
		region, country := Origin(tt.in)
		if region != tt.region || country != tt.country {
			t.Errorf("Origin(%q) = %q, %q; want %q, %q", tt.in, region, country, tt.region, tt.country)
		}
	}
}

// TestCountryRangesDisjoint checks that no two-character code is assigned
// twice, which a transcription slip in countryRanges would cause, and that
// every assigned code sits in a row that has a region.
func TestCountryRangesDisjoint(t *testing.T) {
	owner := map[string]string{}
	for _, entry := range countryRanges {
		span, country, _ := strings.Cut(entry, " ")
		from := strings.IndexByte(secondOrder, span[1])
		to := strings.IndexByte(secondOrder, span[4])
		for i := from; i <= to; i++ {
			code := string([]byte{span[0], secondOrder[i]})
			if prev, ok := owner[code]; ok {
				t.Errorf("%s assigned to both %s and %s", code, prev, country)
			}
			owner[code] = country
		}
		if regions[span[0]] == "" {
			t.Errorf("%s: row %c has no region", entry, span[0])
		}
	}
	if len(owner) != len(countries) {
		t.Errorf("countries has %d codes, ranges cover %d", len(countries), len(owner))
	}
}
