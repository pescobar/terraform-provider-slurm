package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// newTestClient wires a Client to point at a test server and replaces the
// retry sleep with a no-op so the loop runs instantly.
func newTestClient(t *testing.T, srv *httptest.Server, maxRetries int) *Client {
	t.Helper()
	return &Client{
		BaseURL:      srv.URL,
		Token:        "test-token",
		Cluster:      "linux",
		APIVersion:   "v0.0.42",
		HTTPClient:   srv.Client(),
		MaxRetries:   maxRetries,
		RetryBackoff: func(int) time.Duration { return 0 },
		sleep:        func(context.Context, time.Duration) error { return nil },
	}
}

// ---------------------------------------------------------------------------
// isRetryable — pure classifier
// ---------------------------------------------------------------------------

func TestIsRetryable_HTTPStatusCodes(t *testing.T) {
	tests := []struct {
		code int
		want bool
	}{
		{http.StatusBadRequest, false},          // 400 — caller error
		{http.StatusUnauthorized, false},        // 401 — bad token, retry won't help
		{http.StatusForbidden, false},           // 403
		{http.StatusNotFound, false},            // 404
		{http.StatusInternalServerError, false}, // 500 — Slurm uses this for deterministic errors
		{http.StatusRequestTimeout, true},       // 408
		{http.StatusTooManyRequests, true},      // 429
		{http.StatusBadGateway, true},           // 502
		{http.StatusServiceUnavailable, true},   // 503
		{http.StatusGatewayTimeout, true},       // 504
	}
	for _, tc := range tests {
		t.Run(http.StatusText(tc.code), func(t *testing.T) {
			err := &APIError{StatusCode: tc.code}
			if got := isRetryable(err); got != tc.want {
				t.Errorf("HTTP %d: want retryable=%v, got %v", tc.code, tc.want, got)
			}
		})
	}
}

func TestIsRetryable_NetworkErrors(t *testing.T) {
	if !isRetryable(io.EOF) {
		t.Error("io.EOF should be retryable")
	}
	if !isRetryable(io.ErrUnexpectedEOF) {
		t.Error("io.ErrUnexpectedEOF should be retryable")
	}
	if !isRetryable(fmt.Errorf("wrapped: %w", io.EOF)) {
		t.Error("wrapped io.EOF should be retryable")
	}
}

func TestIsRetryable_NonRetryable(t *testing.T) {
	if isRetryable(nil) {
		t.Error("nil error should not be retryable")
	}
	if isRetryable(errors.New("plain error")) {
		t.Error("plain (non-net, non-API) error should not be retryable")
	}
	// 200-with-errors-in-body is deterministic — never retry.
	apiErr := &APIError{StatusCode: 200, Errors: []SlurmError{{Description: "bad"}}}
	if isRetryable(apiErr) {
		t.Error("200-with-errors should not be retryable")
	}
}

// ---------------------------------------------------------------------------
// defaultRetryBackoff
// ---------------------------------------------------------------------------

func TestDefaultRetryBackoff(t *testing.T) {
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{0, 0},
		{1, 250 * time.Millisecond},
		{2, 500 * time.Millisecond},
		{3, 1 * time.Second},
		{4, 2 * time.Second},
		{5, 4 * time.Second},
		{6, 5 * time.Second}, // capped
		{20, 5 * time.Second},
	}
	for _, tc := range tests {
		if got := defaultRetryBackoff(tc.attempt); got != tc.want {
			t.Errorf("attempt %d: want %v, got %v", tc.attempt, tc.want, got)
		}
	}
}

// ---------------------------------------------------------------------------
// doRequest — retry behaviour against a controlled HTTP server
// ---------------------------------------------------------------------------

func TestDoRequest_RetriesOn503ThenSucceeds(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv, 2)
	body, err := c.doRequest(context.Background(), http.MethodGet, "/test", nil)
	if err != nil {
		t.Fatalf("expected success on retry, got %v", err)
	}
	if string(body) != "{}" {
		t.Errorf("unexpected body: %s", body)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("expected 2 calls (initial + 1 retry), got %d", got)
	}
}

func TestDoRequest_DoesNotRetryOn400(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"errors":[{"description":"bad request"}]}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv, 2)
	_, err := c.doRequest(context.Background(), http.MethodPost, "/test", map[string]string{"a": "b"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusBadRequest {
		t.Errorf("expected APIError(400), got %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("expected exactly 1 call (no retry on 400), got %d", got)
	}
}

func TestDoRequest_DoesNotRetryOn500(t *testing.T) {
	// Slurm returns 500 for deterministic user-input errors. Retrying
	// would just amplify the same failure.
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"errors":[{"description":"Missing required field 'user'"}]}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv, 2)
	_, err := c.doRequest(context.Background(), http.MethodPost, "/test", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("expected 1 call (500 not retried), got %d", got)
	}
}

func TestDoRequest_GivesUpAfterMaxAttempts(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := newTestClient(t, srv, 2) // 3 attempts total
	_, err := c.doRequest(context.Background(), http.MethodGet, "/test", nil)
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected final error to be APIError(503), got %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("expected 3 calls (initial + 2 retries), got %d", got)
	}
}

func TestDoRequest_NoRetriesWhenMaxRetriesZero(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	c := newTestClient(t, srv, 0)
	_, err := c.doRequest(context.Background(), http.MethodGet, "/test", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("MaxRetries=0 should yield exactly 1 attempt, got %d", got)
	}
}

func TestDoRequest_BodyReplayedOnEachAttempt(t *testing.T) {
	// Each retry must send the same body. Use a server that records bodies
	// and returns 503 once, then 200.
	var calls int32
	var receivedBodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		receivedBodies = append(receivedBodies, string(b))
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv, 2)
	_, err := c.doRequest(context.Background(), http.MethodPost, "/test", map[string]int{"x": 7})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if len(receivedBodies) != 2 {
		t.Fatalf("expected 2 bodies received, got %d", len(receivedBodies))
	}
	if receivedBodies[0] != receivedBodies[1] {
		t.Errorf("retry sent a different body:\nfirst:  %s\nsecond: %s", receivedBodies[0], receivedBodies[1])
	}
}

func TestDoRequest_CancelledContextStopsRetries(t *testing.T) {
	// The server always returns a retryable 503. Cancelling the context
	// after the first attempt must abort the loop during the backoff sleep
	// instead of burning through the remaining attempts.
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	c := newTestClient(t, srv, 5)
	c.sleep = func(ctx context.Context, d time.Duration) error {
		cancel() // simulate Ctrl-C arriving while backing off
		return ctx.Err()
	}

	_, err := c.doRequest(ctx, http.MethodGet, "/test", nil)
	if err == nil {
		t.Fatal("expected error after cancellation")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled in error chain, got %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("expected 1 call (cancelled during first backoff), got %d", got)
	}
}

func TestSleepCtx_ReturnsEarlyOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if err := sleepCtx(ctx, 5*time.Second); !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("sleepCtx did not return early: took %v", elapsed)
	}
}

func TestDoRequest_RetriesOnConnectionClose(t *testing.T) {
	// Server hijacks the connection and closes it without writing a
	// response on the first call — simulates slurmrestd restart mid-request.
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("ResponseWriter does not support Hijacker")
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				t.Fatalf("hijack: %v", err)
			}
			_ = conn.Close()
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv, 2)
	_, err := c.doRequest(context.Background(), http.MethodGet, "/test", nil)
	if err != nil {
		t.Fatalf("expected retry to recover from dropped connection, got %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("expected 2 calls (initial drop + 1 retry), got %d", got)
	}
}
