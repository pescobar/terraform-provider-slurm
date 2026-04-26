package resources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/pescobar/terraform-provider-slurm/internal/client"
)

var (
	_ resource.Resource                = &userResource{}
	_ resource.ResourceWithImportState = &userResource{}
)

type userResource struct {
	client *client.Client
}

// userResourceModel maps the slurm_user resource schema to Go.
type userResourceModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	AdminLevel     types.String `tfsdk:"admin_level"`
	DefaultAccount types.String `tfsdk:"default_account"`
	Associations   types.Set    `tfsdk:"association"`
}

// associationModel maps a single embedded association block.
type associationModel struct {
	Account    types.String `tfsdk:"account"`
	Partition  types.String `tfsdk:"partition"`
	Fairshare  types.Int64  `tfsdk:"fairshare"`
	DefaultQOS types.String `tfsdk:"default_qos"`
	MaxJobsPU  types.Int64  `tfsdk:"max_jobs"`
	QOS        types.List   `tfsdk:"qos"`
}

// associationModelType returns the object type definition for an association block.
// This is needed for TypeSet to know the shape of each element.
func associationModelType() map[string]attr.Type {
	return map[string]attr.Type{
		"account":     types.StringType,
		"partition":   types.StringType,
		"fairshare":   types.Int64Type,
		"default_qos": types.StringType,
		"max_jobs":    types.Int64Type,
		"qos":         types.ListType{ElemType: types.StringType},
	}
}

func NewUserResource() resource.Resource {
	return &userResource{}
}

func (r *userResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (r *userResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Slurm user with embedded account associations.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The user name (same as name).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The Slurm user name.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"admin_level": schema.StringAttribute{
				Description: "Administrative level: None, Operator, or Administrator.",
				Optional:    true,
				Computed:    true,
			},
			"default_account": schema.StringAttribute{
				Description: "The user's default Slurm account. Must match one of the association accounts.",
				Required:    true,
			},
		},
		Blocks: map[string]schema.Block{
			"association": schema.SetNestedBlock{
				Description: "Account associations for this user. Each block defines the user's " +
					"membership in an account with associated limits and QOS settings. " +
					"At least one association is required.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"account": schema.StringAttribute{
							Description: "The Slurm account name for this association.",
							Required:    true,
						},
						"partition": schema.StringAttribute{
							Description: "Optional partition to scope this association to.",
							Optional:    true,
							Computed:    true,
						},
						"fairshare": schema.Int64Attribute{
							Description: "Fairshare value for this association.",
							Optional:    true,
						},
						"default_qos": schema.StringAttribute{
							Description: "Default QOS for this association.",
							Optional:    true,
						},
						"max_jobs": schema.Int64Attribute{
							Description: "Maximum number of running jobs for this association.",
							Optional:    true,
						},
						"qos": schema.ListAttribute{
							Description: "List of allowed QOS names for this association.",
							Optional:    true,
							ElementType: types.StringType,
						},
					},
				},
			},
		},
	}
}

func (r *userResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// Create creates the user and all its associations.
//
// It uses the users_association endpoint for the first association (which creates
// the user and its initial association atomically), then creates any additional
// associations via the associations endpoint.
func (r *userResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan userResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	userName := plan.Name.ValueString()
	defaultAccount := plan.DefaultAccount.ValueString()

	tflog.Debug(ctx, "Creating user", map[string]interface{}{
		"name":            userName,
		"default_account": defaultAccount,
	})

	// Parse association blocks from the plan
	assocs := r.extractAssociations(ctx, plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	if len(assocs) == 0 {
		resp.Diagnostics.AddError(
			"No associations defined",
			"At least one association block is required for a user.",
		)
		return
	}

	// Validate that default_account matches one of the association accounts
	if !r.hasAccountAssociation(assocs, defaultAccount) {
		resp.Diagnostics.AddError(
			"Invalid default_account",
			fmt.Sprintf("default_account '%s' must match one of the association account values.", defaultAccount),
		)
		return
	}

	// Step 1: Create the user with the first association using the atomic endpoint.
	// We pick the default account's association as the first one.
	firstAssoc, remainingAssocs := r.splitFirstAssociation(assocs, defaultAccount)

	userReq := client.UserAssociationRequest{
		Users: []client.User{
			{
				Name: userName,
				Default: &client.UserDefault{
					Account: defaultAccount,
				},
			},
		},
		Associations: []client.Association{firstAssoc},
	}

	if err := r.client.CreateUserWithAssociation(userReq); err != nil {
		resp.Diagnostics.AddError(
			"Error creating user",
			fmt.Sprintf("Could not create user '%s': %s", userName, err.Error()),
		)
		return
	}

	// Step 2: Create remaining associations (if any)
	if len(remainingAssocs) > 0 {
		if err := r.client.CreateAssociations(remainingAssocs); err != nil {
			resp.Diagnostics.AddError(
				"Error creating additional associations",
				fmt.Sprintf("User '%s' was created but additional associations failed: %s", userName, err.Error()),
			)
			return
		}
	}

	plan.ID = plan.Name

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Read reads the user and all its associations from the API and updates state.
func (r *userResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state userResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	userName := state.Name.ValueString()

	// Read user metadata
	user, err := r.client.GetUser(userName)
	if err != nil {
		resp.Diagnostics.AddError("Error reading user", err.Error())
		return
	}
	if user == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	state.Name = types.StringValue(user.Name)
	state.ID = types.StringValue(user.Name)

	// Admin level from API
	if len(user.AdminLevel) > 0 {
		state.AdminLevel = types.StringValue(user.AdminLevel[0])
	} else {
		state.AdminLevel = types.StringValue("None")
	}

	// Default account from API
	if user.Default != nil && user.Default.Account != "" {
		state.DefaultAccount = types.StringValue(user.Default.Account)
	}

	// Read associations for this user
	assocResp, err := r.client.GetAssociations(map[string]string{
		"user":    userName,
		"cluster": r.client.Cluster,
	})
	if err != nil {
		resp.Diagnostics.AddError("Error reading user associations", err.Error())
		return
	}

	// Convert API associations to Terraform state
	assocObjects := r.apiAssociationsToState(ctx, assocResp.Associations, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	assocSet, setDiags := types.SetValue(types.ObjectType{AttrTypes: associationModelType()}, assocObjects)
	resp.Diagnostics.Append(setDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.Associations = assocSet

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

// Update handles changes to the user and its associations.
//
// This is the critical function. It must:
// 1. Update user-level attributes (admin_level, default_account)
// 2. Diff old vs new associations and apply individual create/update/delete
//
// IMPORTANT: The order of operations matters when changing default_account.
// If the new default_account is a new association, we must create that
// association before updating the user's default. If the old default_account
// is being removed, we must update the user's default before deleting it.
//
// The order is: Create → Update user → Update associations → Delete
func (r *userResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan userResourceModel
	var state userResourceModel

	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	userName := plan.Name.ValueString()
	newDefaultAccount := plan.DefaultAccount.ValueString()
	oldDefaultAccount := state.DefaultAccount.ValueString()

	tflog.Debug(ctx, "Updating user", map[string]interface{}{
		"name":                userName,
		"old_default_account": oldDefaultAccount,
		"new_default_account": newDefaultAccount,
	})

	// Parse old and new associations
	oldAssocs := r.extractAssociations(ctx, state, &resp.Diagnostics)
	newAssocs := r.extractAssociations(ctx, plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// Validate that new default_account matches one of the new associations
	if !r.hasAccountAssociation(newAssocs, newDefaultAccount) {
		resp.Diagnostics.AddError(
			"Invalid default_account",
			fmt.Sprintf("default_account '%s' must match one of the association account values.", newDefaultAccount),
		)
		return
	}

	// Compute the diff
	diff := DiffAssociations(oldAssocs, newAssocs)

	tflog.Debug(ctx, "Association diff computed", map[string]interface{}{
		"creates": len(diff.Create),
		"updates": len(diff.Update),
		"deletes": len(diff.Delete),
	})

	// --- Step 1: Create new associations ---
	// Must happen before updating default_account, in case the new default
	// points to a newly created association.
	if len(diff.Create) > 0 {
		if err := r.client.CreateAssociations(diff.Create); err != nil {
			resp.Diagnostics.AddError(
				"Error creating new associations",
				fmt.Sprintf("Failed to create associations for user '%s': %s", userName, err.Error()),
			)
			return
		}
		tflog.Debug(ctx, "Created new associations", map[string]interface{}{
			"count": len(diff.Create),
		})
	}

	// --- Step 2: Update user-level attributes ---
	// This includes default_account and admin_level changes.
	if oldDefaultAccount != newDefaultAccount ||
		state.AdminLevel.ValueString() != plan.AdminLevel.ValueString() {

		user := client.User{
			Name: userName,
			Default: &client.UserDefault{
				Account: newDefaultAccount,
			},
		}

		if !plan.AdminLevel.IsNull() && !plan.AdminLevel.IsUnknown() {
			user.AdminLevel = []string{plan.AdminLevel.ValueString()}
		}

		if err := r.client.UpdateUser(user); err != nil {
			resp.Diagnostics.AddError(
				"Error updating user",
				fmt.Sprintf("Failed to update user '%s': %s", userName, err.Error()),
			)
			return
		}
		tflog.Debug(ctx, "Updated user attributes")
	}

	// --- Step 3: Update existing associations ---
	if len(diff.Update) > 0 {
		if err := r.client.CreateAssociations(diff.Update); err != nil {
			resp.Diagnostics.AddError(
				"Error updating associations",
				fmt.Sprintf("Failed to update associations for user '%s': %s", userName, err.Error()),
			)
			return
		}
		tflog.Debug(ctx, "Updated existing associations", map[string]interface{}{
			"count": len(diff.Update),
		})
	}

	// --- Step 4: Delete removed associations ---
	// Must happen after updating default_account, in case the old default
	// account's association is being removed.
	for _, key := range diff.Delete {
		if err := r.client.DeleteAssociation(key.Account, userName, r.client.Cluster, key.Partition); err != nil {
			resp.Diagnostics.AddError(
				"Error deleting association",
				fmt.Sprintf("Failed to delete association '%s' for user '%s': %s",
					key.String(), userName, err.Error()),
			)
			return
		}
		tflog.Debug(ctx, "Deleted association", map[string]interface{}{
			"key": key.String(),
		})
	}

	plan.ID = plan.Name

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Delete removes the user (Slurm automatically cleans up its associations).
func (r *userResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state userResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	userName := state.Name.ValueString()

	tflog.Debug(ctx, "Deleting user", map[string]interface{}{
		"name": userName,
	})

	if err := r.client.DeleteUser(userName); err != nil {
		resp.Diagnostics.AddError("Error deleting user", err.Error())
		return
	}
}

// ImportState imports an existing user by name.
func (r *userResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// extractAssociations parses the association blocks from a user model into
// client.Association structs ready for API calls.
func (r *userResource) extractAssociations(ctx context.Context, model userResourceModel, diagnostics *diag.Diagnostics) []client.Association {
	if model.Associations.IsNull() || model.Associations.IsUnknown() {
		return nil
	}

	var assocModels []associationModel
	diags := model.Associations.ElementsAs(ctx, &assocModels, false)
	diagnostics.Append(diags...)
	if diagnostics.HasError() {
		return nil
	}

	userName := model.Name.ValueString()
	var result []client.Association

	for _, am := range assocModels {
		a := client.Association{
			Account: am.Account.ValueString(),
			Cluster: r.client.Cluster,
			User:    userName,
		}

		if !am.Partition.IsNull() && !am.Partition.IsUnknown() {
			a.Partition = am.Partition.ValueString()
		}

		if !am.Fairshare.IsNull() && !am.Fairshare.IsUnknown() {
			a.Fairshare = &client.SlurmInt{
				Number: int(am.Fairshare.ValueInt64()),
				Set:    true,
			}
		}

		if !am.DefaultQOS.IsNull() && !am.DefaultQOS.IsUnknown() {
			a.Default = &client.AssociationDefaults{
				QOS: am.DefaultQOS.ValueString(),
			}
		}

		if !am.MaxJobsPU.IsNull() && !am.MaxJobsPU.IsUnknown() {
			a.Max = &client.AssociationMax{
				Jobs: &client.AssociationMaxJobs{
					Per: &client.AssociationMaxJobsPer{
						Count: &client.SlurmInt{
							Number: int(am.MaxJobsPU.ValueInt64()),
							Set:    true,
						},
					},
				},
			}
		}

		if !am.QOS.IsNull() && !am.QOS.IsUnknown() {
			var qosList []string
			diags := am.QOS.ElementsAs(ctx, &qosList, false)
			diagnostics.Append(diags...)
			a.QOS = qosList
		}

		result = append(result, a)
	}

	return result
}

// apiAssociationsToState converts API association responses to Terraform
// attr.Value objects suitable for setting in state.
func (r *userResource) apiAssociationsToState(ctx context.Context, assocs []client.Association, diagnostics *diag.Diagnostics) []attr.Value {
	var result []attr.Value

	for _, a := range assocs {
		// Skip account-level associations (no user)
		if a.User == "" {
			continue
		}

		attrs := map[string]attr.Value{
			"account":     types.StringValue(a.Account),
			"partition":   types.StringValue(a.Partition),
			"fairshare":   types.Int64Null(),
			"default_qos": types.StringNull(),
			"max_jobs":    types.Int64Null(),
			"qos":         types.ListNull(types.StringType),
		}

		if a.Fairshare != nil && a.Fairshare.Set {
			attrs["fairshare"] = types.Int64Value(int64(a.Fairshare.Number))
		}

		if a.Default != nil && a.Default.QOS != "" {
			attrs["default_qos"] = types.StringValue(a.Default.QOS)
		}

		if a.Max != nil && a.Max.Jobs != nil && a.Max.Jobs.Per != nil && a.Max.Jobs.Per.Count != nil && a.Max.Jobs.Per.Count.Set {
			attrs["max_jobs"] = types.Int64Value(int64(a.Max.Jobs.Per.Count.Number))
		}

		if len(a.QOS) > 0 {
			qosVal, diags := types.ListValueFrom(ctx, types.StringType, a.QOS)
			diagnostics.Append(diags...)
			attrs["qos"] = qosVal
		}

		obj, diags := types.ObjectValue(associationModelType(), attrs)
		diagnostics.Append(diags...)
		result = append(result, obj)
	}

	return result
}

// hasAccountAssociation checks if the given account appears in any association.
func (r *userResource) hasAccountAssociation(assocs []client.Association, account string) bool {
	for _, a := range assocs {
		if a.Account == account {
			return true
		}
	}
	return false
}

// splitFirstAssociation separates the association for the default account
// from the rest. The default account's association is returned first because
// it must be created atomically with the user via users_association endpoint.
func (r *userResource) splitFirstAssociation(assocs []client.Association, defaultAccount string) (client.Association, []client.Association) {
	var first client.Association
	var rest []client.Association
	found := false

	for _, a := range assocs {
		if !found && a.Account == defaultAccount {
			first = a
			found = true
		} else {
			rest = append(rest, a)
		}
	}

	return first, rest
}
