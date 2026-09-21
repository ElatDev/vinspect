package vpic

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
)

// The fixtures in testdata are real vPIC responses, recorded once, so these
// tests never touch the network.

func serveFile(t *testing.T, path string) *httptest.Server {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestDecode(t *testing.T) {
	srv := serveFile(t, "testdata/decode-1HGCM82633A004352.json")
	c := &Client{BaseURL: srv.URL}
	r, err := c.Decode(context.Background(), "1HGCM82633A004352")
	if err != nil {
		t.Fatal(err)
	}
	want := Result{
		VIN: "1HGCM82633A004352", Make: "HONDA", Model: "Accord", Trim: "EX-V6",
		ModelYear: 2003, Manufacturer: "AMERICAN HONDA MOTOR CO., INC.", ManufacturerID: 988,
		VehicleType: "PASSENGER CAR", BodyClass: "Coupe", Doors: "2", FuelType: "Gasoline",
		Cylinders: "6", DisplacementL: "2.998832712", EngineHP: "240", Transmission: "Automatic",
		Gears: "5", PlantCity: "MARYSVILLE", PlantState: "OHIO", PlantCountry: "UNITED STATES (USA)",
		ErrorCodes: []string{"0"},
	}
	got := r
	got.Fields, got.ErrorText, got.GVWR = nil, "", ""
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
	if !r.Clean() || r.HasError("1") {
		t.Errorf("Clean() = %v, HasError(1) = %v", r.Clean(), r.HasError("1"))
	}
	if r.Fields["EngineModel"] != "J30A4" {
		t.Errorf("Fields[EngineModel] = %q", r.Fields["EngineModel"])
	}
	if _, ok := r.Fields["BusType"]; ok {
		t.Error(`"Not Applicable" fields should be dropped`)
	}
}

func TestDecodeBatchFixture(t *testing.T) {
	srv := serveFile(t, "testdata/batch.json")
	c := &Client{BaseURL: srv.URL}
	rs, err := c.DecodeBatch(context.Background(), []string{"JM1BN1U70H1100922", "1HGCM82633A004353"})
	if err != nil {
		t.Fatal(err)
	}
	if rs[0].ModelYear != 2017 || rs[0].Model != "Mazda3" || !rs[0].Clean() {
		t.Errorf("first result %+v", rs[0])
	}
	if !rs[1].HasError("1") || rs[1].Clean() {
		t.Errorf("second result should carry error 1 (check digit): %+v", rs[1].ErrorCodes)
	}
}

// TestDecodeBatchChunks checks that large batches are split into requests
// of at most MaxBatch VINs and reassembled in order.
func TestDecodeBatchChunks(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/DecodeVINValuesBatch/") {
			http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusBadRequest)
			return
		}
		r.ParseForm()
		if r.Form.Get("format") != "json" {
			http.Error(w, "format", http.StatusBadRequest)
			return
		}
		vins := strings.Split(r.Form.Get("DATA"), ";")
		if len(vins) > MaxBatch {
			http.Error(w, "too many", http.StatusBadRequest)
			return
		}
		var parts []string
		for _, v := range vins {
			parts = append(parts, fmt.Sprintf(`{"VIN":%q,"ErrorCode":"0","ModelYear":"2020"}`, v))
		}
		fmt.Fprintf(w, `{"Count":%d,"Results":[%s]}`, len(parts), strings.Join(parts, ","))
	}))
	defer srv.Close()

	var vins []string
	for i := range 120 {
		vins = append(vins, fmt.Sprintf("TESTVIN%010d", i))
	}
	rs, err := (&Client{BaseURL: srv.URL}).DecodeBatch(context.Background(), vins)
	if err != nil {
		t.Fatal(err)
	}
	if n := requests.Load(); n != 3 {
		t.Errorf("%d requests for 120 VINs, want 3", n)
	}
	var got []string
	for _, r := range rs {
		got = append(got, r.VIN)
	}
	if !slices.Equal(got, vins) {
		t.Error("results out of order")
	}
}

func TestDecodeBatchRejectsSeparators(t *testing.T) {
	c := &Client{BaseURL: "http://127.0.0.1:1"} // never contacted
	if _, err := c.DecodeBatch(context.Background(), []string{"A;B"}); err == nil {
		t.Error("want an error for a VIN containing ';'")
	}
}

func TestErrors(t *testing.T) {
	for status, want := range map[int]error{
		http.StatusForbidden:       ErrRateLimited,
		http.StatusTooManyRequests: ErrRateLimited,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
		}))
		_, err := (&Client{BaseURL: srv.URL}).Decode(context.Background(), "1HGCM82633A004352")
		srv.Close()
		if !errors.Is(err, want) {
			t.Errorf("status %d: err = %v, want %v", status, err, want)
		}
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	if _, err := (&Client{BaseURL: srv.URL}).Decode(context.Background(), "1HGCM82633A004352"); err == nil || !strings.Contains(err.Error(), "500") {
		t.Errorf("err = %v, want a 500 error", err)
	}
}

func TestUserAgentAndCancel(t *testing.T) {
	var ua atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua.Store(r.Header.Get("User-Agent"))
		fmt.Fprint(w, `{"Results":[{"VIN":"X","ErrorCode":"0"}]}`)
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, UserAgent: "vinspect-test"}
	if _, err := c.Decode(context.Background(), "X"); err != nil {
		t.Fatal(err)
	}
	if ua.Load() != "vinspect-test" {
		t.Errorf("User-Agent = %v", ua.Load())
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Decode(ctx, "X"); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}
