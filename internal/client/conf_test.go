package client

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRequireAPIVersion(t *testing.T) {
	tests := []struct {
		name       string
		apiVersion string
		wantErr    bool
	}{
		{"older version rejected", "v0.0.42", true},
		{"previous version rejected", "v0.0.44", true},
		{"minimum version accepted", "v0.0.45", false},
		{"newer version accepted", "v0.0.46", false},
		// Unparsable versions must pass — the server stays the authority.
		{"garbage passes through", "latest", false},
		{"non-numeric suffix passes through", "v0.0.x", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Client{APIVersion: tt.apiVersion}
			err := c.requireAPIVersion(confMinAPIVersion, confMinSlurmVersion, "test endpoint")
			if (err != nil) != tt.wantErr {
				t.Fatalf("requireAPIVersion with %q: err = %v, wantErr %v", tt.apiVersion, err, tt.wantErr)
			}
			if err != nil {
				// The message must name both the configured and required versions
				// so the user can act on it without reading provider source.
				for _, needle := range []string{tt.apiVersion, "v0.0.45", "26.05", "test endpoint"} {
					if !strings.Contains(err.Error(), needle) {
						t.Errorf("error %q does not mention %q", err.Error(), needle)
					}
				}
			}
		})
	}
}

func TestStringifyConfValue(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"string", `"linux"`, "linux"},
		{"empty string", `""`, ""},
		{"integer", `6817`, "6817"},
		{"float", `1.5`, "1.5"},
		{"bool true", `true`, "true"},
		{"bool false", `false`, "false"},
		{"null", `null`, ""},
		{"string array", `["cpu","mem","node"]`, "cpu,mem,node"},
		{"empty array", `[]`, ""},
		{"slurmint set", `{"set":true,"infinite":false,"number":120}`, "120"},
		{"slurmint unset", `{"set":false,"infinite":false,"number":0}`, ""},
		{"slurmint infinite", `{"set":false,"infinite":true,"number":0}`, "infinite"},
		// Objects that merely look close to a SlurmInt must not be claimed.
		{"other object", `{"set":true,"number":1,"extra":"x"}`, `{"extra":"x","number":1,"set":true}`},
		{"nested array of slurmint", `[{"set":true,"infinite":false,"number":3}]`, "3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StringifyConfValue(json.RawMessage(tt.raw))
			if got != tt.want {
				t.Errorf("StringifyConfValue(%s) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
