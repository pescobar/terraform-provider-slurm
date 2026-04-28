package resources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
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
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T", req.ProviderData),
		)
		return
	}
	r.client = c
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

	// Build the accounts_association request which atomically creates the account
	// metadata AND its cluster-level association with limits in one API call.
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

	assocSet := &client.AssocRecSet{}
	hasLimits := false

	if !plan.Fairshare.IsNull() && !plan.Fairshare.IsUnknown() {
		v := int(plan.Fairshare.ValueInt64())
		assocSet.Fairshare = &v
		hasLimits = true
	}
	if !plan.DefaultQOS.IsNull() && !plan.DefaultQOS.IsUnknown() {
		assocSet.DefaultQOS = plan.DefaultQOS.ValueString()
		hasLimits = true
	}
	if !plan.AllowedQOS.IsNull() && !plan.AllowedQOS.IsUnknown() {
		var qosList []string
		diags = plan.AllowedQOS.ElementsAs(ctx, &qosList, false)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		assocSet.QOSLevel = qosList
		hasLimits = true
	}
	if !plan.MaxJobs.IsNull() && !plan.MaxJobs.IsUnknown() {
		assocSet.MaxJobs = &client.SlurmInt{
			Number: int(plan.MaxJobs.ValueInt64()),
			Set:    true,
		}
		hasLimits = true
	}

	acctAssocReq := client.AccountAssociationRequest{
		AssociationCondition: client.AccountAssociationCondition{
			Accounts: []string{plan.Name.ValueString()},
			Clusters: []string{r.client.Cluster},
		},
		Account: acctShort,
	}
	if hasLimits {
		acctAssocReq.AssociationCondition.Association = assocSet
	}

	if err := r.client.CreateAccountWithAssociation(acctAssocReq); err != nil {
		resp.Diagnostics.AddError("Error creating account", err.Error())
		return
	}

	// accounts_association/ does not accept TRES limits; set them via associations/
	// in a second call if any are configured.
	if tresMax := r.extractAccountTRESMax(ctx, plan, &resp.Diagnostics); tresMax != nil {
		if resp.Diagnostics.HasError() {
			return
		}
		tresAssoc := client.Association{
			Account: plan.Name.ValueString(),
			Cluster: r.client.Cluster,
			User:    "",
			Max:     &client.AssociationMax{TRES: tresMax},
		}
		if err := r.client.CreateAssociations([]client.Association{tresAssoc}); err != nil {
			resp.Diagnostics.AddError("Error setting account TRES limits", err.Error())
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
			if assoc.Max != nil && assoc.Max.TRES != nil {
				t := assoc.Max.TRES
				if len(t.Total) > 0 && !state.GrpTRES.IsNull() {
					state.GrpTRES = apiTresListToSet(ctx, t.Total, &resp.Diagnostics)
				}
				if t.Group != nil {
					if len(t.Group.Minutes) > 0 && !state.GrpTRESMins.IsNull() {
						state.GrpTRESMins = apiTresListToSet(ctx, t.Group.Minutes, &resp.Diagnostics)
					}
					if len(t.Group.Active) > 0 && !state.GrpTRESRunMins.IsNull() {
						state.GrpTRESRunMins = apiTresListToSet(ctx, t.Group.Active, &resp.Diagnostics)
					}
				}
				if t.Per != nil {
					if len(t.Per.Job) > 0 && !state.MaxTRESPerJob.IsNull() {
						state.MaxTRESPerJob = apiTresListToSet(ctx, t.Per.Job, &resp.Diagnostics)
					}
					if len(t.Per.Node) > 0 && !state.MaxTRESPerNode.IsNull() {
						state.MaxTRESPerNode = apiTresListToSet(ctx, t.Per.Node, &resp.Diagnostics)
					}
				}
				if t.Minutes != nil && t.Minutes.Per != nil {
					if len(t.Minutes.Per.Job) > 0 && !state.MaxTRESMinsPerJob.IsNull() {
						state.MaxTRESMinsPerJob = apiTresListToSet(ctx, t.Minutes.Per.Job, &resp.Diagnostics)
					}
				}
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

	// Update account-level association limits via /associations/ (same endpoint
	// as user-level associations). Using accounts_association/ here causes a
	// race condition when user association updates run in parallel: Slurm drops
	// QOS entries from the account-level association. The /associations/ endpoint
	// serializes correctly alongside concurrent user association updates.
	assoc := client.Association{
		Account: plan.Name.ValueString(),
		Cluster: r.client.Cluster,
		User:    "",
	}
	hasLimits := false

	if !plan.Fairshare.IsNull() {
		v := int(plan.Fairshare.ValueInt64())
		assoc.SharesRaw = &v
		hasLimits = true
	}
	if !plan.DefaultQOS.IsNull() {
		assoc.Default = &client.AssociationDefaults{QOS: plan.DefaultQOS.ValueString()}
		hasLimits = true
	}
	if !plan.AllowedQOS.IsNull() {
		var qosList []string
		diags = plan.AllowedQOS.ElementsAs(ctx, &qosList, false)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		assoc.QOS = qosList
		hasLimits = true
	}
	if !plan.MaxJobs.IsNull() {
		assoc.Max = &client.AssociationMax{
			Jobs: &client.AssociationMaxJobs{
				Active: &client.SlurmInt{Number: int(plan.MaxJobs.ValueInt64()), Set: true},
			},
		}
		hasLimits = true
	}
	if tresMax := r.extractAccountTRESMax(ctx, plan, &resp.Diagnostics); tresMax != nil {
		if resp.Diagnostics.HasError() {
			return
		}
		if assoc.Max == nil {
			assoc.Max = &client.AssociationMax{}
		}
		assoc.Max.TRES = tresMax
		hasLimits = true
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
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// extractAccountTRESMax builds the TRES portion of AssociationMax from the plan.
// Returns nil when no TRES fields are set, so callers can skip the API call.
//
// API path mapping (v0.0.42):
//
//	MaxTRES        → max.tres.per.job
//	MaxTRESPerNode → max.tres.per.node
//	MaxTRESMins    → max.tres.minutes.per.job
//	GrpTRES        → max.tres.total
//	GrpTRESMins    → max.tres.group.minutes
//	GrpTRESRunMins → max.tres.group.active
func (r *accountResource) extractAccountTRESMax(ctx context.Context, plan accountResourceModel, diagnostics *diag.Diagnostics) *client.AssociationMaxTRES {
	var tres client.AssociationMaxTRES
	set := false

	if !plan.MaxTRESPerJob.IsNull() && !plan.MaxTRESPerJob.IsUnknown() {
		if list := planTresListToAPI(ctx, plan.MaxTRESPerJob); len(list) > 0 {
			if tres.Per == nil {
				tres.Per = &client.AssociationMaxTRESPer{}
			}
			tres.Per.Job = list
			set = true
		}
	}
	if !plan.MaxTRESPerNode.IsNull() && !plan.MaxTRESPerNode.IsUnknown() {
		if list := planTresListToAPI(ctx, plan.MaxTRESPerNode); len(list) > 0 {
			if tres.Per == nil {
				tres.Per = &client.AssociationMaxTRESPer{}
			}
			tres.Per.Node = list
			set = true
		}
	}
	if !plan.MaxTRESMinsPerJob.IsNull() && !plan.MaxTRESMinsPerJob.IsUnknown() {
		if list := planTresListToAPI(ctx, plan.MaxTRESMinsPerJob); len(list) > 0 {
			tres.Minutes = &client.AssociationMaxTRESMins{
				Per: &client.AssociationMaxTRESMinsPer{Job: list},
			}
			set = true
		}
	}
	if !plan.GrpTRES.IsNull() && !plan.GrpTRES.IsUnknown() {
		if list := planTresListToAPI(ctx, plan.GrpTRES); len(list) > 0 {
			tres.Total = list
			set = true
		}
	}
	if !plan.GrpTRESMins.IsNull() && !plan.GrpTRESMins.IsUnknown() {
		if list := planTresListToAPI(ctx, plan.GrpTRESMins); len(list) > 0 {
			if tres.Group == nil {
				tres.Group = &client.AssociationMaxTRESGroup{}
			}
			tres.Group.Minutes = list
			set = true
		}
	}
	if !plan.GrpTRESRunMins.IsNull() && !plan.GrpTRESRunMins.IsUnknown() {
		if list := planTresListToAPI(ctx, plan.GrpTRESRunMins); len(list) > 0 {
			if tres.Group == nil {
				tres.Group = &client.AssociationMaxTRESGroup{}
			}
			tres.Group.Active = list
			set = true
		}
	}

	if !set {
		return nil
	}
	return &tres
}
