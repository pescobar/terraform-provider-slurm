package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// defaultRetryBackoff produces 250ms, 500ms, 1s, 2s, 4s … capped at 5s.
// No jitter — slurmrestd is single-tenant for a given provider so thundering
// herd from a tofu apply isn't a real concern, and deterministic timing
// makes failures easier to reason about in CI logs.
func defaultRetryBackoff(attempt int) time.Duration {
	const base = 250 * time.Millisecond
	const maxDelay = 5 * time.Second
	if attempt < 1 {
		return 0
	}
	d := base << (attempt - 1)
	if d > maxDelay || d <= 0 {
		return maxDelay
	}
	return d
}

// isRetryable returns true when err is a transient failure that is safe to
// retry. The classifier is intentionally narrow:
//
//   - APIError with HTTP 408, 429, 502, 503, 504 → transient gateway/load
//     issues. Slurm 500 is excluded because slurmrestd uses it for
//     deterministic user-input errors (e.g. "Missing required field"); a
//     retry would just amplify the same failure.
//   - Network-level errors: dial failures, connection resets, EOFs the
//     server emits when restarting mid-stream. Always safe — none are
//     deterministic.
//   - 200-with-errors-in-body Slurm responses → never retried. Those are
//     application-level rejections (constraint violations, missing parents)
//     that won't change on the next attempt.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case http.StatusRequestTimeout,
			http.StatusTooManyRequests,
			http.StatusBadGateway,
			http.StatusServiceUnavailable,
			http.StatusGatewayTimeout:
			return true
		}
		return false
	}
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	return false
}

// doRequest performs an HTTP request against slurmrestd, retrying transient
// failures with exponential backoff (see isRetryable / RetryBackoff).
// All Slurm POST endpoints we use are idempotent upserts and DELETE on a
// missing resource is a no-op, so retrying is always safe at the protocol
// level. The body is marshalled once and replayed from a byte slice so each
// attempt sends the exact same request.
//
// The context is threaded into every HTTP attempt and into the backoff
// sleeps, so cancelling it (Terraform Ctrl-C, timeouts) aborts the request
// immediately instead of finishing the retry loop.
func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}) ([]byte, error) {
	var jsonBody []byte
	if body != nil {
		var err error
		jsonBody, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
	}

	maxAttempts := c.MaxRetries + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			if err := c.sleep(ctx, c.RetryBackoff(attempt)); err != nil {
				return nil, fmt.Errorf("request cancelled during retry backoff: %w", err)
			}
		}
		respBody, err := c.doRequestOnce(ctx, method, path, jsonBody)
		if err == nil {
			return respBody, nil
		}
		lastErr = err
		if !isRetryable(err) || attempt == maxAttempts-1 {
			return nil, err
		}
	}
	return nil, lastErr
}

// doRequestOnce issues a single HTTP attempt and returns the (parsed) result.
func (c *Client) doRequestOnce(ctx context.Context, method, path string, jsonBody []byte) ([]byte, error) {
	var bodyReader io.Reader
	if jsonBody != nil {
		bodyReader = bytes.NewReader(jsonBody)
	}

	reqURL := fmt.Sprintf("%s%s", c.BaseURL, path)
	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-SLURM-USER-TOKEN", c.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check for HTTP-level errors.
	// HTTP 304 (Not Modified) is returned by Slurm when a POST results in no
	// changes (the submitted data matches the current state). Treat it as
	// success — the resource is already in the desired state.
	if resp.StatusCode == http.StatusNotModified {
		return nil, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		apiErr := &APIError{
			StatusCode: resp.StatusCode,
			Body:       string(respBody),
		}
		var base baseResponse
		if json.Unmarshal(respBody, &base) == nil {
			apiErr.Errors = base.Errors
		}
		return nil, apiErr
	}

	// Even on 200, Slurm may return errors in the body
	var base baseResponse
	if err := json.Unmarshal(respBody, &base); err == nil && len(base.Errors) > 0 {
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Body:       string(respBody),
			Errors:     base.Errors,
		}
	}

	return respBody, nil
}
