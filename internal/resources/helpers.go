package resources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
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

// buildAssocMaxTRES builds the *client.AssociationMaxTRES portion of an
// association from the six TRES fields shared by slurm_account and the per-
// association blocks of slurm_user. Returns nil when every field is null,
// unknown, or empty so the parent struct's omitempty drops the whole node.
//
// Mapping (sacctmgr → API JSON path):
//
//	MaxTRES        → max.tres.per.job
//	MaxTRESPerNode → max.tres.per.node
//	MaxTRESMins    → max.tres.minutes.per.job
//	GrpTRES        → max.tres.total
//	GrpTRESMins    → max.tres.group.minutes
//	GrpTRESRunMins → max.tres.group.active
func buildAssocMaxTRES(ctx context.Context, perJob, perNode, minsPerJob, grpTotal, grpMins, grpRunMins types.Set) *client.AssociationMaxTRES {
	perJobL := planTresListToAPI(ctx, perJob)
	perNodeL := planTresListToAPI(ctx, perNode)
	minsPerJobL := planTresListToAPI(ctx, minsPerJob)
	grpTotalL := planTresListToAPI(ctx, grpTotal)
	grpMinsL := planTresListToAPI(ctx, grpMins)
	grpRunMinsL := planTresListToAPI(ctx, grpRunMins)

	if len(perJobL)+len(perNodeL)+len(minsPerJobL)+len(grpTotalL)+len(grpMinsL)+len(grpRunMinsL) == 0 {
		return nil
	}

	out := &client.AssociationMaxTRES{Total: grpTotalL}
	if len(grpMinsL) > 0 || len(grpRunMinsL) > 0 {
		out.Group = &client.AssociationMaxTRESGroup{
			Minutes: grpMinsL,
			Active:  grpRunMinsL,
		}
	}
	if len(perJobL) > 0 || len(perNodeL) > 0 {
		out.Per = &client.AssociationMaxTRESPer{
			Job:  perJobL,
			Node: perNodeL,
		}
	}
	if len(minsPerJobL) > 0 {
		out.Minutes = &client.AssociationMaxTRESMins{
			Per: &client.AssociationMaxTRESMinsPer{Job: minsPerJobL},
		}
	}
	return out
}

// assocMaxTRESSnapshot is the result of converting the API-side
// AssociationMaxTRES tree into a flat set of types.Set values, one per logical
// TRES field. Fields with no API data are types.SetNull(tresElemType()) so
// callers can use IsNull() to decide whether to commit to state. Field names
// match the underlying sacctmgr / API semantics (Grp* are group-aggregate,
// Max* are per-job).
type assocMaxTRESSnapshot struct {
	GrpTotal      types.Set // max.tres.total              (GrpTRES)
	GrpMins       types.Set // max.tres.group.minutes      (GrpTRESMins)
	GrpRunMins    types.Set // max.tres.group.active       (GrpTRESRunMins)
	MaxPerJob     types.Set // max.tres.per.job            (MaxTRES)
	MaxPerNode    types.Set // max.tres.per.node           (MaxTRESPerNode)
	MaxMinsPerJob types.Set // max.tres.minutes.per.job    (MaxTRESMins)
}

// snapshotAssocMaxTRES walks an API-side *client.AssociationMax and converts
// its TRES sub-tree into the flat snapshot used by both slurm_account.Read and
// slurm_user.apiAssociationsToState. Empty/missing API lists become null sets.
func snapshotAssocMaxTRES(ctx context.Context, max *client.AssociationMax, diags *diag.Diagnostics) assocMaxTRESSnapshot {
	null := types.SetNull(tresElemType())
	snap := assocMaxTRESSnapshot{
		GrpTotal:      null,
		GrpMins:       null,
		GrpRunMins:    null,
		MaxPerJob:     null,
		MaxPerNode:    null,
		MaxMinsPerJob: null,
	}
	if max == nil || max.TRES == nil {
		return snap
	}
	t := max.TRES
	if len(t.Total) > 0 {
		snap.GrpTotal = apiTresListToSet(ctx, t.Total, diags)
	}
	if t.Group != nil {
		if len(t.Group.Minutes) > 0 {
			snap.GrpMins = apiTresListToSet(ctx, t.Group.Minutes, diags)
		}
		if len(t.Group.Active) > 0 {
			snap.GrpRunMins = apiTresListToSet(ctx, t.Group.Active, diags)
		}
	}
	if t.Per != nil {
		if len(t.Per.Job) > 0 {
			snap.MaxPerJob = apiTresListToSet(ctx, t.Per.Job, diags)
		}
		if len(t.Per.Node) > 0 {
			snap.MaxPerNode = apiTresListToSet(ctx, t.Per.Node, diags)
		}
	}
	if t.Minutes != nil && t.Minutes.Per != nil && len(t.Minutes.Per.Job) > 0 {
		snap.MaxMinsPerJob = apiTresListToSet(ctx, t.Minutes.Per.Job, diags)
	}
	return snap
}
