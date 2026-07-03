// Package client provides an HTTP client for the Slurm REST API (slurmrestd).
//
// It handles JWT authentication, API versioning, and provides typed methods
// for each slurmdb endpoint used by the provider.
//
// File layout:
//   - client.go  — Client struct, NewClient, error types, shared types
//     (SlurmInt / SlurmFloat / TRES), Ping
//   - retry.go   — doRequest retry loop, isRetryable classifier, backoff
//   - cluster.go — /clusters/ endpoints
//   - account.go — /accounts/ + /accounts_association/ endpoints
//   - qos.go     — /qos/ endpoints and the deeply-nested QOS limit types
//   - user.go    — /users/ + /users_association/ endpoints
//   - assoc.go   — /associations/ endpoints and the AssociationMax tree
package client

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Client is an HTTP client for the Slurm REST API.
type Client struct {
	// BaseURL is the slurmrestd endpoint (e.g. "http://localhost:6820")
	BaseURL string

	// Token is the JWT token for authentication
	Token string

	// Cluster is the Slurm cluster name
	Cluster string

	// APIVersion is the data_parser version (e.g. "v0.0.42")
	APIVersion string

	// UserAgent is sent as the User-Agent header on every request so
	// slurmrestd logs can attribute traffic to this provider. NewClient sets
	// a default; the provider overrides it with the release version.
	UserAgent string

	// HTTPClient is the underlying HTTP client
	HTTPClient *http.Client

	// MaxRetries is the number of retry attempts on transient HTTP failures.
	// Total attempts = MaxRetries + 1. Only retryable errors (5xx subset,
	// 408, 429, network-level failures) are retried; deterministic Slurm
	// rejections (4xx, 200-with-errors) are returned immediately.
	MaxRetries int

	// RetryBackoff returns the sleep duration before retry attempt n
	// (1-indexed: n=1 is the first retry). Tests override this to make the
	// retry loop run instantly.
	RetryBackoff func(attempt int) time.Duration

	// sleep is indirected for tests. It must return early with ctx.Err()
	// when the context is cancelled mid-backoff.
	sleep func(ctx context.Context, d time.Duration) error

	// deleteMu serializes all delete operations. slurmdbd uses optimistic locking
	// and returns MySQL error 1020 when concurrent deletes race on cross-row
	// updates (e.g. QOS preempt references, account association cascades).
	deleteMu sync.Mutex
}

// NewClient creates a new Slurm REST API client.
func NewClient(baseURL, token, cluster, apiVersion string) *Client {
	return &Client{
		BaseURL:    baseURL,
		Token:      token,
		Cluster:    cluster,
		APIVersion: apiVersion,
		UserAgent:  "terraform-provider-slurm",
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		MaxRetries:   2, // 3 attempts total
		RetryBackoff: defaultRetryBackoff,
		sleep:        sleepCtx,
	}
}

// sleepCtx sleeps for d but returns early with the context's error when ctx
// is cancelled first. This keeps retry backoff responsive to Terraform's
// cancellation (Ctrl-C, timeouts).
func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// APIError represents an error returned by the Slurm REST API.
type APIError struct {
	StatusCode int
	Body       string
	Errors     []SlurmError
}

func (e *APIError) Error() string {
	if len(e.Errors) > 0 {
		msg := fmt.Sprintf("slurm API error (HTTP %d):", e.StatusCode)
		for _, slurmErr := range e.Errors {
			msg += fmt.Sprintf(" %s", slurmErr.Description)
			// Include the Error field when it adds detail beyond Description.
			// Slurm sometimes puts the specific constraint message there
			// (e.g. QOS access violations) while Description is a generic
			// "slurmdb_X failed" string.
			if slurmErr.Error != "" && slurmErr.Error != slurmErr.Description {
				msg += fmt.Sprintf(" (%s)", slurmErr.Error)
			}
		}
		return msg
	}
	return fmt.Sprintf("slurm API error (HTTP %d): %s", e.StatusCode, e.Body)
}

// SlurmError is an individual error from a Slurm API response.
type SlurmError struct {
	Description string `json:"description"`
	Source      string `json:"source"`
	Error       string `json:"error"`
	ErrorNumber int    `json:"error_number"`
}

// SlurmWarning is an individual warning from a Slurm API response.
type SlurmWarning struct {
	Description string `json:"description"`
	Source      string `json:"source"`
}

// baseResponse contains the common fields in every Slurm API response.
type baseResponse struct {
	Errors   []SlurmError   `json:"errors"`
	Warnings []SlurmWarning `json:"warnings"`
}

// slurmdbPath builds a URL path for a slurmdb endpoint.
// Example: slurmdbPath("accounts") => "/slurmdb/v0.0.42/accounts/"
func (c *Client) slurmdbPath(endpoint string) string {
	return fmt.Sprintf("/slurmdb/%s/%s", c.APIVersion, endpoint)
}

// SlurmInt represents Slurm's integer type which includes set/infinite flags.
// Omitting infinite when false matches slurmrestd's default and avoids sending
// an explicit "infinite":false that can confuse slurmdbd's query-back logic.
type SlurmInt struct {
	Number   int  `json:"number"`
	Set      bool `json:"set"`
	Infinite bool `json:"infinite,omitempty"`
}

// SlurmFloat is like SlurmInt but for API fields that Slurm serialises as a
// JSON float (e.g. usage_factor returns 1.0, not 1). Using float64 avoids
// "cannot unmarshal number 1.0 into Go struct field … of type int".
type SlurmFloat struct {
	Number   float64 `json:"number"`
	Set      bool    `json:"set"`
	Infinite bool    `json:"infinite,omitempty"`
}

// TRES represents a Trackable Resource (cpu, mem, gres, …) with a count limit.
type TRES struct {
	Type  string `json:"type"`
	Name  string `json:"name,omitempty"`
	ID    int    `json:"id,omitempty"`
	Count int64  `json:"count"`
}

// Ping checks connectivity to slurmrestd.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.doRequest(ctx, http.MethodGet, c.slurmdbPath("diag/"), nil)
	return err
}
