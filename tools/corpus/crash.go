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
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ElatDev/vinspect/internal/fetch"
)

// NHTSA's research-database API. Each crash test lists its vehicles, and the
// per-vehicle detail record carries the VIN of the car that was crashed.
const nrd = "https://nrd.api.nhtsa.dot.gov/nhtsa/vehicle/api/v1/vehicle-database-test-results"

type crashVehicle struct {
	TestNo    int    `json:"testNo"`
	VehicleNo int    `json:"vehicleNo"`
	Make      string `json:"make"`
	Model     string `json:"model"`
	ModelYear int    `json:"modelYear"`
	VIN       string `json:"vin"`
}

func runCrash(args []string) error {
	fs := flag.NewFlagSet("crash", flag.ExitOnError)
	out := fs.String("o", "testdata/corpus/crash-test-vehicles.csv", "output CSV")
	cache := fs.String("cache", ".cache/nrd", "response cache directory")
	workers := fs.Int("workers", 4, "concurrent requests")
	interval := fs.Duration("interval", 200*time.Millisecond, "minimum gap between requests")
	offline := fs.Bool("offline", false, "use cached responses only, skipping uncached tests")
	fs.Parse(args)

	ctx := context.Background()
	c := fetch.New(*interval)

	tests, err := listTests(ctx, c, *cache)
	if err != nil {
		return err
	}
	log.Printf("%d crash tests", len(tests))
	if *offline {
		tests = slices.DeleteFunc(tests, func(t int) bool {
			_, err := os.Stat(filepath.Join(*cache, "tests", strconv.Itoa(t)+".json"))
			return err != nil
		})
		log.Printf("-offline: using the %d cached tests", len(tests))
	}

	jobs := make(chan int)
	var (
		mu       sync.Mutex
		vehicles []crashVehicle
		firstErr error
		done     int
		wg       sync.WaitGroup
	)
	start := time.Now()
	for range *workers {
		wg.Go(func() {
			for t := range jobs {
				vs, err := testVehicles(ctx, c, *cache, t)
				mu.Lock()
				if err != nil && firstErr == nil {
					firstErr = err
				}
				vehicles = append(vehicles, vs...)
				if done++; done%500 == 0 {
					log.Printf("%d/%d tests (%s)", done, len(tests), time.Since(start).Round(time.Second))
				}
				mu.Unlock()
			}
		})
	}
	for _, t := range tests {
		jobs <- t
	}
	close(jobs)
	wg.Wait()
	if firstErr != nil {
		return firstErr
	}

	slices.SortFunc(vehicles, func(a, b crashVehicle) int {
		return cmp.Or(cmp.Compare(a.TestNo, b.TestNo), cmp.Compare(a.VehicleNo, b.VehicleNo))
	})
	if err := writeCrashCSV(*out, vehicles); err != nil {
		return err
	}
	log.Printf("wrote %d vehicles to %s", len(vehicles), *out)
	return nil
}

// listTests returns every test number, paging through the listing endpoint.
func listTests(ctx context.Context, c *fetch.Client, cache string) ([]int, error) {
	path := filepath.Join(cache, "tests.json")
	var tests []int
	if readCache(path, &tests) {
		return tests, nil
	}
	const pageSize = 1000
	for page := 0; ; page++ {
		var resp struct {
			Meta struct {
				Pagination struct{ Total int }
			}
			Results []struct {
				TestNo int `json:"testNo"`
			}
		}
		u := fmt.Sprintf("%s?count=%d&pageNumber=%d", nrd, pageSize, page)
		if err := c.GetJSON(ctx, u, &resp); err != nil {
			return nil, err
		}
		for _, r := range resp.Results {
			tests = append(tests, r.TestNo)
		}
		total := resp.Meta.Pagination.Total
		if len(resp.Results) == 0 || len(tests) >= total {
			if len(tests) != total {
				return nil, fmt.Errorf("listed %d tests, API reports %d", len(tests), total)
			}
			break
		}
	}
	slices.Sort(tests)
	tests = slices.Compact(tests)
	return tests, writeCache(path, tests)
}

// testVehicles returns the vehicles in one test, with their VINs.
func testVehicles(ctx context.Context, c *fetch.Client, cache string, test int) ([]crashVehicle, error) {
	path := filepath.Join(cache, "tests", strconv.Itoa(test)+".json")
	var vs []crashVehicle
	if readCache(path, &vs) {
		return vs, nil
	}

	var info struct {
		Results []struct {
			VehicleNo    int    `json:"vehicleNo"`
			VehicleMake  string `json:"vehicleMake"`
			VehicleModel string `json:"vehicleModel"`
			ModelYear    *int   `json:"modelYear"`
		}
	}
	if err := c.GetJSON(ctx, fmt.Sprintf("%s/get-vehicle-info/%d", nrd, test), &info); err != nil {
		return nil, err
	}
	vs = []crashVehicle{} // cache "no vehicles" as [] rather than null
	for _, r := range info.Results {
		var detail struct {
			Results []struct {
				VIN string `json:"vehicleIdentificationNo"`
			}
		}
		u := fmt.Sprintf("%s/get-vehicle-detail-info/%d/%d", nrd, r.VehicleNo, test)
		if err := c.GetJSON(ctx, u, &detail); err != nil {
			return nil, err
		}
		v := crashVehicle{
			TestNo:    test,
			VehicleNo: r.VehicleNo,
			Make:      strings.TrimSpace(r.VehicleMake),
			Model:     strings.TrimSpace(r.VehicleModel),
		}
		if r.ModelYear != nil {
			v.ModelYear = *r.ModelYear
		}
		if len(detail.Results) > 0 {
			v.VIN = strings.TrimSpace(detail.Results[0].VIN)
		}
		vs = append(vs, v)
	}
	return vs, writeCache(path, vs)
}

func writeCrashCSV(path string, vs []crashVehicle) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	fmt.Fprintf(f, "# NHTSA Vehicle Crash Test Database (nrd.api.nhtsa.dot.gov), retrieved %s.\n", time.Now().UTC().Format("2006-01-02"))
	fmt.Fprintf(f, "# One row per test vehicle. vin is exactly as NHTSA recorded it, including typos.\n")
	fmt.Fprintf(f, "# Regenerate with: go run ./tools/corpus crash\n")
	w := csv.NewWriter(f)
	w.Write([]string{"test_no", "vehicle_no", "make", "model", "model_year", "vin"})
	for _, v := range vs {
		year := ""
		if v.ModelYear != 0 {
			year = strconv.Itoa(v.ModelYear)
		}
		w.Write([]string{strconv.Itoa(v.TestNo), strconv.Itoa(v.VehicleNo), v.Make, v.Model, year, v.VIN})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func readCache(path string, v any) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			log.Printf("ignoring unreadable cache %s: %v", path, err)
		}
		return false
	}
	return json.Unmarshal(b, v) == nil
}

func writeCache(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
