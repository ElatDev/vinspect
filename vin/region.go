package vin

import "strings"

// Regions by the first character of the WMI, from the ISO 3780 WMI country
// code chart (revised 2021-04-13). Rows D, F, G and 0 are unassigned.
var regions = map[byte]string{
	'A': "Africa", 'B': "Africa", 'C': "Africa",
	'E': "Europe", // the whole row is assigned to Russia
	'H': "Asia",   // the whole row is assigned to China
	'J': "Asia", 'K': "Asia", 'L': "Asia", 'M': "Asia", 'N': "Asia", 'P': "Asia", 'R': "Asia",
	'S': "Europe", 'T': "Europe", 'U': "Europe", 'V': "Europe", 'W': "Europe", 'X': "Europe", 'Y': "Europe", 'Z': "Europe",
	'1': "North America", '2': "North America", '3': "North America", '4': "North America", '5': "North America", '7': "North America",
	'6': "Oceania",
	'8': "South America", '9': "South America",
}

// secondOrder is the order ISO 3780 uses for the second character when it
// writes ranges: letters first, then 1-9, then 0. "AZ-A1" means AZ and A1.
const secondOrder = "ABCDEFGHJKLMNPRSTUVWXYZ1234567890"

// countryRanges transcribes the ISO 3780 WMI country code chart (revised
// 2021-04-13), cell by cell. Codes not listed are unassigned or held in
// reserve for their region.
var countryRanges = []string{
	"AA-AH South Africa", "AJ-AK Ivory Coast", "AL-AM Lesotho", "AN-AP Botswana",
	"AR-AS Namibia", "AT-AU Madagascar", "AV-AW Mauritius", "AX-AY Tunisia",
	"AZ-A1 Cyprus", "A2-A3 Zimbabwe", "A4-A5 Mozambique",
	"BA-BB Angola", "BC-BC Ethiopia", "BF-BG Kenya", "BH-BH Rwanda", "BL-BL Nigeria",
	"BR-BR Algeria", "BT-BT Eswatini", "BU-BU Uganda", "B3-B4 Libya",
	"CA-CB Egypt", "CF-CG Morocco", "CL-CM Zambia",
	"EA-E0 Russia",
	"HA-H0 China",
	"JA-J0 Japan",
	"KF-KH Israel", "KL-KR South Korea", "KS-KT Jordan", "K1-K3 South Korea", "K5-K5 Kyrgyzstan",
	"LA-L0 China",
	"MA-ME India", "MF-MK Indonesia", "ML-MR Thailand", "MS-MS Myanmar", "MU-MU Mongolia",
	"MX-MX Kazakhstan", "MY-M0 India",
	"NA-NE Iran", "NF-NG Pakistan", "NJ-NJ Iraq", "NL-NR Turkey", "NS-NT Uzbekistan",
	"NV-NV Azerbaijan", "NX-NX Tajikistan", "NY-NY Armenia", "N1-N5 Iran", "N7-N8 Turkey",
	"PA-PC Philippines", "PF-PG Singapore", "PL-PR Malaysia", "PS-PT Bangladesh", "P5-P0 India",
	"RA-RB United Arab Emirates", "RF-RK Taiwan", "RL-RN Vietnam", "RP-RP Laos",
	"RS-RT Saudi Arabia", "RU-RW Russia", "R1-R7 Hong Kong",
	"SA-SM United Kingdom", "SN-ST Germany", "SU-SZ Poland", "S1-S2 Latvia", "S3-S3 Georgia", "S4-S4 Iceland",
	"TA-TH Switzerland", "TJ-TP Czech Republic", "TR-TV Hungary", "TW-T2 Portugal", "T3-T5 Serbia",
	"T6-T6 Andorra", "T7-T8 Netherlands",
	"UA-UC Spain", "UH-UM Denmark", "UN-UR Ireland", "UU-UX Romania", "U1-U2 North Macedonia",
	"U5-U7 Slovakia", "U8-U0 Bosnia and Herzegovina",
	"VA-VE Austria", "VF-VR France", "VS-VW Spain", "VX-V2 France", "V3-V5 Croatia", "V6-V8 Estonia",
	"WA-W0 Germany",
	"XA-XC Bulgaria", "XD-XE Russia", "XF-XH Greece", "XJ-XK Russia", "XL-XR Netherlands",
	"XS-XW Russia", "XX-XY Luxembourg", "XZ-X0 Russia",
	"YA-YE Belgium", "YF-YK Finland", "YN-YN Malta", "YS-YW Sweden", "YX-Y2 Norway",
	"Y3-Y5 Belarus", "Y6-Y8 Ukraine",
	"ZA-ZU Italy", "ZX-ZZ Slovenia", "Z1-Z1 San Marino", "Z3-Z5 Lithuania", "Z6-Z0 Russia",
	"1A-10 United States",
	"2A-25 Canada",
	"3A-3X Mexico", "34-34 Nicaragua", "35-35 Dominican Republic", "36-36 Honduras",
	"37-37 Panama", "38-39 Puerto Rico",
	"4A-40 United States",
	"5A-50 United States",
	"6A-6X Australia", "6Y-61 New Zealand",
	"7A-70 United States",
	"8A-8E Argentina", "8F-8G Chile", "8L-8N Ecuador", "8S-8T Peru", "8X-8Z Venezuela",
	"82-82 Bolivia", "84-84 Costa Rica",
	"9A-9E Brazil", "9F-9G Colombia", "9S-9V Uruguay", "91-90 Brazil",
}

// countries maps every assigned two-character prefix to its country.
var countries = func() map[string]string {
	m := make(map[string]string, 1100)
	for _, entry := range countryRanges {
		span, country, _ := strings.Cut(entry, " ")
		first, from, to := span[0], strings.IndexByte(secondOrder, span[1]), strings.IndexByte(secondOrder, span[4])
		if len(span) != 5 || span[2] != '-' || span[3] != first || from < 0 || to < from {
			panic("vin: malformed country range " + entry)
		}
		for i := from; i <= to; i++ {
			m[string([]byte{first, secondOrder[i]})] = country
		}
	}
	return m
}()

// Origin returns the region and country that ISO 3780 assigns to the first
// two characters of a VIN or WMI. Either is "" when the code is unassigned.
//
// This is where the manufacturer registered the identifier, which is usually
// but not always where the vehicle was built: European manufacturers often
// use one WMI for every plant on the continent.
func Origin(s string) (region, country string) {
	if len(s) < 2 {
		return "", ""
	}
	a, b := upper(s[0]), upper(s[1])
	return regions[a], countries[string([]byte{a, b})]
}
