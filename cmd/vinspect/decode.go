package main

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ElatDev/vinspect/vin"
	"github.com/ElatDev/vinspect/vpic"
	"github.com/ElatDev/vinspect/wmi"
)

// newVPICClient builds the client for --online. Tests replace it to point
// at a local server.
var newVPICClient = func() *vpic.Client {
	return &vpic.Client{UserAgent: "vinspect/" + versionString() + " (+https://github.com/ElatDev/vinspect)"}
}

// decoded is one input and everything learned about it.
type decoded struct {
	input string
	info  vin.Info
	err   error        // non-nil if the input is not 17 VIN characters
	vpic  *vpic.Result // set with --online
}

// decodeRecord is the --json form of one decode.
type decodeRecord struct {
	Input string `json:"input"`
	*vin.Info
	VPIC    *vpic.Result `json:"vpic,omitempty"`
	Error   string       `json:"error,omitempty"`
	Message string       `json:"message,omitempty"`
}

func runDecode(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	var cf commonFlags
	fs := newFlagSet("decode", stderr, &cf)
	online := fs.Bool("online", false, "also decode through NHTSA's vPIC API")
	timeout := fs.Duration("timeout", 30*time.Second, "time limit for --online requests")
	rest, err := parseArgs(fs, args)
	if err != nil {
		return flagError(err)
	}
	st, err := newStyle(cf.color, stdout)
	if err != nil {
		fmt.Fprintln(stderr, "vinspect:", err)
		return 2
	}

	var results []decoded
	if err := readInputs(rest, stdin, func(input string) {
		s := vin.Normalize(input)
		info, err := vin.Decode(s)
		results = append(results, decoded{input: s, info: info, err: err})
	}); err != nil {
		fmt.Fprintln(stderr, "vinspect: reading input:", err)
		return 2
	}
	if len(results) == 0 {
		fmt.Fprintln(stderr, "vinspect: no VINs given")
		return 2
	}

	status := 0
	for _, r := range results {
		if r.err != nil {
			status = 1
		}
	}
	if *online {
		ctx, cancel := context.WithTimeout(context.Background(), *timeout)
		defer cancel()
		if err := addOnline(ctx, results); err != nil {
			fmt.Fprintln(stderr, "vinspect: online decode failed, showing offline results only:", err)
			status = 1
		}
	}

	switch {
	case cf.json:
		enc := json.NewEncoder(stdout)
		for _, r := range results {
			rec := decodeRecord{Input: r.input, VPIC: r.vpic}
			if r.err != nil {
				c := newCheckRecord(r.input, r.err)
				rec.Error, rec.Message = c.Error, c.Message
			} else {
				rec.Info = &r.info
			}
			enc.Encode(rec)
		}
	case len(results) == 1 && results[0].err == nil:
		writeTree(stdout, st, results[0].info, results[0].vpic)
	default:
		writeTable(stdout, st, results, *online)
	}
	return status
}

// addOnline decodes every well-formed input through vPIC, one batch request
// per 50 distinct VINs.
func addOnline(ctx context.Context, results []decoded) error {
	var vins []string
	for _, r := range results {
		if r.err == nil && !slices.Contains(vins, r.info.VIN) {
			vins = append(vins, r.info.VIN)
		}
	}
	if len(vins) == 0 {
		return nil
	}
	c := newVPICClient()
	var got []vpic.Result
	if len(vins) == 1 {
		r, err := c.Decode(ctx, vins[0])
		if err != nil {
			return err
		}
		got = []vpic.Result{r}
	} else {
		var err error
		if got, err = c.DecodeBatch(ctx, vins); err != nil {
			return err
		}
	}
	byVIN := make(map[string]*vpic.Result, len(got))
	for i := range got {
		byVIN[vins[i]] = &got[i]
	}
	for i := range results {
		results[i].vpic = byVIN[results[i].info.VIN]
	}
	return nil
}

// writeTree prints one VIN as a labelled breakdown of its sections.
//
//	1HG CM826 3 3 A 004352
//	│   │     │ │ │ └────── serial        004352
//	│   │     │ │ └──────── plant         A
//	...
func writeTree(w io.Writer, st style, info vin.Info, online *vpic.Result) {
	type field struct{ label, value string }
	type section struct {
		text   string
		color  func(string) string
		fields []field
	}

	check := st.green(info.CheckDigit.Have) + " matches"
	if !info.CheckDigit.Valid {
		check = st.red(info.CheckDigit.Have) + ", should be " + st.bold(info.CheckDigit.Want)
		if !strings.Contains("0123456789X", info.CheckDigit.Have) {
			check = st.red(info.CheckDigit.Have) + st.dim(" cannot be a check digit") + ", should be " + st.bold(info.CheckDigit.Want)
		}
	}
	year := st.yellow(info.ModelYear.String())
	if info.ModelYear.Ambiguous() {
		year += st.dim(" (ambiguous)")
	}
	maker := []field{{"manufacturer", st.dim("not in NHTSA's registry")}}
	if m := info.Manufacturer; m != nil {
		maker = []field{{"manufacturer", st.bold(m.Manufacturer)}}
		if kind := joinNonEmpty(" · ", m.Make, m.VehicleType); kind != "" {
			maker = append(maker, field{"registered", kind})
		}
	}
	origin := joinNonEmpty(" · ", info.Country, info.Region)
	if origin == "" {
		origin = st.dim("unassigned ISO 3780 code")
	}
	maker = append(maker, field{"origin", origin})

	sections := []section{
		{info.WMI, st.cyan, maker},
		{info.VDS, st.blue, []field{{"descriptor", info.VDS + st.dim("  model, body and engine, coded by the manufacturer")}}},
		{info.CheckDigit.Have, st.green, []field{{"check digit", check}}},
		{info.ModelYear.Code, st.yellow, []field{{"model year", year}}},
		{info.PlantCode, st.magenta, []field{{"plant", info.PlantCode + st.dim("  assembly plant, coded by the manufacturer")}}},
		{info.Serial, func(s string) string { return s }, []field{{"serial", info.Serial}}},
	}
	if !info.CheckDigit.Valid {
		sections[2].color = st.red
	}

	// The top line: the VIN with a space between sections.
	var top []string
	starts := make([]int, len(sections))
	col := 0
	for i, s := range sections {
		starts[i] = col
		col += len(s.text) + 1
		top = append(top, s.color(s.text))
	}
	fmt.Fprintf(w, "\n  %s\n", strings.Join(top, " "))

	// One branch per section, last section first, so the lines nest.
	labelCol := col + 2
	for i := len(sections) - 1; i >= 0; i-- {
		for j, f := range sections[i].fields {
			line := []rune(strings.Repeat(" ", labelCol))
			for k := range i {
				line[starts[k]] = '│'
			}
			if j == 0 {
				line[starts[i]] = '└'
				for x := starts[i] + 1; x < labelCol-1; x++ {
					line[x] = '─'
				}
			}
			fmt.Fprintf(w, "  %s%s  %s\n", st.dim(string(line)), st.dim(pad(f.label, 12)), f.value)
		}
	}

	y := info.ModelYear
	switch {
	case y.Certain:
		note(w, st, "", fmt.Sprintf("Model year %d: %s.", y.Year, y.Basis))
	case y.Year != 0:
		note(w, st, "", fmt.Sprintf("Model year %d is likely: %s.", y.Year, y.Basis))
	case y.Ambiguous():
		note(w, st, "", fmt.Sprintf("Model year %s is ambiguous: %s.", y, y.Basis))
	default:
		note(w, st, "", fmt.Sprintf("Model year unknown: %s.", y.Basis))
	}
	if !info.CheckDigit.Valid {
		note(w, st, st.red("!"), fmt.Sprintf("The check digit does not match%s.", checkDigitNote(info.VIN)))
	}
	if info.Manufacturer == nil {
		note(w, st, st.dim("·"), fmt.Sprintf("%s is not among the %s WMIs in NHTSA's registry (snapshot %s).",
			info.WMI, thousands(wmi.Len()), wmi.Retrieved()))
	}
	if online != nil {
		writeOnline(w, st, online)
	}
	fmt.Fprintln(w)
}

// note prints a dimmed footnote under the tree, wrapped to fit an
// 80-column terminal, with marker (if any) before the first line.
func note(w io.Writer, st style, marker, text string) {
	const width = 74
	indent := "  "
	if marker != "" {
		indent = "    "
	} else {
		fmt.Fprintln(w)
	}
	line := ""
	first := true
	flush := func() {
		prefix := indent
		if first && marker != "" {
			prefix = "  " + marker + " "
		}
		fmt.Fprintln(w, prefix+st.dim(line))
		line, first = "", false
	}
	for word := range strings.FieldsSeq(text) {
		switch {
		case line == "":
			line = word
		case len(line)+1+len(word) > width:
			flush()
			line = word
		default:
			line += " " + word
		}
	}
	if line != "" {
		flush()
	}
}

// writeOnline prints the extra detail vPIC returned for one VIN.
func writeOnline(w io.Writer, st style, r *vpic.Result) {
	fmt.Fprintf(w, "\n  %s\n", st.bold("NHTSA vPIC"))
	year := ""
	if r.ModelYear != 0 {
		year = strconv.Itoa(r.ModelYear)
	}
	engine := joinNonEmpty(", ",
		joinNonEmpty(" ", liters(r.DisplacementL), cylinders(r.Cylinders)),
		suffix(r.EngineHP, " hp"),
		r.FuelType)
	rows := [][2]string{
		{"vehicle", joinNonEmpty(" ", year, r.Make, r.Model, r.Trim, r.Series)},
		{"body", joinNonEmpty(", ", r.BodyClass, suffix(r.Doors, " doors"), r.DriveType)},
		{"engine", engine},
		{"transmission", joinNonEmpty(" ", suffix(r.Gears, "-speed"), r.Transmission)},
		{"built in", joinNonEmpty(", ", r.PlantCity, r.PlantState, r.PlantCountry)},
		{"gvwr", r.GVWR},
		{"vPIC says", r.ErrorText},
	}
	for _, row := range rows {
		if row[1] != "" {
			fmt.Fprintf(w, "  %s  %s\n", st.dim(pad(row[0], 12)), row[1])
		}
	}
}

// writeTable prints one row per input.
func writeTable(w io.Writer, st style, results []decoded, online bool) {
	if online {
		fmt.Fprintln(w, st.dim(pad("VIN", 19)+pad("CHECK", 7)+pad("YEAR", 11)+pad("MAKE", 15)+pad("MODEL", 20)+"TRIM"))
	} else {
		fmt.Fprintln(w, st.dim(pad("VIN", 19)+pad("CHECK", 7)+pad("YEAR", 11)+pad("MAKE", 15)+pad("MANUFACTURER", 32)+"ORIGIN"))
	}
	for _, r := range results {
		name := pad(display(r.input, 17), 17)
		if r.err != nil {
			fmt.Fprintf(w, "%s  %s\n", name, st.red(strings.TrimPrefix(r.err.Error(), "vin: ")))
			continue
		}
		info := r.info
		check := st.green(pad("ok", 5))
		if !info.CheckDigit.Valid {
			check = st.red(pad("bad", 5))
		}
		// A trailing "?" marks a year position 7 favours but does not prove.
		year := strings.ReplaceAll(info.ModelYear.String(), " or ", "/")
		switch y := info.ModelYear; {
		case y.Certain:
			year = strconv.Itoa(y.Year)
		case y.Year != 0:
			year = strconv.Itoa(y.Year) + "?"
		}
		var mk, mfr string
		if m := info.Manufacturer; m != nil {
			mk, mfr = m.Make, m.Manufacturer
		}
		if online && r.vpic != nil {
			if r.vpic.ModelYear != 0 {
				year = strconv.Itoa(r.vpic.ModelYear)
			}
			mk = cmp.Or(r.vpic.Make, mk)
			fmt.Fprintf(w, "%s  %s  %s  %s  %s  %s\n", name, check, pad(year, 9),
				pad(display(mk, 13), 13), pad(display(r.vpic.Model, 18), 18), display(r.vpic.Trim, 24))
			continue
		}
		fmt.Fprintf(w, "%s  %s  %s  %s  %s  %s\n", name, check, pad(year, 9),
			pad(display(mk, 13), 13), pad(display(mfr, 30), 30), cmp.Or(info.Country, info.Region))
	}
}

func joinNonEmpty(sep string, parts ...string) string {
	return strings.Join(slices.DeleteFunc(parts, func(s string) bool { return s == "" }), sep)
}

func suffix(s, unit string) string {
	if s == "" {
		return ""
	}
	return s + unit
}

// liters formats vPIC's displacement ("2.998832712") as "3.0 L".
func liters(s string) string {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f <= 0 {
		return ""
	}
	return strconv.FormatFloat(f, 'f', 1, 64) + " L"
}

func cylinders(s string) string {
	if s == "" {
		return ""
	}
	return s + "-cylinder"
}

// thousands formats n with comma separators: 12999 -> "12,999".
func thousands(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
