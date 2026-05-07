package resources

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/pescobar/terraform-provider-slurm/internal/client"
)

var (
	_ resource.Resource                = &accountResource{}
	_ resource.ResourceWithImportState = &accountResource{}
)

type accountResource struct {
	client *client.Client
}

type accountResourceModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Description  types.String `tfsdk:"description"`
	Organization types.String `tfsdk:"organization"`
	Parent       types.String `tfsdk:"parent_account"`
	// Account-level association attributes (direct on the account)
	Fairshare  types.Int64  `tfsdk:"fairshare"`
	DefaultQOS types.String `tfsdk:"default_qos"`
	AllowedQOS types.List   `tfsdk:"allowed_qos"`
	MaxJobs    types.Int64  `tfsdk:"max_jobs"`
	// TRES limits on the account-level association
	MaxTRESPerJob     types.Set `tfsdk:"max_tres_per_job"`
	MaxTRESPerNode    types.Set `tfsdk:"max_tres_per_node"`
	MaxTRESMinsPerJob types.Set `tfsdk:"max_tres_mins_per_job"`
	GrpTRES           types.Set `tfsdk:"grp_tres"`
	GrpTRESMins       types.Set `tfsdk:"grp_tres_mins"`
	GrpTRESRunMins    types.Set `tfsdk:"grp_tres_run_mins"`
}

func NewAccountResource() resource.Resource {
	return &accountResource{}
}

func (r *accountResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_account"
}

func (r *accountResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Slurm account and its account-level association (limits, QOS, fairshare).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The account name (same as name).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the Slurm account.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"description": schema.StringAttribute{
				Description: "A description of the account.",
				Optional:    true,
			},
			"organization": schema.StringAttribute{
				Description: "The organization this account belongs to.",
				Optional:    true,
			},
			"parent_account": schema.StringAttribute{
				Description: "The parent account name. Defaults to 'root'.",
				Optional:    true,
			},
			// Account-level association attributes
			"fairshare": schema.Int64Attribute{
				Description: "Fairshare value for this account's association.",
				Optional:    true,
				Validators:  []validator.Int64{int64validator.AtLeast(0)},
			},
			"default_qos": schema.StringAttribute{
				Description: "Default QOS for this account's association.",
				Optional:    true,
			},
			"allowed_qos": schema.ListAttribute{
				Description: "List of allowed QOS names for this account.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"max_jobs": schema.Int64Attribute{
				Description: "Maximum number of running jobs for this account's association (inherited by users unless overridden).",
				Optional:    true,
				Validators:  []validator.Int64{int64validator.AtLeast(0)},
			},
			"max_tres_per_job":      tresOptionalSchemaAttr("Maximum TRES per job for this account's association (MaxTRES)."),
			"max_tres_per_node":     tresOptionalSchemaAttr("Maximum TRES per node per job for this account's association (MaxTRESPerNode)."),
			"max_tres_mins_per_job": tresOptionalSchemaAttr("Maximum TRES-minutes per job for this account's association (MaxTRESMins)."),
			"grp_tres":              tresOptionalSchemaAttr("Maximum TRES in use at once across this account's group (GrpTRES)."),
			"grp_tres_mins":         tresOptionalSchemaAttr("Maximum TRES-minutes for this account's group (GrpTRESMins)."),
			"grp_tres_run_mins":     tresOptionalSchemaAttr("Maximum TRES-minutes of currently running jobs for this account's group (GrpTRESRunMins)."),
		},
	}
}

func (r *accountResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c := configureClient(req, resp); c != nil {
		r.client = c
	}
}

func (r *accountResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan accountResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating account", map[string]interface{}{
		"name": plan.Name.ValueString(),
	})

	// Step 1: atomically create the account entity + an empty cluster-level
	// association via /accounts_association/. No limits are sent here because
	// that endpoint can race with parallel user association updates and drop
	// QOS entries from the account-level association.
	acctShort := client.AccountShort{}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		acctShort.Description = plan.Description.ValueString()
	}
	if !plan.Organization.IsNull() && !plan.Organization.IsUnknown() {
		acctShort.Organization = plan.Organization.ValueString()
	}
	if !plan.Parent.IsNull() && !plan.Parent.IsUnknown() {
		acctShort.Parent = plan.Parent.ValueString()
	}

	acctAssocReq := client.AccountAssociationRequest{
		AssociationCondition: client.AccountAssociationCondition{
			Accounts: []string{plan.Name.ValueString()},
			Clusters: []string{r.client.Cluster},
		},
		Account: acctShort,
	}
	if err := r.client.CreateAccountWithAssociation(acctAssocReq); err != nil {
		resp.Diagnostics.AddError("Error creating account", err.Error())
		return
	}

	// Step 2: write all limits (flat + TRES) via /associations/ in one call.
	// This is the same path Update takes, so Create and Update produce
	// byte-identical JSON for the cluster-level association.
	assoc, hasLimits := buildAccountAssociation(ctx, plan, r.client.Cluster, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if hasLimits {
		if err := r.client.CreateAssociations([]client.Association{assoc}); err != nil {
			resp.Diagnostics.AddError("Error setting account association limits", err.Error())
			return
		}
	}

	plan.ID = plan.Name

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func (r *accountResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state accountResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Read account metadata
	account, err := r.client.GetAccount(state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading account", err.Error())
		return
	}
	if account == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	state.Name = types.StringValue(account.Name)
	state.ID = types.StringValue(account.Name)
	if !state.Description.IsNull() {
		state.Description = types.StringValue(account.Description)
	}
	if !state.Organization.IsNull() {
		state.Organization = types.StringValue(account.Organization)
	}
	if account.ParentAccount != "" && !state.Parent.IsNull() {
		state.Parent = types.StringValue(account.ParentAccount)
	}

	// Read account-level association.
	// The singular /association/ endpoint does not return account-level entries
	// (user=""); use the plural /associations/ endpoint and filter instead.
	assocResp, err := r.client.GetAssociations(map[string]string{
		"account": account.Name,
		"cluster": r.client.Cluster,
	})
	if err != nil {
		tflog.Warn(ctx, "Could not read account associations", map[string]interface{}{
			"account": account.Name,
			"error":   err.Error(),
		})
	} else {
		for _, assoc := range assocResp.Associations {
			if assoc.User != "" {
				continue // skip user-level associations
			}
			// Null-preservation: only update Optional fields from the API when
			// the current state already has a non-null value. This prevents
			// Slurm's inherited defaults (fairshare=1, QOS=["normal"], etc.)
			// from appearing in state when the config doesn't set those fields,
			// which would otherwise cause perpetual drift after import.
			if assoc.SharesRaw != nil && !state.Fairshare.IsNull() {
				state.Fairshare = types.Int64Value(int64(*assoc.SharesRaw))
			}
			if assoc.Default != nil && assoc.Default.QOS != "" && !state.DefaultQOS.IsNull() {
				state.DefaultQOS = types.StringValue(assoc.Default.QOS)
			}
			if len(assoc.QOS) > 0 && !state.AllowedQOS.IsNull() {
				qosValues, diags := types.ListValueFrom(ctx, types.StringType, assoc.QOS)
				resp.Diagnostics.Append(diags...)
				state.AllowedQOS = qosValues
			}
			// max_jobs maps to max.jobs.active (MaxJobs), not max.jobs.per.count (GrpJobs).
			if assoc.Max != nil && assoc.Max.Jobs != nil &&
				assoc.Max.Jobs.Active != nil && assoc.Max.Jobs.Active.Set &&
				!state.MaxJobs.IsNull() {
				state.MaxJobs = types.Int64Value(int64(assoc.Max.Jobs.Active.Number))
			}
			// TRES limits — null-preservation: only update when prior state is non-null.
			tres := snapshotAssocMaxTRES(ctx, assoc.Max, &resp.Diagnostics)
			if !state.GrpTRES.IsNull() && !tres.GrpTotal.IsNull() {
				state.GrpTRES = tres.GrpTotal
			}
			if !state.GrpTRESMins.IsNull() && !tres.GrpMins.IsNull() {
				state.GrpTRESMins = tres.GrpMins
			}
			if !state.GrpTRESRunMins.IsNull() && !tres.GrpRunMins.IsNull() {
				state.GrpTRESRunMins = tres.GrpRunMins
			}
			if !state.MaxTRESPerJob.IsNull() && !tres.MaxPerJob.IsNull() {
				state.MaxTRESPerJob = tres.MaxPerJob
			}
			if !state.MaxTRESPerNode.IsNull() && !tres.MaxPerNode.IsNull() {
				state.MaxTRESPerNode = tres.MaxPerNode
			}
			if !state.MaxTRESMinsPerJob.IsNull() && !tres.MaxMinsPerJob.IsNull() {
				state.MaxTRESMinsPerJob = tres.MaxMinsPerJob
			}
			break
		}
	}

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

func (r *accountResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan accountResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Updating account", map[string]interface{}{
		"name": plan.Name.ValueString(),
	})

	// Update account metadata via /accounts/ endpoint.
	account := client.Account{
		Name: plan.Name.ValueString(),
	}
	if !plan.Description.IsNull() {
		account.Description = plan.Description.ValueString()
	}
	if !plan.Organization.IsNull() {
		account.Organization = plan.Organization.ValueString()
	}
	if !plan.Parent.IsNull() {
		account.ParentAccount = plan.Parent.ValueString()
	}

	if err := r.client.CreateAccount(account); err != nil {
		resp.Diagnostics.AddError("Error updating account", err.Error())
		return
	}

	// Update the cluster-level association limits via /associations/. Same
	// path as Create — see buildAccountAssociation for why we don't use
	// /accounts_association/ here.
	assoc, hasLimits := buildAccountAssociation(ctx, plan, r.client.Cluster, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if hasLimits {
		if err := r.client.CreateAssociations([]client.Association{assoc}); err != nil {
			resp.Diagnostics.AddError("Error updating account association", err.Error())
			return
		}
	}

	plan.ID = plan.Name

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func (r *accountResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state accountResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Deleting account", map[string]interface{}{
		"name": state.Name.ValueString(),
	})

	if err := r.client.DeleteAccount(state.Name.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting account", err.Error())
		return
	}
}

func (r *accountResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importStateByName(ctx, req, resp)
}

// buildAccountAssociation constructs the cluster-level *client.Association
// for a slurm_account from its model. Identity fields (User="", Account,
// Cluster) are always populated; limit fields are filled only when the
// corresponding plan attribute is non-null.
//
// The boolean is true when at least one limit field is set, so callers can
// skip the CreateAssociations API call when there is nothing to send.
//
// Used by both Create and Update so the two paths produce byte-identical
// JSON for the /associations/ endpoint — the previous Create path used the
// /accounts_association/ endpoint with a different shape (AssocRecSet) and
// could race against parallel user-association updates (Slurm dropped QOS
// entries from the account-level association).
func buildAccountAssociation(ctx context.Context, plan accountResourceModel, cluster string, diags *diag.Diagnostics) (client.Association, bool) {
	assoc := client.Association{
		Account: plan.Name.ValueString(),
		Cluster: cluster,
		User:    "",
	}
	hasLimits := false

	if v := intPtrFromInt64(plan.Fairshare); v != nil {
		assoc.SharesRaw = v
		hasLimits = true
	}
	if !plan.DefaultQOS.IsNull() && !plan.DefaultQOS.IsUnknown() {
		assoc.Default = &client.AssociationDefaults{QOS: plan.DefaultQOS.ValueString()}
		hasLimits = true
	}
	if !plan.AllowedQOS.IsNull() && !plan.AllowedQOS.IsUnknown() {
		var qosList []string
		diags.Append(plan.AllowedQOS.ElementsAs(ctx, &qosList, false)...)
		assoc.QOS = qosList
		hasLimits = true
	}
	if v := slurmIntFromInt64(plan.MaxJobs); v != nil {
		assoc.Max = &client.AssociationMax{
			Jobs: &client.AssociationMaxJobs{Active: v},
		}
		hasLimits = true
	}
	if tresMax := buildAssocMaxTRES(ctx,
		plan.MaxTRESPerJob, plan.MaxTRESPerNode, plan.MaxTRESMinsPerJob,
		plan.GrpTRES, plan.GrpTRESMins, plan.GrpTRESRunMins,
	); tresMax != nil {
		if assoc.Max == nil {
			assoc.Max = &client.AssociationMax{}
		}
		assoc.Max.TRES = tresMax
		hasLimits = true
	}

	return assoc, hasLimits
}
