// Command wmi-import regenerates the embedded WMI snapshot (wmi/wmi.csv) from
// NHTSA's public vPIC API.
//
//	go run ./tools/wmi-import
//
// vPIC has no "list every WMI" endpoint, but GetWMIsForManufacturer accepts a
// vehicle type instead of a manufacturer, and every WMI registered with NHTSA
// has a type. Querying each type from GetVehicleVariableValuesList therefore
// enumerates the whole table in nine requests.
//
// Makes are not part of that response, so the importer then calls DecodeWMI
// once per three-character WMI (about 3,400 requests) to record brand names:
// "CHEVROLET" for 1G1 rather than just "GENERAL MOTORS LLC". NHTSA's CDN
// blocks clients that go fast, so these run one at a time with a gap between
// them, and every response is cached under .cache/wmi so an interrupted run
// resumes instead of starting over. Six-character identifiers belong to
// low-volume manufacturers, mostly trailer builders; they are left without a
// make and decoders fall back to the manufacturer name.
package main

import (
	"cmp"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ElatDev/vinspect/internal/fetch"
)

const api = "https://vpic.nhtsa.dot.gov/api/vehicles/"

// validWMI matches identifiers that can occur in a VIN: three VIN characters,
// or six with a 9 in position 3 for low-volume manufacturers.
var validWMI = regexp.MustCompile(`^[A-HJ-NPR-Z0-9]{2}(?:[A-HJ-NPR-Z0-8]|9[A-HJ-NPR-Z0-9]{3})$`)

type row struct {
	WMI            string
	ManufacturerID int
	Manufacturer   string
	Make           string
	VehicleType    string
	Country        string
}

func main() {
	out := flag.String("o", "wmi/wmi.csv", "output CSV path")
	cache := flag.String("cache", ".cache/wmi", "response cache directory")
	interval := flag.Duration("interval", 1500*time.Millisecond, "minimum gap between requests")
	makes := flag.Bool("makes", true, "look up makes with DecodeWMI (slow)")
	flag.Parse()
	log.SetFlags(log.Ltime)

	ctx := context.Background()
	c := fetch.New(*interval)

	var types []string
	if err := cached(filepath.Join(*cache, "types.json"), &types, func() error {
		var err error
		types, err = vehicleTypes(ctx, c)
		return err
	}); err != nil {
		log.Fatal(err)
	}

	var rows []row
	for _, t := range types {
		var got []row
		if err := cached(filepath.Join(*cache, "type", url.PathEscape(t)+".json"), &got, func() error {
			var err error
			got, err = wmisForType(ctx, c, t)
			return err
		}); err != nil {
			log.Fatal(err)
		}
		log.Printf("%-40s %5d WMIs", t, len(got))
		rows = append(rows, got...)
	}

	if *makes {
		if err := addMakes(ctx, c, *cache, rows); err != nil {
			log.Fatal(err)
		}
	}

	slices.SortFunc(rows, func(a, b row) int {
		return cmp.Or(
			cmp.Compare(a.WMI, b.WMI),
			cmp.Compare(a.VehicleType, b.VehicleType),
			cmp.Compare(a.ManufacturerID, b.ManufacturerID),
		)
	})
	rows = slices.Compact(rows)

	// vPIC has a few typos, such as the letter O where a zero was meant.
	// Those identifiers can never match a valid VIN, so they are dropped,
	// and named in the output header so the omission is visible.
	var skipped []string
	rows = slices.DeleteFunc(rows, func(r row) bool {
		if validWMI.MatchString(r.WMI) {
			return false
		}
		skipped = append(skipped, r.WMI)
		return true
	})
	if len(skipped) > 0 {
		log.Printf("skipped %d WMIs that cannot appear in a VIN: %s", len(skipped), strings.Join(skipped, ", "))
	}

	if err := write(*out, rows, skipped); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %d rows to %s", len(rows), *out)
}

func vehicleTypes(ctx context.Context, c *fetch.Client) ([]string, error) {
	var resp struct {
		Results []struct{ Name string }
	}
	if err := c.GetJSON(ctx, api+"GetVehicleVariableValuesList/Vehicle%20Type?format=json", &resp); err != nil {
		return nil, err
	}
	var names []string
	for _, r := range resp.Results {
		names = append(names, r.Name)
	}
	if len(names) == 0 {
		return nil, errors.New("vPIC returned no vehicle types")
	}
	return names, nil
}

func wmisForType(ctx context.Context, c *fetch.Client, vehicleType string) ([]row, error) {
	var resp struct {
		Results []struct {
			WMI         string
			ID          int `json:"Id"`
			Name        string
			VehicleType string
			Country     *string
		}
	}
	u := api + "GetWMIsForManufacturer/?format=json&vehicleType=" + url.QueryEscape(vehicleType)
	if err := c.GetJSON(ctx, u, &resp); err != nil {
		return nil, err
	}
	rows := []row{}
	for _, r := range resp.Results {
		// The type filter is a substring match; keep only exact hits so a
		// type whose name contains another's is not counted twice.
		if r.VehicleType != vehicleType {
			continue
		}
		var country string
		if r.Country != nil {
			country = strings.TrimSpace(*r.Country)
		}
		rows = append(rows, row{
			WMI:            strings.ToUpper(strings.TrimSpace(r.WMI)),
			ManufacturerID: r.ID,
			Manufacturer:   strings.TrimSpace(r.Name),
			VehicleType:    r.VehicleType,
			Country:        country,
		})
	}
	return rows, nil
}

// wmiMake is one DecodeWMI result: a make a manufacturer sells under a WMI.
type wmiMake struct {
	Make             string
	ManufacturerName string
}

// addMakes fills Make for three-character WMIs via DecodeWMI.
func addMakes(ctx context.Context, c *fetch.Client, cache string, rows []row) error {
	var codes []string
	for _, r := range rows {
		if len(r.WMI) == 3 {
			codes = append(codes, r.WMI)
		}
	}
	slices.Sort(codes)
	codes = slices.Compact(codes)

	byWMI := make(map[string][]wmiMake, len(codes))
	start := time.Now()
	for i, w := range codes {
		var got []wmiMake
		if err := cached(filepath.Join(cache, "decodewmi", w+".json"), &got, func() error {
			var resp struct{ Results []wmiMake }
			err := c.GetJSON(ctx, api+"DecodeWMI/"+url.PathEscape(w)+"?format=json", &resp)
			got = resp.Results
			if got == nil {
				got = []wmiMake{}
			}
			return err
		}); err != nil {
			return err
		}
		byWMI[w] = got
		if (i+1)%250 == 0 {
			log.Printf("DecodeWMI %d/%d (%s)", i+1, len(codes), time.Since(start).Round(time.Second))
		}
	}

	for i, r := range rows {
		var makes []string
		for _, m := range byWMI[r.WMI] {
			mk := strings.TrimSpace(m.Make)
			if mk != "" && strings.EqualFold(strings.TrimSpace(m.ManufacturerName), r.Manufacturer) && !slices.Contains(makes, mk) {
				makes = append(makes, mk)
			}
		}
		slices.Sort(makes)
		rows[i].Make = strings.Join(makes, "/")
	}
	return nil
}

// cached loads v from path if it exists; otherwise it runs fill (which must
// populate v) and saves the result so the next run skips the request.
func cached(path string, v any, fill func() error) error {
	b, err := os.ReadFile(path)
	if err == nil && json.Unmarshal(b, v) == nil {
		return nil
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := fill(); err != nil {
		return err
	}
	if b, err = json.Marshal(v); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func write(path string, rows []row, skipped []string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	fmt.Fprintf(f, "# NHTSA vPIC World Manufacturer Identifier table, retrieved %s.\n", time.Now().UTC().Format("2006-01-02"))
	fmt.Fprintf(f, "# Regenerate with: go run ./tools/wmi-import\n")
	if len(skipped) > 0 {
		fmt.Fprintf(f, "# Left out because they cannot appear in a VIN: %s\n", strings.Join(skipped, ", "))
	}
	w := csv.NewWriter(f)
	w.Write([]string{"wmi", "manufacturer_id", "manufacturer", "make", "vehicle_type", "country"})
	for _, r := range rows {
		w.Write([]string{r.WMI, strconv.Itoa(r.ManufacturerID), r.Manufacturer, r.Make, r.VehicleType, r.Country})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
