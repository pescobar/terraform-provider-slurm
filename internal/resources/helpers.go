package resources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/pescobar/terraform-provider-slurm/internal/client"
)

// slurmIntFromInt64 converts a Terraform Int64 attribute into Slurm's
// {number, set, infinite} representation. Returns nil when the attribute is
// null or unknown so callers can rely on the parent struct's `omitempty` tag.
func slurmIntFromInt64(v types.Int64) *client.SlurmInt {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	return &client.SlurmInt{Number: int(v.ValueInt64()), Set: true}
}

// intPtrFromInt64 converts a Terraform Int64 attribute into a *int suitable
// for fields the API exposes as a plain integer (e.g. Association.SharesRaw).
// Returns nil when the attribute is null or unknown.
func intPtrFromInt64(v types.Int64) *int {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	n := int(v.ValueInt64())
	return &n
}

// configureClient extracts the *client.Client passed in via ProviderData by
// the framework. Adds a diagnostic and returns nil when ProviderData is the
// wrong type (which is a programming error, not a user error). Returns nil
// without diagnostics when ProviderData is nil — the framework calls Configure
// twice and the first call has no data.
func configureClient(req resource.ConfigureRequest, resp *resource.ConfigureResponse) *client.Client {
	if req.ProviderData == nil {
		return nil
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T", req.ProviderData),
		)
		return nil
	}
	return c
}

// importStateByName implements the standard "import by name" handler used by
// every resource in this provider. It writes req.ID to both `name` and `id`
// state attributes so a subsequent Read can locate the entity in Slurm.
func importStateByName(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
