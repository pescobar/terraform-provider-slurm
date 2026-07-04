package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestResolveBoolConfigValue(t *testing.T) {
	const envVar = "SLURM_INSECURE_SKIP_SSL_VERIFY_TEST"
	ctx := context.Background()

	tests := []struct {
		name       string
		configured types.Bool
		envValue   string // "" means unset
		defaultVal bool
		want       bool
	}{
		{"config true wins over env", types.BoolValue(true), "false", false, true},
		{"config false wins over env", types.BoolValue(false), "true", true, false},
		{"null config falls back to env true", types.BoolNull(), "true", false, true},
		{"null config falls back to env false", types.BoolNull(), "false", true, false},
		{"null config, env accepts 1/0", types.BoolNull(), "1", false, true},
		{"null config, no env, uses default true", types.BoolNull(), "", true, true},
		{"null config, no env, uses default false", types.BoolNull(), "", false, false},
		{"null config, unparsable env falls back to default", types.BoolNull(), "not-a-bool", true, true},
		{"unknown config treated like null", types.BoolUnknown(), "true", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(envVar, tt.envValue)
			got := resolveBoolConfigValue(ctx, tt.configured, envVar, tt.defaultVal)
			if got != tt.want {
				t.Errorf("resolveBoolConfigValue(%v, env=%q, default=%v) = %v, want %v",
					tt.configured, tt.envValue, tt.defaultVal, got, tt.want)
			}
		})
	}
}
