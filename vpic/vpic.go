// Package vpic is a small client for NHTSA's vPIC VIN-decoding API
// (https://vpic.nhtsa.dot.gov/api/), for the details a VIN only encodes by
// reference: model, trim, body style, engine, plant location.
//
// It is the only package in vinspect that uses the network. Package vin
// does not import it, and the command-line tool calls it only when asked
// to with --online.
package vpic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// DefaultBaseURL is the public vPIC vehicles API.
const DefaultBaseURL = "https://vpic.nhtsa.dot.gov/api/vehicles/"

// MaxBatch is the most VINs vPIC decodes in one batch request.
const MaxBatch = 50

// ErrRateLimited is returned when vPIC's CDN refuses requests, which it does
// to clients that send too many too quickly. Waiting a few minutes clears it.
var ErrRateLimited = errors.New("vpic: request refused, probably rate-limited; wait a few minutes")

// Client decodes VINs through vPIC. The zero value is ready to use.
type Client struct {
	BaseURL   string       // defaults to DefaultBaseURL
	HTTP      *http.Client // defaults to a client with a 30-second timeout
	UserAgent string       // sent with every request if set
}

var defaultHTTP = &http.Client{Timeout: 30 * time.Second}

// Result is vPIC's decode of one VIN. vPIC answers with about 150 flat
// string fields; the ones most callers want are promoted to struct fields,
// and every non-empty field is kept in Fields under vPIC's own name.
type Result struct {
	VIN            string `json:"vin"`
	Make           string `json:"make,omitempty"`
	Model          string `json:"model,omitempty"`
	Trim           string `json:"trim,omitempty"`
	Series         string `json:"series,omitempty"`
	ModelYear      int    `json:"model_year,omitempty"`
	Manufacturer   string `json:"manufacturer,omitempty"`
	ManufacturerID int    `json:"manufacturer_id,omitempty"`
	VehicleType    string `json:"vehicle_type,omitempty"`
	BodyClass      string `json:"body_class,omitempty"`
	Doors          string `json:"doors,omitempty"`
	DriveType      string `json:"drive_type,omitempty"`
	FuelType       string `json:"fuel_type,omitempty"`
	Cylinders      string `json:"engine_cylinders,omitempty"`
	DisplacementL  string `json:"displacement_l,omitempty"`
	EngineHP       string `json:"engine_hp,omitempty"`
	Transmission   string `json:"transmission,omitempty"`
	Gears          string `json:"transmission_speeds,omitempty"`
	GVWR           string `json:"gvwr,omitempty"`
	PlantCity      string `json:"plant_city,omitempty"`
	PlantState     string `json:"plant_state,omitempty"`
	PlantCountry   string `json:"plant_country,omitempty"`

	// ErrorCodes are vPIC's decode diagnostics: "0" means the VIN decoded
	// cleanly, "1" that the check digit is wrong, and so on (see ErrorText).
	ErrorCodes   []string `json:"error_codes"`
	ErrorText    string   `json:"error_text,omitempty"`
	SuggestedVIN string   `json:"suggested_vin,omitempty"`

	Fields map[string]string `json:"-"`
}

// Clean reports whether vPIC decoded the VIN with no errors at all.
func (r Result) Clean() bool { return slices.Equal(r.ErrorCodes, []string{"0"}) }

// HasError reports whether vPIC raised the given error code.
func (r Result) HasError(code string) bool { return slices.Contains(r.ErrorCodes, code) }

// Decode asks vPIC to decode a single VIN.
func (c *Client) Decode(ctx context.Context, vin string) (Result, error) {
	u := c.base() + "DecodeVinValues/" + url.PathEscape(vin) + "?format=json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return Result{}, err
	}
	rs, err := c.do(req)
	if err != nil {
		return Result{}, err
	}
	if len(rs) != 1 {
		return Result{}, fmt.Errorf("vpic: %d results for one VIN", len(rs))
	}
	return rs[0], nil
}

// DecodeBatch decodes any number of VINs, MaxBatch per request, and returns
// the results in the order of vins. A VIN containing ';' or ',' cannot be
// expressed in vPIC's batch syntax and is rejected before anything is sent;
// neither character can occur in a real VIN.
func (c *Client) DecodeBatch(ctx context.Context, vins []string) ([]Result, error) {
	for _, v := range vins {
		if strings.ContainsAny(v, ";,") {
			return nil, fmt.Errorf("vpic: %q cannot be sent in a batch", v)
		}
	}
	out := make([]Result, 0, len(vins))
	for chunk := range slices.Chunk(vins, MaxBatch) {
		form := url.Values{"DATA": {strings.Join(chunk, ";")}, "format": {"json"}}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base()+"DecodeVINValuesBatch/", strings.NewReader(form.Encode()))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rs, err := c.do(req)
		if err != nil {
			return nil, err
		}
		if len(rs) != len(chunk) {
			return nil, fmt.Errorf("vpic: %d results for %d VINs", len(rs), len(chunk))
		}
		for i := range rs {
			if !strings.EqualFold(rs[i].VIN, chunk[i]) {
				return nil, fmt.Errorf("vpic: result %d is for %q, expected %q", i, rs[i].VIN, chunk[i])
			}
		}
		out = append(out, rs...)
	}
	return out, nil
}

func (c *Client) base() string {
	if c.BaseURL != "" {
		return strings.TrimSuffix(c.BaseURL, "/") + "/"
	}
	return DefaultBaseURL
}

func (c *Client) do(req *http.Request) ([]Result, error) {
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	req.Header.Set("Accept", "application/json")
	hc := c.HTTP
	if hc == nil {
		hc = defaultHTTP
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vpic: %w", err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusForbidden, http.StatusTooManyRequests:
		return nil, ErrRateLimited
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return nil, fmt.Errorf("vpic: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var payload struct {
		Results []map[string]any
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("vpic: decoding response: %w", err)
	}
	rs := make([]Result, len(payload.Results))
	for i, raw := range payload.Results {
		rs[i] = fromFields(raw)
	}
	return rs, nil
}

// fromFields turns one flat vPIC result into a Result.
func fromFields(raw map[string]any) Result {
	f := make(map[string]string, len(raw))
	for k, v := range raw {
		if v == nil {
			continue
		}
		if s := strings.TrimSpace(fmt.Sprint(v)); s != "" && s != "Not Applicable" {
			f[k] = s
		}
	}
	r := Result{
		VIN:           f["VIN"],
		Make:          f["Make"],
		Model:         f["Model"],
		Trim:          f["Trim"],
		Series:        f["Series"],
		Manufacturer:  f["Manufacturer"],
		VehicleType:   f["VehicleType"],
		BodyClass:     f["BodyClass"],
		Doors:         f["Doors"],
		DriveType:     f["DriveType"],
		FuelType:      f["FuelTypePrimary"],
		Cylinders:     f["EngineCylinders"],
		DisplacementL: f["DisplacementL"],
		EngineHP:      f["EngineHP"],
		Transmission:  f["TransmissionStyle"],
		Gears:         f["TransmissionSpeeds"],
		GVWR:          f["GVWR"],
		PlantCity:     f["PlantCity"],
		PlantState:    f["PlantState"],
		PlantCountry:  f["PlantCountry"],
		ErrorText:     f["ErrorText"],
		SuggestedVIN:  f["SuggestedVIN"],
		ErrorCodes:    []string{},
		Fields:        f,
	}
	r.ModelYear, _ = strconv.Atoi(f["ModelYear"])
	r.ManufacturerID, _ = strconv.Atoi(f["ManufacturerId"])
	for code := range strings.SplitSeq(f["ErrorCode"], ",") {
		if code = strings.TrimSpace(code); code != "" {
			r.ErrorCodes = append(r.ErrorCodes, code)
		}
	}
	return r
}
