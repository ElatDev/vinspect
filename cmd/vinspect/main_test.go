package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ElatDev/vinspect/vpic"
)

var update = flag.Bool("update", false, "rewrite the golden files in testdata")

type result struct {
	code           int
	stdout, stderr string
}

func runWith(stdin string, args ...string) result {
	var out, errOut bytes.Buffer
	code := run(args, strings.NewReader(stdin), &out, &errOut)
	return result{code, out.String(), errOut.String()}
}

// golden compares got with testdata/name, or rewrites it with -update.
func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test -update to create it)", err)
	}
	if got != strings.ReplaceAll(string(want), "\r\n", "\n") {
		t.Errorf("output differs from %s (go test -update rewrites it)\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
	}
}

func TestCheckTable(t *testing.T) {
	r := runWith("", "check", "1HGCM82633A004352", "1HGCM82633A004353", "1hgcm82633a0o4352", "WP0ZZZ99ZTS392124", "abc")
	if r.code != 1 {
		t.Errorf("exit %d, want 1", r.code)
	}
	golden(t, "check.golden", r.stdout)
	if r.stderr != "5 checked: 1 valid, 4 invalid\n" {
		t.Errorf("stderr %q", r.stderr)
	}
}

func TestCheckAllValid(t *testing.T) {
	r := runWith("", "check", "1HGCM82633A004352")
	if r.code != 0 || r.stderr != "" {
		t.Errorf("exit %d, stderr %q", r.code, r.stderr)
	}
}

func TestCheckJSON(t *testing.T) {
	r := runWith("", "check", "--json", "1HGCM82633A004352", "1HGCM82633A004353", "1HGCM82633Q004352", "1HG")
	if r.code != 1 || r.stderr != "" {
		t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
	}
	lines := strings.Split(strings.TrimSpace(r.stdout), "\n")
	if len(lines) != 4 {
		t.Fatalf("%d JSON lines, want 4:\n%s", len(lines), r.stdout)
	}
	var recs []checkRecord
	for _, l := range lines {
		var rec checkRecord
		if err := json.Unmarshal([]byte(l), &rec); err != nil {
			t.Fatalf("%q: %v", l, err)
		}
		recs = append(recs, rec)
	}
	if !recs[0].Valid || recs[0].Error != "" {
		t.Errorf("first: %+v", recs[0])
	}
	if recs[1].Error != "check_digit" || recs[1].Have != "3" || recs[1].Want != "5" {
		t.Errorf("second: %+v", recs[1])
	}
	if recs[2].Error != "character" || recs[2].Position != 11 || recs[2].Char != "Q" {
		t.Errorf("third: %+v", recs[2])
	}
	if recs[3].Error != "length" || recs[3].Length != 3 {
		t.Errorf("fourth: %+v", recs[3])
	}
}

func TestCheckStdin(t *testing.T) {
	stdin := "\uFEFF1HGCM82633A004352\r\n# a comment\n\n   jm1bn1u70h1100922  \n"
	r := runWith(stdin, "check")
	if r.code != 0 {
		t.Fatalf("exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	if !strings.Contains(r.stdout, "1HGCM82633A004352  valid") || !strings.Contains(r.stdout, "JM1BN1U70H1100922  valid") {
		t.Errorf("stdout:\n%s", r.stdout)
	}
	if r.stderr != "2 checked: 2 valid, 0 invalid\n" {
		t.Errorf("stderr %q", r.stderr)
	}
	// "-" reads stdin explicitly.
	if r := runWith("1HGCM82633A004352\n", "check", "-"); r.code != 0 {
		t.Errorf("check - : exit %d", r.code)
	}
}

func TestDecodeTree(t *testing.T) {
	for _, vin := range []string{"1HGCM82633A004352", "WP0ZZZ99ZTS392124", "1FTFW1ET9DFC10312", "LRW3E7EA6MC000000"} {
		r := runWith("", "decode", vin)
		if r.stderr != "" {
			t.Errorf("%s: stderr %q", vin, r.stderr)
		}
		if r.code != 0 {
			t.Errorf("%s: exit %d", vin, r.code)
		}
		golden(t, "decode-"+vin+".golden", r.stdout)
	}
}

func TestDecodeTable(t *testing.T) {
	stdin := "1HGCM82633A004352\nJM1BN1U70H1100922\n1FTFW1ET9DFC10312\nLRW3E7EA6MC000000\nnot-a-vin\n"
	r := runWith(stdin, "decode")
	if r.code != 1 {
		t.Errorf("exit %d, want 1 for the malformed line", r.code)
	}
	golden(t, "decode-table.golden", r.stdout)
}

func TestDecodeJSON(t *testing.T) {
	// Flags may come after the VINs.
	r := runWith("", "decode", "1HGCM82633A004352", "x", "--json")
	if r.code != 1 {
		t.Errorf("exit %d", r.code)
	}
	lines := strings.Split(strings.TrimSpace(r.stdout), "\n")
	if len(lines) != 2 {
		t.Fatalf("%d lines:\n%s", len(lines), r.stdout)
	}
	var ok struct {
		Input        string `json:"input"`
		VIN          string `json:"vin"`
		WMI          string `json:"wmi"`
		Manufacturer struct {
			Manufacturer string `json:"manufacturer"`
		} `json:"manufacturer"`
		ModelYear struct {
			Year       int   `json:"year"`
			Candidates []int `json:"candidates"`
		} `json:"model_year"`
		CheckDigit struct {
			Valid bool `json:"valid"`
		} `json:"check_digit"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &ok); err != nil {
		t.Fatal(err)
	}
	if ok.VIN != "1HGCM82633A004352" || ok.WMI != "1HG" || ok.ModelYear.Year != 2003 || !ok.CheckDigit.Valid ||
		ok.Manufacturer.Manufacturer != "AMERICAN HONDA MOTOR CO., INC." {
		t.Errorf("decoded %+v", ok)
	}
	var bad map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &bad); err != nil {
		t.Fatal(err)
	}
	if bad["error"] != "length" || bad["vin"] != nil {
		t.Errorf("error record %v", bad)
	}
}

func withVPIC(t *testing.T, h http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	orig := newVPICClient
	newVPICClient = func() *vpic.Client { return &vpic.Client{BaseURL: srv.URL} }
	t.Cleanup(func() { newVPICClient = orig })
}

func TestDecodeOnline(t *testing.T) {
	fixture, err := os.ReadFile("../../vpic/testdata/decode-1HGCM82633A004352.json")
	if err != nil {
		t.Fatal(err)
	}
	withVPIC(t, func(w http.ResponseWriter, r *http.Request) { w.Write(fixture) })
	r := runWith("", "decode", "--online", "1HGCM82633A004352")
	if r.code != 0 || r.stderr != "" {
		t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
	}
	golden(t, "decode-online.golden", r.stdout)
}

func TestDecodeOnlineBatch(t *testing.T) {
	fixture, err := os.ReadFile("../../vpic/testdata/batch.json")
	if err != nil {
		t.Fatal(err)
	}
	withVPIC(t, func(w http.ResponseWriter, r *http.Request) { w.Write(fixture) })
	r := runWith("JM1BN1U70H1100922\n1HGCM82633A004353\nJM1BN1U70H1100922\n", "decode", "--online")
	if r.code != 0 {
		t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
	}
	golden(t, "decode-online-table.golden", r.stdout)
}

func TestDecodeOnlineFailure(t *testing.T) {
	withVPIC(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) })
	r := runWith("", "decode", "--online", "1HGCM82633A004352")
	if r.code != 1 {
		t.Errorf("exit %d, want 1", r.code)
	}
	if !strings.Contains(r.stderr, "offline results only") || !strings.Contains(r.stderr, "rate-limited") {
		t.Errorf("stderr %q", r.stderr)
	}
	if !strings.Contains(r.stdout, "AMERICAN HONDA") {
		t.Errorf("offline decode missing:\n%s", r.stdout)
	}
}

func TestColor(t *testing.T) {
	if r := runWith("", "check", "--color=always", "1HGCM82633A004352"); !strings.Contains(r.stdout, "\x1b[32m") {
		t.Errorf("--color=always produced no escape codes: %q", r.stdout)
	}
	if r := runWith("", "check", "--color", "never", "1HGCM82633A004352"); strings.Contains(r.stdout, "\x1b[") {
		t.Errorf("--color never produced escape codes: %q", r.stdout)
	}
	// Output that is not a terminal gets no color under auto.
	if r := runWith("", "decode", "1HGCM82633A004352"); strings.Contains(r.stdout, "\x1b[") {
		t.Errorf("auto colored a non-terminal")
	}
	if r := runWith("", "check", "--color=sometimes", "1HGCM82633A004352"); r.code != 2 {
		t.Errorf("bad --color: exit %d", r.code)
	}
}

func TestUsage(t *testing.T) {
	tests := []struct {
		args []string
		code int
		out  string // substring of stdout+stderr
	}{
		{nil, 2, "Usage:"},
		{[]string{"help"}, 0, "Usage:"},
		{[]string{"--help"}, 0, "Usage:"},
		{[]string{"check", "-h"}, 0, "Usage:"},
		{[]string{"version"}, 0, "vinspect "},
		{[]string{"frobnicate"}, 2, `unknown command "frobnicate"`},
		{[]string{"check", "--bogus"}, 2, "flag provided but not defined"},
		{[]string{"check"}, 2, "no VINs given"},
		{[]string{"decode"}, 2, "no VINs given"},
	}
	for _, tt := range tests {
		r := runWith("", tt.args...)
		if r.code != tt.code || !strings.Contains(r.stdout+r.stderr, tt.out) {
			t.Errorf("%v: exit %d, output %q; want exit %d containing %q", tt.args, r.code, r.stdout+r.stderr, tt.code, tt.out)
		}
	}
}

func TestParseArgs(t *testing.T) {
	var cf commonFlags
	fs := newFlagSet("t", &bytes.Buffer{}, &cf)
	rest, err := parseArgs(fs, []string{"A", "--color", "never", "B", "--json", "--", "--C"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(rest, " ") != "A B --C" || cf.color != "never" || !cf.json {
		t.Errorf("rest %q, color %q, json %v", rest, cf.color, cf.json)
	}
}
