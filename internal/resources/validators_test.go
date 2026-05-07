package resources

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// runStringValidator invokes a string validator against a single value and
// returns whether it passed (no diagnostics).
func runStringValidator(v validator.String, value string) bool {
	req := validator.StringRequest{
		Path:        path.Root("test"),
		ConfigValue: types.StringValue(value),
	}
	resp := &validator.StringResponse{}
	v.ValidateString(context.Background(), req, resp)
	return !resp.Diagnostics.HasError()
}

// runInt64Validator invokes an int64 validator and returns pass/fail.
func runInt64Validator(v validator.Int64, value int64) bool {
	req := validator.Int64Request{
		Path:        path.Root("test"),
		ConfigValue: types.Int64Value(value),
	}
	resp := &validator.Int64Response{}
	v.ValidateInt64(context.Background(), req, resp)
	return !resp.Diagnostics.HasError()
}

// runSetValidator invokes a set validator against a set of strings.
func runSetValidator(t *testing.T, v validator.Set, values ...string) bool {
	t.Helper()
	elems := make([]string, len(values))
	copy(elems, values)
	setVal, diags := types.SetValueFrom(context.Background(), types.StringType, elems)
	if diags.HasError() {
		t.Fatalf("SetValueFrom: %v", diags)
	}
	req := validator.SetRequest{
		Path:        path.Root("test"),
		ConfigValue: setVal,
	}
	resp := &validator.SetResponse{}
	v.ValidateSet(context.Background(), req, resp)
	return !resp.Diagnostics.HasError()
}

// ---------------------------------------------------------------------------
// admin_level — slurm_user
// ---------------------------------------------------------------------------

func TestAdminLevelValidator(t *testing.T) {
	v := stringvalidator.OneOf("None", "Operator", "Administrator")
	tests := []struct {
		value string
		valid bool
	}{
		{"None", true},
		{"Operator", true},
		{"Administrator", true},
		{"Sudo", false},                    // unknown level
		{"none", false},                    // case-sensitive
		{"administrator", false},           // case-sensitive
		{"", false},                        // empty rejected when set explicitly
	}
	for _, tc := range tests {
		t.Run(tc.value, func(t *testing.T) {
			got := runStringValidator(v, tc.value)
			if got != tc.valid {
				t.Errorf("OneOf admin_level %q: got pass=%v, want pass=%v", tc.value, got, tc.valid)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// QOS enum lists
// ---------------------------------------------------------------------------

func TestQOSFlagValues_NotEmpty(t *testing.T) {
	if len(qosFlagValues) == 0 {
		t.Fatal("qosFlagValues must not be empty — validator would reject every flag")
	}
	// Spot-check a few well-known values that have shipped for years.
	want := map[string]bool{
		"NO_RESERVE":             false,
		"OVERRIDE_PARTITION_QOS": false,
		"DENY_LIMIT":             false,
	}
	for _, f := range qosFlagValues {
		if _, ok := want[f]; ok {
			want[f] = true
		}
	}
	for k, ok := range want {
		if !ok {
			t.Errorf("qosFlagValues missing well-known flag %q", k)
		}
	}
}

func TestQOSFlagSetValidator(t *testing.T) {
	v := setvalidator.ValueStringsAre(stringvalidator.OneOf(qosFlagValues...))
	if !runSetValidator(t, v, "NO_RESERVE", "DENY_LIMIT") {
		t.Error("expected pass for valid flag set")
	}
	if runSetValidator(t, v, "NO_RESERVE", "MADE_UP_FLAG") {
		t.Error("expected reject when one element is invalid")
	}
}

func TestQOSPreemptModeSetValidator(t *testing.T) {
	v := setvalidator.ValueStringsAre(stringvalidator.OneOf(qosPreemptModeValues...))
	if !runSetValidator(t, v, "CANCEL", "REQUEUE") {
		t.Error("expected pass for valid preempt_mode set")
	}
	if runSetValidator(t, v, "PANIC") {
		t.Error("expected reject for invalid preempt_mode")
	}
	if runSetValidator(t, v, "cancel") {
		t.Error("expected reject for lowercase preempt_mode (case-sensitive)")
	}
}

// ---------------------------------------------------------------------------
// AtLeast(0) — applied across most numeric attributes
// ---------------------------------------------------------------------------

func TestAtLeastZero(t *testing.T) {
	v := int64validator.AtLeast(0)
	tests := []struct {
		value int64
		valid bool
	}{
		{0, true},
		{1, true},
		{1_000_000, true},
		{-1, false},
		{-100, false},
	}
	for _, tc := range tests {
		got := runInt64Validator(v, tc.value)
		if got != tc.valid {
			t.Errorf("AtLeast(0) %d: got pass=%v, want pass=%v", tc.value, got, tc.valid)
		}
	}
}
