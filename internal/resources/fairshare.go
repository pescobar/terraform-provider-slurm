package resources

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Fairshare is exposed as a string attribute because Slurm's fairshare has two
// distinct kinds of value that don't share a numeric type: an ordinary weight
// (a non-negative integer) and the special "parent" mode, in which the
// association inherits its parent account's fairshare weight instead of
// carrying its own.
//
// Slurm stores "parent" as shares_raw = INT32_MAX (SLURMDB_FS_USE_PARENT), and
// the REST API surfaces it as exactly that number with no accompanying flag —
// verified identical across Slurm 25.05 / 25.11 / 26.05. Detecting parent mode
// by numeric equality with the sentinel is therefore safe: the value cannot
// mean anything else.
const (
	fairshareParentSentinel = 2147483647
	fairshareParentKeyword  = "parent"
)

// sharesRawFromFairshare converts a fairshare string attribute into Slurm's
// shares_raw *int. The keyword "parent" maps to the sentinel; every other
// value is a base-10 integer. Returns nil when the attribute is null or
// unknown so callers can rely on the parent struct's omitempty tag. Assumes
// the value has already passed fairshareValidator.
func sharesRawFromFairshare(v types.String) *int {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	if v.ValueString() == fairshareParentKeyword {
		n := fairshareParentSentinel
		return &n
	}
	n, err := strconv.Atoi(v.ValueString())
	if err != nil {
		return nil // unreachable once validated
	}
	return &n
}

// fairshareStringFromSharesRaw is the inverse of sharesRawFromFairshare: it
// renders a shares_raw value read back from Slurm into the string form,
// mapping the parent sentinel to "parent".
func fairshareStringFromSharesRaw(n int) string {
	if n == fairshareParentSentinel {
		return fairshareParentKeyword
	}
	return strconv.Itoa(n)
}

// fairshareValidator accepts either the keyword "parent" or a base-10 integer
// in [0, fairshareParentSentinel). It replaces the int64validator.AtLeast(0)
// used while fairshare was an Int64 attribute. The literal sentinel number is
// rejected on purpose: it canonicalises to "parent" on read, so allowing it in
// config would produce perpetual drift.
type fairshareValidator struct{}

var _ validator.String = fairshareValidator{}

func (fairshareValidator) Description(_ context.Context) string {
	return `"parent" or a non-negative integer`
}

func (v fairshareValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (fairshareValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	s := req.ConfigValue.ValueString()
	if s == fairshareParentKeyword {
		return
	}
	n, err := strconv.Atoi(s)
	switch {
	case err != nil || n < 0:
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid fairshare value",
			fmt.Sprintf("fairshare must be %q or a non-negative integer, got: %q", fairshareParentKeyword, s),
		)
	case n >= fairshareParentSentinel:
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid fairshare value",
			fmt.Sprintf("%d is Slurm's internal sentinel for parent mode; set fairshare = %q instead", fairshareParentSentinel, fairshareParentKeyword),
		)
	}
}
