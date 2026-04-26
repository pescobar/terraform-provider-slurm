package resources

import (
	"context"
	"fmt"

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
				Computed:    true,
			},
			"organization": schema.StringAttribute{
				Description: "The organization this account belongs to.",
				Optional:    true,
				Computed:    true,
			},
			"parent_account": schema.StringAttribute{
				Description: "The parent account name. Defaults to 'root'.",
				Optional:    true,
				Computed:    true,
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

	plan.ID = plan.Name

	// Read back from API to resolve computed fields (parent_account, description, organization)
	// that are Optional+Computed and may still be unknown if not set in config.
	created, err := r.client.GetAccount(plan.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading account after create", err.Error())
		return
	}
	if created != nil {
		plan.Description = types.StringValue(created.Description)
		plan.Organization = types.StringValue(created.Organization)
		if created.ParentAccount != "" {
			plan.Parent = types.StringValue(created.ParentAccount)
		} else if plan.Parent.IsUnknown() {
			// Not set by user and API returned empty — resolve the unknown to null.
			plan.Parent = types.StringNull()
		}
		// If user set parent_account but API returned empty, keep the planned
		// value — Slurm accepted the parent but may not echo it back immediately.
	}

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
	state.Description = types.StringValue(account.Description)
	state.Organization = types.StringValue(account.Organization)
	if account.ParentAccount != "" {
		state.Parent = types.StringValue(account.ParentAccount)
	}

	// Read account-level association
	assoc, err := r.client.GetAssociation(account.Name, "", r.client.Cluster, "")
	if err != nil {
		// Association might not exist yet, that's ok
		tflog.Warn(ctx, "Could not read account association", map[string]interface{}{
			"account": account.Name,
			"error":   err.Error(),
		})
	} else if assoc != nil {
		if assoc.SharesRaw != nil {
			state.Fairshare = types.Int64Value(int64(*assoc.SharesRaw))
		}
		if assoc.Default != nil && assoc.Default.QOS != "" {
			state.DefaultQOS = types.StringValue(assoc.Default.QOS)
		}
		if len(assoc.QOS) > 0 {
			qosValues, diags := types.ListValueFrom(ctx, types.StringType, assoc.QOS)
			resp.Diagnostics.Append(diags...)
			state.AllowedQOS = qosValues
		}
		if assoc.Max != nil && assoc.Max.Jobs != nil && assoc.Max.Jobs.Per != nil &&
			assoc.Max.Jobs.Per.Count != nil && assoc.Max.Jobs.Per.Count.Set {
			state.MaxJobs = types.Int64Value(int64(assoc.Max.Jobs.Per.Count.Number))
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

	// Update account metadata
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

	// Update account-level association limits via accounts_association (upsert).
	assocSet := &client.AssocRecSet{}
	if !plan.Fairshare.IsNull() {
		v := int(plan.Fairshare.ValueInt64())
		assocSet.Fairshare = &v
	}
	if !plan.DefaultQOS.IsNull() {
		assocSet.DefaultQOS = plan.DefaultQOS.ValueString()
	}
	if !plan.AllowedQOS.IsNull() {
		var qosList []string
		diags = plan.AllowedQOS.ElementsAs(ctx, &qosList, false)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		assocSet.QOSLevel = qosList
	}
	if !plan.MaxJobs.IsNull() {
		assocSet.MaxJobs = &client.SlurmInt{
			Number: int(plan.MaxJobs.ValueInt64()),
			Set:    true,
		}
	}

	updateReq := client.AccountAssociationRequest{
		AssociationCondition: client.AccountAssociationCondition{
			Accounts:    []string{plan.Name.ValueString()},
			Clusters:    []string{r.client.Cluster},
			Association: assocSet,
		},
		Account: client.AccountShort{},
	}
	if err := r.client.CreateAccountWithAssociation(updateReq); err != nil {
		resp.Diagnostics.AddError("Error updating account association", err.Error())
		return
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
