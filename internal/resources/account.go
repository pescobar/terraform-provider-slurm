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
	Fairshare  types.Int64    `tfsdk:"fairshare"`
	DefaultQOS types.String   `tfsdk:"default_qos"`
	AllowedQOS types.List     `tfsdk:"allowed_qos"`
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

	// Step 1: Create the account
	account := client.Account{
		Name: plan.Name.ValueString(),
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		account.Description = plan.Description.ValueString()
	}
	if !plan.Organization.IsNull() && !plan.Organization.IsUnknown() {
		account.Organization = plan.Organization.ValueString()
	}
	if !plan.Parent.IsNull() && !plan.Parent.IsUnknown() {
		account.ParentAccount = plan.Parent.ValueString()
	}

	if err := r.client.CreateAccount(account); err != nil {
		resp.Diagnostics.AddError("Error creating account", err.Error())
		return
	}

	// Step 2: Create the account-level association with limits
	assoc := client.Association{
		Account: plan.Name.ValueString(),
		Cluster: r.client.Cluster,
	}

	hasAssocAttrs := false

	if !plan.Fairshare.IsNull() && !plan.Fairshare.IsUnknown() {
		assoc.Fairshare = &client.SlurmInt{
			Number: int(plan.Fairshare.ValueInt64()),
			Set:    true,
		}
		hasAssocAttrs = true
	}
	if !plan.DefaultQOS.IsNull() && !plan.DefaultQOS.IsUnknown() {
		assoc.Default = &client.AssociationDefaults{
			QOS: plan.DefaultQOS.ValueString(),
		}
		hasAssocAttrs = true
	}
	if !plan.AllowedQOS.IsNull() && !plan.AllowedQOS.IsUnknown() {
		var qosList []string
		diags = plan.AllowedQOS.ElementsAs(ctx, &qosList, false)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		assoc.QOS = qosList
		hasAssocAttrs = true
	}

	if hasAssocAttrs {
		if err := r.client.CreateAssociations([]client.Association{assoc}); err != nil {
			resp.Diagnostics.AddError("Error creating account association", err.Error())
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
		if assoc.Fairshare != nil && assoc.Fairshare.Set {
			state.Fairshare = types.Int64Value(int64(assoc.Fairshare.Number))
		}
		if assoc.Default != nil && assoc.Default.QOS != "" {
			state.DefaultQOS = types.StringValue(assoc.Default.QOS)
		}
		if len(assoc.QOS) > 0 {
			qosValues, diags := types.ListValueFrom(ctx, types.StringType, assoc.QOS)
			resp.Diagnostics.Append(diags...)
			state.AllowedQOS = qosValues
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

	// Update account-level association
	assoc := client.Association{
		Account: plan.Name.ValueString(),
		Cluster: r.client.Cluster,
	}
	if !plan.Fairshare.IsNull() {
		assoc.Fairshare = &client.SlurmInt{
			Number: int(plan.Fairshare.ValueInt64()),
			Set:    true,
		}
	}
	if !plan.DefaultQOS.IsNull() {
		assoc.Default = &client.AssociationDefaults{
			QOS: plan.DefaultQOS.ValueString(),
		}
	}
	if !plan.AllowedQOS.IsNull() {
		var qosList []string
		diags = plan.AllowedQOS.ElementsAs(ctx, &qosList, false)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		assoc.QOS = qosList
	}

	if err := r.client.CreateAssociations([]client.Association{assoc}); err != nil {
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
