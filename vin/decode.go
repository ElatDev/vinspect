package vin

import (
	"strings"

	"github.com/ElatDev/vinspect/wmi"
)

// Info is everything a VIN says about itself, decoded offline.
type Info struct {
	VIN string `json:"vin"` // the VIN, upper-cased

	WMI string `json:"wmi"` // positions 1-3, world manufacturer identifier
	VDS string `json:"vds"` // positions 4-8, vehicle descriptor
	VIS string `json:"vis"` // positions 10-17, vehicle identifier

	CheckDigit CheckDigitResult `json:"check_digit"`

	Region  string `json:"region,omitempty"`  // ISO 3780 region of the WMI
	Country string `json:"country,omitempty"` // ISO 3780 country of the WMI

	// Manufacturer is NHTSA's registry entry for the WMI, or nil if the WMI
	// is not registered with NHTSA.
	Manufacturer *wmi.Entry `json:"manufacturer"`

	ModelYear ModelYear `json:"model_year"`

	PlantCode string `json:"plant_code"` // position 11; the meaning is manufacturer-specific
	Serial    string `json:"serial"`     // positions 12-17
}

// CheckDigitResult compares position 9 with the computed check digit.
type CheckDigitResult struct {
	Have  string `json:"have"`  // the character in position 9
	Want  string `json:"want"`  // the computed check digit
	Valid bool   `json:"valid"` // Have == Want
}

// Decode splits a VIN into its sections and decodes what can be decoded
// without the network: the manufacturer (from the embedded NHTSA registry),
// the ISO 3780 region and country, and the model year.
//
// It returns an error only when s is not 17 valid VIN characters. A wrong
// check digit is reported in Info.CheckDigit instead, because the check digit
// is optional outside North America and China and those VINs still decode.
func Decode(s string) (Info, error) {
	if err := validateSyntax(s); err != nil {
		return Info{}, err
	}
	s = strings.ToUpper(s)
	want := checkDigit(s)

	info := Info{
		VIN: s,
		WMI: s[0:3],
		VDS: s[3:8],
		VIS: s[9:17],
		CheckDigit: CheckDigitResult{
			Have:  s[8:9],
			Want:  string(want),
			Valid: s[8] == want,
		},
		PlantCode: s[10:11],
		Serial:    s[11:17],
	}
	info.Region, info.Country = Origin(s)

	var vehicleType string
	if e, ok := wmi.ForVIN(s); ok {
		info.Manufacturer = &e
		vehicleType = e.VehicleType
	}
	info.ModelYear = decodeYear(s, vehicleType)
	return info, nil
}
