package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// The /conf endpoints were added in Slurm 26.05 and exist only in API
// version v0.0.45 and later. Two layers guard against confusing failures:
//
//  1. requireAPIVersion — a deterministic pre-flight check against the
//     provider's configured api_version, so `tofu plan` fails with a clear
//     "this feature needs Slurm ≥ X" message before any HTTP round-trip.
//  2. IsNotFound — catches the HTTP 404 that still occurs when api_version
//     is new enough but the server itself runs an older Slurm release.

// confMinAPIVersion is the first API version that serves the /conf endpoints,
// shipped with Slurm 26.05.
const (
	confMinAPIVersion   = 45
	confMinSlurmVersion = "26.05"
)

// VersionError reports that a feature is not available in the provider's
// configured API version.
type VersionError struct {
	Feature       string // human name, e.g. "slurmctld configuration endpoint"
	MinAPI        int    // minimum v0.0.NN
	MinSlurm      string // first Slurm release shipping MinAPI
	ConfiguredAPI string // the api_version the provider is running with
}

func (e *VersionError) Error() string {
	return fmt.Sprintf(
		"the %s requires Slurm %s or later (API version v0.0.%d+), but the provider is configured with api_version %s. "+
			"Set api_version = \"v0.0.%d\" (provider block or SLURM_API_VERSION) and make sure the cluster runs Slurm %s+.",
		e.Feature, e.MinSlurm, e.MinAPI, e.ConfiguredAPI, e.MinAPI, e.MinSlurm,
	)
}

// apiVersionNumber parses the trailing NN of "v0.0.NN". It returns -1 for
// unparsable values so an exotic api_version never blocks a request — the
// server stays the authority (the request may then 404).
func apiVersionNumber(v string) int {
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return -1
	}
	n, err := strconv.Atoi(parts[2])
	if err != nil {
		return -1
	}
	return n
}

// requireAPIVersion returns a VersionError when the configured api_version is
// recognisably older than min. Unparsable versions pass (see apiVersionNumber).
func (c *Client) requireAPIVersion(min int, minSlurm, feature string) error {
	n := apiVersionNumber(c.APIVersion)
	if n >= 0 && n < min {
		return &VersionError{
			Feature:       feature,
			MinAPI:        min,
			MinSlurm:      minSlurm,
			ConfiguredAPI: c.APIVersion,
		}
	}
	return nil
}

// SlurmConfMeta is the slurm_conf_meta block of GET /slurm/{v}/conf.
// Field names follow the API's capitalisation, not the usual snake_case.
type SlurmConfMeta struct {
	SlurmVersion string `json:"SLURM_VERSION"`
	ConfPath     string `json:"SLURM_CONF"`
	LastUpdate   int64  `json:"LastUpdate"`
}

// slurmConfResponse is the response from GET /slurm/{version}/conf.
type slurmConfResponse struct {
	SlurmConf map[string]json.RawMessage `json:"slurm_conf"`
	Meta      *SlurmConfMeta             `json:"slurm_conf_meta"`
	Errors    []SlurmError               `json:"errors"`
	Warnings  []SlurmWarning             `json:"warnings"`
}

// slurmdbdConfResponse is the response from GET /slurmdb/{version}/conf.
type slurmdbdConfResponse struct {
	SlurmdbdConf map[string]json.RawMessage `json:"slurmdbd_conf"`
	Errors       []SlurmError               `json:"errors"`
	Warnings     []SlurmWarning             `json:"warnings"`
}

// GetSlurmConf returns the active slurmctld configuration as a raw key →
// JSON-value map (keys use slurm.conf capitalisation, e.g. "ClusterName")
// plus the meta block. Requires API v0.0.45+ (Slurm 26.05).
func (c *Client) GetSlurmConf(ctx context.Context) (map[string]json.RawMessage, *SlurmConfMeta, error) {
	if err := c.requireAPIVersion(confMinAPIVersion, confMinSlurmVersion, "slurmctld configuration endpoint (/slurm/*/conf)"); err != nil {
		return nil, nil, err
	}
	data, err := c.doRequest(ctx, http.MethodGet, c.slurmPath("conf"), nil)
	if err != nil {
		return nil, nil, err
	}
	var resp slurmConfResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal slurm conf response: %w", err)
	}
	return resp.SlurmConf, resp.Meta, nil
}

// GetSlurmdbdConf returns the active slurmdbd configuration as a raw key →
// JSON-value map. Requires API v0.0.45+ (Slurm 26.05).
func (c *Client) GetSlurmdbdConf(ctx context.Context) (map[string]json.RawMessage, error) {
	if err := c.requireAPIVersion(confMinAPIVersion, confMinSlurmVersion, "slurmdbd configuration endpoint (/slurmdb/*/conf)"); err != nil {
		return nil, err
	}
	data, err := c.doRequest(ctx, http.MethodGet, c.slurmdbPath("conf"), nil)
	if err != nil {
		return nil, err
	}
	var resp slurmdbdConfResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal slurmdbd conf response: %w", err)
	}
	return resp.SlurmdbdConf, nil
}

// IsNotFound reports whether err is an APIError with HTTP status 404 —
// i.e. the endpoint does not exist in the configured API version.
func IsNotFound(err error) bool {
	apiErr, ok := err.(*APIError)
	return ok && apiErr.StatusCode == http.StatusNotFound
}

// StringifyConfValue renders one raw conf JSON value as a flat string so the
// whole configuration fits a Terraform map(string):
//
//   - string        → as-is
//   - number        → decimal ("6817")
//   - bool          → "true" / "false"
//   - null          → ""
//   - array         → elements stringified recursively, comma-joined
//   - {set,infinite,number} (Slurm's tri-state int):
//     unset → "", infinite → "infinite", else the number
//   - any other object → compact JSON
func StringifyConfValue(raw json.RawMessage) string {
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	return stringifyValue(v)
}

func stringifyValue(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	case float64:
		// JSON numbers decode as float64; render integers without ".0".
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%g", t)
	case []interface{}:
		parts := make([]string, 0, len(t))
		for _, e := range t {
			parts = append(parts, stringifyValue(e))
		}
		return strings.Join(parts, ",")
	case map[string]interface{}:
		if s, ok := stringifySlurmInt(t); ok {
			return s
		}
		// Unknown object shape — keep it inspectable as compact JSON with
		// deterministic key order.
		return compactJSON(t)
	default:
		return fmt.Sprintf("%v", t)
	}
}

// stringifySlurmInt renders Slurm's {set, infinite, number} tri-state object.
// It only claims the object when it has exactly that shape.
func stringifySlurmInt(m map[string]interface{}) (string, bool) {
	if len(m) != 3 {
		return "", false
	}
	set, okSet := m["set"].(bool)
	infinite, okInf := m["infinite"].(bool)
	number, okNum := m["number"].(float64)
	if !okSet || !okInf || !okNum {
		return "", false
	}
	switch {
	case infinite:
		return "infinite", true
	case !set:
		return "", true
	default:
		return stringifyValue(number), true
	}
}

// compactJSON marshals a map with sorted keys (encoding/json sorts map keys).
func compactJSON(m map[string]interface{}) string {
	b, err := json.Marshal(m)
	if err != nil {
		// Fall back to Go syntax with sorted keys for determinism.
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s:%v", k, m[k]))
		}
		return "{" + strings.Join(parts, ",") + "}"
	}
	return string(b)
}
