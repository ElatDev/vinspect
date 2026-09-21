// Package fetch holds the HTTP plumbing shared by the data-import tools
// under tools/: a transport that keeps NHTSA's servers happy by spacing
// requests out, retrying transient failures, and waiting out the CDN when it
// starts refusing. It is internal because it is tooling, not library API.
package fetch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// UserAgent identifies the import tools to NHTSA's servers.
const UserAgent = "vinspect-data-import (+https://github.com/ElatDev/vinspect)"

// Transport rate-limits and retries every request that goes through it.
type Transport struct {
	Base     http.RoundTripper
	Interval time.Duration // minimum gap between request starts
	Cooldown time.Duration // wait after a 403 or 429 before trying again
	Retries  int

	gate chan struct{} // holds the time the next request may start
	next time.Time
}

// NewTransport returns a Transport that starts at most one request per
// interval, across all goroutines using it.
func NewTransport(interval time.Duration) *Transport {
	t := &Transport{
		Base:     http.DefaultTransport,
		Interval: interval,
		Cooldown: 10 * time.Minute,
		Retries:  6,
		gate:     make(chan struct{}, 1),
	}
	t.gate <- struct{}{}
	return t
}

// NewHTTPClient returns an http.Client using a rate-limiting Transport.
func NewHTTPClient(interval time.Duration) *http.Client {
	return &http.Client{Transport: NewTransport(interval), Timeout: 120 * time.Second}
}

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get("User-Agent") == "" {
		req = req.Clone(req.Context())
		req.Header.Set("User-Agent", UserAgent)
	}
	var lastErr error
	for attempt := 0; ; attempt++ {
		if attempt > 0 {
			if attempt > t.Retries {
				return nil, lastErr
			}
			pause := time.Duration(attempt*attempt) * time.Second
			if blocked(lastErr) {
				pause = t.Cooldown
				log.Printf("NHTSA is refusing requests (%v); pausing %s", lastErr, pause)
			}
			if err := sleep(req.Context(), pause); err != nil {
				return nil, err
			}
			if err := rewind(req); err != nil {
				return nil, err
			}
		}
		if err := t.wait(req.Context()); err != nil {
			return nil, err
		}
		resp, err := t.Base.RoundTrip(req)
		switch {
		case err != nil:
			lastErr = err
		case resp.StatusCode == http.StatusForbidden, resp.StatusCode == http.StatusTooManyRequests:
			io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			lastErr = statusError(resp.StatusCode)
		case resp.StatusCode >= 500:
			io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			lastErr = statusError(resp.StatusCode)
		default:
			return resp, nil
		}
	}
}

// wait blocks until this request may start.
func (t *Transport) wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.gate:
	}
	now := time.Now()
	start := now
	if t.next.After(now) {
		start = t.next
	}
	t.next = start.Add(t.Interval)
	t.gate <- struct{}{}
	return sleep(ctx, start.Sub(now))
}

func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// rewind restores a request body so the request can be sent again.
func rewind(req *http.Request) error {
	if req.GetBody == nil {
		return nil
	}
	body, err := req.GetBody()
	if err != nil {
		return err
	}
	req.Body = body
	return nil
}

type statusError int

func (e statusError) Error() string { return fmt.Sprintf("HTTP %d", int(e)) }

func blocked(err error) bool {
	s, ok := err.(statusError)
	return ok && (s == http.StatusForbidden || s == http.StatusTooManyRequests)
}

// Client fetches JSON documents through a rate-limiting transport.
type Client struct{ HTTP *http.Client }

// New returns a Client that starts at most one request per interval.
func New(interval time.Duration) *Client { return &Client{HTTP: NewHTTPClient(interval)} }

// GetJSON fetches url and decodes the JSON body into v.
func (c *Client) GetJSON(ctx context.Context, url string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("GET %s: %s: %s", url, resp.Status, body)
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("GET %s: %w", url, err)
	}
	return nil
}
