package main

import (
	"bufio"
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
	"time"

	"github.com/ElatDev/vinspect/internal/corpus"
	"github.com/ElatDev/vinspect/internal/fetch"
	"github.com/ElatDev/vinspect/vpic"
)

// runVPIC decodes every corpus VIN through NHTSA's vPIC API, using the same
// client the CLI's --online flag uses, wrapped in a rate-limiting transport.
// Results are appended to a cache as they arrive, so an interrupted run
// resumes rather than re-asking.
func runVPIC(args []string) error {
	fs := flag.NewFlagSet("vpic", flag.ExitOnError)
	in := fs.String("in", "testdata/corpus/crash-test-vehicles.csv", "crash-test corpus CSV")
	out := fs.String("o", "testdata/corpus/vpic-decodes.csv", "output CSV")
	cachePath := fs.String("cache", ".cache/vpic/decodes.jsonl", "append-only response cache")
	interval := fs.Duration("interval", 3*time.Second, "minimum gap between requests")
	fs.Parse(args)

	vehicles, err := corpus.LoadVehicles(*in)
	if err != nil {
		return err
	}
	vins := corpus.CandidateVINs(vehicles)
	log.Printf("%d distinct 17-character VINs in %s", len(vins), *in)

	have, err := loadDecodeCache(*cachePath)
	if err != nil {
		return err
	}
	var todo []string
	for _, v := range vins {
		if _, ok := have[v]; !ok {
			todo = append(todo, v)
		}
	}
	log.Printf("%d cached, %d to fetch", len(vins)-len(todo), len(todo))

	if len(todo) > 0 {
		if err := os.MkdirAll(filepath.Dir(*cachePath), 0o755); err != nil {
			return err
		}
		cf, err := os.OpenFile(*cachePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		defer cf.Close()
		enc := json.NewEncoder(cf)

		client := &vpic.Client{HTTP: fetch.NewHTTPClient(*interval), UserAgent: fetch.UserAgent}
		start := time.Now()
		for batch := range slices.Chunk(todo, vpic.MaxBatch) {
			got, err := decodeChunk(context.Background(), client, batch)
			if err != nil {
				return err
			}
			for _, r := range got {
				if err := enc.Encode(r); err != nil {
					return err
				}
				have[r.VIN] = r
			}
			log.Printf("%d/%d decoded (%s)", len(have), len(vins), time.Since(start).Round(time.Second))
		}
	}

	return writeDecodeCSV(*out, vins, have)
}

func loadDecodeCache(path string) (map[string]vpic.Result, error) {
	have := map[string]vpic.Result{}
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return have, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var r vpic.Result
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			continue // a torn last line from an interrupted run
		}
		have[r.VIN] = r
	}
	return have, sc.Err()
}

func writeDecodeCSV(path string, vins []string, have map[string]vpic.Result) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	fmt.Fprintf(f, "# NHTSA vPIC decode of every VIN in crash-test-vehicles.csv, retrieved %s.\n", time.Now().UTC().Format("2006-01-02"))
	fmt.Fprintf(f, "# error_code is vPIC's own: 0 decoded clean, 1 check digit wrong, 400 illegal characters.\n")
	fmt.Fprintf(f, "# Regenerate with: go run ./tools/corpus vpic\n")
	w := csv.NewWriter(f)
	w.Write([]string{"vin", "error_code", "manufacturer", "manufacturer_id", "make", "model", "model_year", "vehicle_type", "plant_country"})
	for _, v := range vins {
		r, ok := have[v]
		if !ok {
			f.Close()
			return fmt.Errorf("no vPIC decode for %s", v)
		}
		year := ""
		if r.ModelYear != 0 {
			year = strconv.Itoa(r.ModelYear)
		}
		id := ""
		if r.ManufacturerID != 0 {
			id = strconv.Itoa(r.ManufacturerID)
		}
		w.Write([]string{v, strings.Join(r.ErrorCodes, ","), r.Manufacturer, id, r.Make, r.Model, year, r.VehicleType, r.PlantCountry})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// decodeChunk decodes up to MaxBatch VINs. NHTSA has recorded a few VINs with
// punctuation in them; those cannot go in a batch, whose syntax separates
// VINs with ';', so they are asked for one at a time instead of failing the
// whole run.
func decodeChunk(ctx context.Context, c *vpic.Client, vins []string) ([]vpic.Result, error) {
	plain := slices.Clone(vins)
	var odd []string
	plain = slices.DeleteFunc(plain, func(v string) bool {
		if strings.ContainsAny(v, ";,") {
			odd = append(odd, v)
			return true
		}
		return false
	})
	out, err := c.DecodeBatch(ctx, plain)
	if err != nil {
		return nil, err
	}
	for _, v := range odd {
		r, err := c.Decode(ctx, v)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}
