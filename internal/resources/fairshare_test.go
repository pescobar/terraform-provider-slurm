package resources

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestSharesRawFromFairshare(t *testing.T) {
	cases := []struct {
		name string
		in   types.String
		want *int
	}{
		{"null", types.StringNull(), nil},
		{"unknown", types.StringUnknown(), nil},
		{"zero", types.StringValue("0"), intPtr(0)},
		{"weight", types.StringValue("42"), intPtr(42)},
		{"parent", types.StringValue(fairshareParentKeyword), intPtr(fairshareParentSentinel)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := sharesRawFromFairshare(c.in)
			switch {
			case c.want == nil && got != nil:
				t.Fatalf("got %d, want nil", *got)
			case c.want != nil && got == nil:
				t.Fatalf("got nil, want %d", *c.want)
			case c.want != nil && *got != *c.want:
				t.Fatalf("got %d, want %d", *got, *c.want)
			}
		})
	}
}

func TestFairshareStringFromSharesRaw(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{0, "0"},
		{42, "42"},
		{fairshareParentSentinel, fairshareParentKeyword},
	}
	for _, c := range cases {
		if got := fairshareStringFromSharesRaw(c.in); got != c.want {
			t.Errorf("fairshareStringFromSharesRaw(%d): got %q, want %q", c.in, got, c.want)
		}
	}
}

// Round-trip: every value the provider writes must read back identically.
func TestFairshareRoundTrip(t *testing.T) {
	for _, s := range []string{"0", "1", "42", fairshareParentKeyword} {
		raw := sharesRawFromFairshare(types.StringValue(s))
		if raw == nil {
			t.Fatalf("%q: unexpected nil shares_raw", s)
		}
		if back := fairshareStringFromSharesRaw(*raw); back != s {
			t.Errorf("%q round-tripped to %q", s, back)
		}
	}
}

func TestFairshareValidator(t *testing.T) {
	v := fairshareValidator{}
	valid := []string{"parent", "0", "1", "1000", "2147483646"}
	for _, s := range valid {
		if !runStringValidator(v, s) {
			t.Errorf("expected %q to be valid", s)
		}
	}
	invalid := []string{"", "-1", "abc", "1.5", "PARENT", "2147483647"}
	for _, s := range invalid {
		if runStringValidator(v, s) {
			t.Errorf("expected %q to be rejected", s)
		}
	}
}
