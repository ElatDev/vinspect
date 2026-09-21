package vin_test

import (
	"errors"
	"fmt"

	"github.com/ElatDev/vinspect/vin"
)

func ExampleValidate() {
	for _, s := range []string{"1HGCM82633A004352", "1HGCM82633A004353", "1HGCM82633A0O4352"} {
		err := vin.Validate(s)
		var cde *vin.CheckDigitError
		switch {
		case err == nil:
			fmt.Println(s, "valid")
		case errors.As(err, &cde):
			fmt.Printf("%s check digit should be %c\n", s, cde.Want)
		default:
			fmt.Println(s, err)
		}
	}
	// Output:
	// 1HGCM82633A004352 valid
	// 1HGCM82633A004353 check digit should be 5
	// 1HGCM82633A0O4352 vin: position 13 is 'O'; I, O and Q are never used because they read as 1 and 0
}

func ExampleDecode() {
	info, err := vin.Decode("1HGCM82633A004352")
	if err != nil {
		panic(err)
	}
	fmt.Println(info.Manufacturer.Manufacturer)
	fmt.Println(info.Region, "/", info.Country)
	fmt.Println(info.ModelYear.Year, "-", info.ModelYear.Basis)
	fmt.Println("plant", info.PlantCode, "serial", info.Serial)
	// Output:
	// AMERICAN HONDA MOTOR CO., INC.
	// North America / United States
	// 2003 - position 7 is a digit, and since model year 2010 a passenger car must carry a letter there, so this is the 1980-2009 cycle (49 CFR 565.15)
	// plant A serial 004352
}

func ExampleModelYears() {
	fmt.Println(vin.ModelYears('1'))
	fmt.Println(vin.ModelYears('0'))
	// Output:
	// [2001 2031]
	// []
}

func ExampleCheckDigit() {
	d, _ := vin.CheckDigit("1M8GDM9A_KP042788"[:8] + "0" + "KP042788")
	fmt.Printf("%c\n", d)
	// Output: X
}
