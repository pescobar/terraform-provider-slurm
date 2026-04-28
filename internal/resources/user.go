package resources

import (
	"context"
	"fmt"
	"strings"

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
	DefaultWCKey   types.String `tfsdk:"default_wc_key"`
	Associations   types.Set    `tfsdk:"association"`
}

// associationModel maps a single embedded association block.
type associationModel struct {
	Account   types.String `tfsdk:"account"`
	Partition types.String `tfsdk:"partition"`
	// Priority and fairshare
	Fairshare types.Int64 `tfsdk:"fairshare"`
	Priority  types.Int64 `tfsdk:"priority"`
	// Default settings
	DefaultQOS types.String `tfsdk:"default_qos"`
	QOS        types.List   `tfsdk:"qos"`
	// Max job-count limits
	MaxJobs       types.Int64 `tfsdk:"max_jobs"`
	MaxJobsAccrue types.Int64 `tfsdk:"max_jobs_accrue"`
	MaxSubmitJobs types.Int64 `tfsdk:"max_submit_jobs"`
	// Max wall-clock
	MaxWallPJ types.Int64 `tfsdk:"max_wall_pj"`
	// Max TRES limits
	MaxTRESPerJob    types.Set `tfsdk:"max_tres_per_job"`
	MaxTRESPerNode   types.Set `tfsdk:"max_tres_per_node"`
	MaxTRESMinsPerJob types.Set `tfsdk:"max_tres_mins_per_job"`
	// Grp job-count limits
	GrpJobs       types.Int64 `tfsdk:"grp_jobs"`
	GrpJobsAccrue types.Int64 `tfsdk:"grp_jobs_accrue"`
	GrpSubmitJobs types.Int64 `tfsdk:"grp_submit_jobs"`
	// Grp wall-clock
	GrpWall types.Int64 `tfsdk:"grp_wall"`
	// Grp TRES limits
	GrpTRES        types.Set `tfsdk:"grp_tres"`
	GrpTRESMins    types.Set `tfsdk:"grp_tres_mins"`
	GrpTRESRunMins types.Set `tfsdk:"grp_tres_run_mins"`
}

// associationModelType returns the object type definition for an association block.
// This is needed for TypeSet to know the shape of each element.
func associationModelType() map[string]attr.Type {
	tresST := types.SetType{ElemType: tresElemType()}
	return map[string]attr.Type{
		"account":   types.StringType,
		"partition": types.StringType,
		// Priority and fairshare
		"fairshare": types.Int64Type,
		"priority":  types.Int64Type,
		// Default settings
		"default_qos": types.StringType,
		"qos":         types.ListType{ElemType: types.StringType},
		// Max job-count limits
		"max_jobs":        types.Int64Type,
		"max_jobs_accrue": types.Int64Type,
		"max_submit_jobs": types.Int64Type,
		// Max wall-clock
		"max_wall_pj": types.Int64Type,
		// Max TRES limits
		"max_tres_per_job":     tresST,
		"max_tres_per_node":    tresST,
		"max_tres_mins_per_job": tresST,
		// Grp job-count limits
		"grp_jobs":        types.Int64Type,
		"grp_jobs_accrue": types.Int64Type,
		"grp_submit_jobs": types.Int64Type,
		// Grp wall-clock
		"grp_wall": types.Int64Type,
		// Grp TRES limits
		"grp_tres":         tresST,
		"grp_tres_mins":    tresST,
		"grp_tres_run_mins": tresST,
	}
}

// qosAccessHint is appended to association errors caused by Slurm's QOS
// access constraints so users understand what went wrong and how to fix it.
const qosAccessHint = `
Slurm enforces two QOS access rules on user associations:

  Rule 1 — qos list overrides account default_qos:
    If you set 'qos' on an association, any 'default_qos' that the account
    inherits is blocked for the user unless it also appears in that list.
    Fix: add the account's default_qos value to the association's 'qos' list.

  Rule 2 — default_qos requires direct allowed_qos on the account:
    If you set 'default_qos' on an association without an explicit 'qos' list,
    the QOS must be present in the account's own 'allowed_qos'. Slurm does NOT
    follow the parent account hierarchy for this check.
    Fix: add the QOS to the account's 'allowed_qos', or add an explicit 'qos'
    list to the association that includes the 'default_qos' value.`

// isQOSAccessError reports whether a Slurm API error is caused by a QOS
// access constraint violation on an association.
func isQOSAccessError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "would not have access to their default qos") ||
		strings.Contains(msg, "don't have access to their default qos")
}

// assocErrorDetail builds the diagnostic detail string for an association
// error, appending qosAccessHint when the root cause is a QOS constraint.
// prefix is a plain string describing the operation (no format verbs needed).
func assocErrorDetail(prefix string, err error) string {
	if isQOSAccessError(err) {
		return fmt.Sprintf("%s: %s%s", prefix, err.Error(), qosAccessHint)
	}
	return fmt.Sprintf("%s: %s", prefix, err.Error())
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
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"default_account": schema.StringAttribute{
				Description: "The user's default Slurm account. Must match one of the association accounts.",
				Required:    true,
			},
			"default_wc_key": schema.StringAttribute{
				Description: "Default workload characterization key for the user.",
				Optional:    true,
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
						},
						"fairshare": schema.Int64Attribute{
							Description: "Fairshare value for this association (default: 1).",
							Optional:    true,
						},
						"priority": schema.Int64Attribute{
							Description: "Association-level priority (distinct from QOS priority).",
							Optional:    true,
						},
						"default_qos": schema.StringAttribute{
							Description: "Default QOS for this association.",
							Optional:    true,
						},
						"qos": schema.ListAttribute{
							Description: "List of allowed QOS names for this association.",
							Optional:    true,
							ElementType: types.StringType,
						},
						// Max job-count limits
						"max_jobs": schema.Int64Attribute{
							Description: "Maximum number of running jobs for this association (MaxJobs).",
							Optional:    true,
						},
						"max_jobs_accrue": schema.Int64Attribute{
							Description: "Maximum pending jobs that can accrue age priority (MaxJobsAccrue).",
							Optional:    true,
						},
						"max_submit_jobs": schema.Int64Attribute{
							Description: "Maximum number of jobs that can be submitted at once (MaxSubmitJobs).",
							Optional:    true,
						},
						// Max wall-clock
						"max_wall_pj": schema.Int64Attribute{
							Description: "Maximum wall-clock time per job in minutes (MaxWallDurationPerJob).",
							Optional:    true,
						},
						// Max TRES limits
						"max_tres_per_job":      tresOptionalSchemaAttr("Maximum TRES per job (MaxTRES). Each entry specifies type, optional name, and count."),
						"max_tres_per_node":     tresOptionalSchemaAttr("Maximum TRES per node per job (MaxTRESPerNode)."),
						"max_tres_mins_per_job": tresOptionalSchemaAttr("Maximum TRES-minutes per job (MaxTRESMins)."),
						// Grp job-count limits
						"grp_jobs": schema.Int64Attribute{
							Description: "Maximum running jobs across all users in this association group (GrpJobs).",
							Optional:    true,
						},
						"grp_jobs_accrue": schema.Int64Attribute{
							Description: "Maximum pending jobs accruing priority across the group (GrpJobsAccrue).",
							Optional:    true,
						},
						"grp_submit_jobs": schema.Int64Attribute{
							Description: "Maximum submitted jobs across the group (GrpSubmitJobs).",
							Optional:    true,
						},
						// Grp wall-clock
						"grp_wall": schema.Int64Attribute{
							Description: "Maximum cumulative wall-clock minutes for running jobs in the group (GrpWall).",
							Optional:    true,
						},
						// Grp TRES limits
						"grp_tres":          tresOptionalSchemaAttr("Maximum TRES in use at once across the group (GrpTRES)."),
						"grp_tres_mins":     tresOptionalSchemaAttr("Maximum TRES-minutes for the group (GrpTRESMins)."),
						"grp_tres_run_mins": tresOptionalSchemaAttr("Maximum TRES-minutes of currently running jobs for the group (GrpTRESRunMins)."),
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

	// Step 1: Create the user entity + initial association via the atomic endpoint.
	// In API v0.0.42 this endpoint takes association_condition (user/account lists),
	// not a full users+associations payload. Limits cannot be set here.
	userReq := client.UserAssociationRequest{
		AssociationCondition: client.UserAssociationCondition{
			Users:    []string{userName},
			Accounts: []string{defaultAccount},
		},
		User: client.UserShort{},
	}
	if !plan.AdminLevel.IsNull() && !plan.AdminLevel.IsUnknown() {
		userReq.User.AdminLevel = []string{plan.AdminLevel.ValueString()}
	}

	if err := r.client.CreateUserWithAssociation(userReq); err != nil {
		resp.Diagnostics.AddError(
			"Error creating user",
			fmt.Sprintf("Could not create user '%s': %s", userName, err.Error()),
		)
		return
	}

	// The users_association endpoint ignores administrator_level even when sent.
	// Apply admin_level and default_wc_key with a separate UpdateUser call.
	needsUpdate := false
	updateUser := client.User{
		Name:    userName,
		Default: &client.UserDefault{Account: defaultAccount},
	}
	if !plan.AdminLevel.IsNull() && !plan.AdminLevel.IsUnknown() {
		if al := plan.AdminLevel.ValueString(); al != "" && al != "None" {
			updateUser.AdminLevel = []string{al}
			needsUpdate = true
		}
	}
	if !plan.DefaultWCKey.IsNull() && !plan.DefaultWCKey.IsUnknown() {
		updateUser.Default.WCKey = plan.DefaultWCKey.ValueString()
		needsUpdate = true
	}
	if needsUpdate {
		if err := r.client.UpdateUser(updateUser); err != nil {
			resp.Diagnostics.AddError(
				"Error setting user attributes after creation",
				fmt.Sprintf("User '%s' was created but attributes could not be set: %s", userName, err.Error()),
			)
			return
		}
	}

	// Step 2: Set limits on all associations via the associations endpoint (upsert).
	// This covers both the initial association created above and any additional ones.
	if err := r.client.CreateAssociations(assocs); err != nil {
		resp.Diagnostics.AddError(
			"Error creating associations",
			assocErrorDetail(fmt.Sprintf("User '%s' was created but associations failed", userName), err),
		)
		return
	}

	plan.ID = plan.Name

	// admin_level is Optional+Computed; resolve to "None" on first apply when
	// no prior state exists for UseStateForUnknown to draw from.
	if plan.AdminLevel.IsUnknown() {
		plan.AdminLevel = types.StringValue("None")
	}

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

	// Default account — the user endpoint always returns default.account="".
	// We derive it from the association marked is_default instead.
	if user.Default != nil && user.Default.WCKey != "" && !state.DefaultWCKey.IsNull() {
		state.DefaultWCKey = types.StringValue(user.Default.WCKey)
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

	// Derive default_account from the association marked is_default.
	// The user endpoint always returns default.account="" in Slurm REST API,
	// so we use the is_default flag on associations as the authoritative source.
	for _, a := range assocResp.Associations {
		if a.User != "" && a.IsDefault {
			state.DefaultAccount = types.StringValue(a.Account)
			break
		}
	}

	// Build a map of prior associations keyed by "account|partition".
	// This is used in apiAssociationsToState to suppress Slurm-default and
	// inherited values that were never explicitly set in the config.
	priorAssocMap := make(map[string]associationModel)
	if !state.Associations.IsNull() && !state.Associations.IsUnknown() {
		var priorAssocs []associationModel
		if d := state.Associations.ElementsAs(ctx, &priorAssocs, false); !d.HasError() {
			for _, pa := range priorAssocs {
				key := pa.Account.ValueString() + "|" + pa.Partition.ValueString()
				priorAssocMap[key] = pa
			}
		}
	}

	// Convert API associations to Terraform state
	assocObjects := r.apiAssociationsToState(ctx, assocResp.Associations, priorAssocMap, &resp.Diagnostics)
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
				assocErrorDetail(fmt.Sprintf("Failed to create associations for user '%s'", userName), err),
			)
			return
		}
		tflog.Debug(ctx, "Created new associations", map[string]interface{}{
			"count": len(diff.Create),
		})
	}

	// --- Step 2: Update user-level attributes ---
	wcKeyChanged := state.DefaultWCKey.ValueString() != plan.DefaultWCKey.ValueString()
	if oldDefaultAccount != newDefaultAccount ||
		state.AdminLevel.ValueString() != plan.AdminLevel.ValueString() ||
		wcKeyChanged {

		user := client.User{
			Name:    userName,
			Default: &client.UserDefault{Account: newDefaultAccount},
		}
		if !plan.AdminLevel.IsNull() && !plan.AdminLevel.IsUnknown() {
			user.AdminLevel = []string{plan.AdminLevel.ValueString()}
		}
		if !plan.DefaultWCKey.IsNull() && !plan.DefaultWCKey.IsUnknown() {
			user.Default.WCKey = plan.DefaultWCKey.ValueString()
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
				assocErrorDetail(fmt.Sprintf("Failed to update associations for user '%s'", userName), err),
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
			v := int(am.Fairshare.ValueInt64())
			a.SharesRaw = &v
		}

		if !am.Priority.IsNull() && !am.Priority.IsUnknown() {
			a.Priority = &client.SlurmInt{Number: int(am.Priority.ValueInt64()), Set: true}
		}

		// Default settings
		if !am.DefaultQOS.IsNull() && !am.DefaultQOS.IsUnknown() {
			if a.Default == nil {
				a.Default = &client.AssociationDefaults{}
			}
			a.Default.QOS = am.DefaultQOS.ValueString()
		}

		if !am.QOS.IsNull() && !am.QOS.IsUnknown() {
			var qosList []string
			diags := am.QOS.ElementsAs(ctx, &qosList, false)
			diagnostics.Append(diags...)
			a.QOS = qosList
		}

		a.Max = r.extractAssocMax(ctx, am, diagnostics)

		result = append(result, a)
	}

	return result
}

// extractAssocMax builds the AssociationMax struct from an association model.
// Returns nil when no limits are configured.
//
// API path reference (v0.0.42):
//
//	MaxJobs        → max.jobs.active
//	MaxJobsAccrue  → max.jobs.accruing
//	MaxSubmitJobs  → max.jobs.total
//	MaxWall        → max.jobs.per.wall_clock  (minutes)
//	GrpJobs        → max.jobs.per.count
//	GrpJobsAccrue  → max.jobs.per.accruing
//	GrpSubmitJobs  → max.jobs.per.submitted
//	GrpWall        → max.per.account.wall_clock  (minutes)
//	MaxTRES        → max.tres.per.job
//	MaxTRESPerNode → max.tres.per.node
//	MaxTRESMins    → max.tres.minutes.per.job
//	GrpTRES        → max.tres.total
//	GrpTRESMins    → max.tres.group.minutes
//	GrpTRESRunMins → max.tres.group.active
func (r *userResource) extractAssocMax(ctx context.Context, am associationModel, diagnostics *diag.Diagnostics) *client.AssociationMax {
	var m client.AssociationMax
	set := false

	// ---- max.jobs ----
	var jobs client.AssociationMaxJobs
	var jobsPer client.AssociationMaxJobsPer
	jobsSet, jobsPerSet := false, false

	if !am.MaxJobs.IsNull() && !am.MaxJobs.IsUnknown() {
		jobs.Active = &client.SlurmInt{Number: int(am.MaxJobs.ValueInt64()), Set: true}
		jobsSet = true
	}
	if !am.MaxJobsAccrue.IsNull() && !am.MaxJobsAccrue.IsUnknown() {
		jobs.Accruing = &client.SlurmInt{Number: int(am.MaxJobsAccrue.ValueInt64()), Set: true}
		jobsSet = true
	}
	if !am.MaxSubmitJobs.IsNull() && !am.MaxSubmitJobs.IsUnknown() {
		jobs.Total = &client.SlurmInt{Number: int(am.MaxSubmitJobs.ValueInt64()), Set: true}
		jobsSet = true
	}
	if !am.MaxWallPJ.IsNull() && !am.MaxWallPJ.IsUnknown() {
		jobsPer.WallClock = &client.SlurmInt{Number: int(am.MaxWallPJ.ValueInt64()), Set: true}
		jobsPerSet = true
	}
	if !am.GrpJobs.IsNull() && !am.GrpJobs.IsUnknown() {
		jobsPer.Count = &client.SlurmInt{Number: int(am.GrpJobs.ValueInt64()), Set: true}
		jobsPerSet = true
	}
	if !am.GrpJobsAccrue.IsNull() && !am.GrpJobsAccrue.IsUnknown() {
		jobsPer.Accruing = &client.SlurmInt{Number: int(am.GrpJobsAccrue.ValueInt64()), Set: true}
		jobsPerSet = true
	}
	if !am.GrpSubmitJobs.IsNull() && !am.GrpSubmitJobs.IsUnknown() {
		jobsPer.Submitted = &client.SlurmInt{Number: int(am.GrpSubmitJobs.ValueInt64()), Set: true}
		jobsPerSet = true
	}
	if jobsPerSet {
		jobs.Per = &jobsPer
		jobsSet = true
	}
	if jobsSet {
		m.Jobs = &jobs
		set = true
	}

	// ---- max.tres ----
	var tres client.AssociationMaxTRES
	tresSet := false

	if !am.MaxTRESPerJob.IsNull() && !am.MaxTRESPerJob.IsUnknown() {
		list := planTresListToAPI(ctx, am.MaxTRESPerJob)
		if len(list) > 0 {
			if tres.Per == nil {
				tres.Per = &client.AssociationMaxTRESPer{}
			}
			tres.Per.Job = list
			tresSet = true
		}
	}
	if !am.MaxTRESPerNode.IsNull() && !am.MaxTRESPerNode.IsUnknown() {
		list := planTresListToAPI(ctx, am.MaxTRESPerNode)
		if len(list) > 0 {
			if tres.Per == nil {
				tres.Per = &client.AssociationMaxTRESPer{}
			}
			tres.Per.Node = list
			tresSet = true
		}
	}
	if !am.MaxTRESMinsPerJob.IsNull() && !am.MaxTRESMinsPerJob.IsUnknown() {
		list := planTresListToAPI(ctx, am.MaxTRESMinsPerJob)
		if len(list) > 0 {
			tres.Minutes = &client.AssociationMaxTRESMins{
				Per: &client.AssociationMaxTRESMinsPer{Job: list},
			}
			tresSet = true
		}
	}
	if !am.GrpTRES.IsNull() && !am.GrpTRES.IsUnknown() {
		list := planTresListToAPI(ctx, am.GrpTRES)
		if len(list) > 0 {
			tres.Total = list
			tresSet = true
		}
	}
	if !am.GrpTRESMins.IsNull() && !am.GrpTRESMins.IsUnknown() {
		list := planTresListToAPI(ctx, am.GrpTRESMins)
		if len(list) > 0 {
			if tres.Group == nil {
				tres.Group = &client.AssociationMaxTRESGroup{}
			}
			tres.Group.Minutes = list
			tresSet = true
		}
	}
	if !am.GrpTRESRunMins.IsNull() && !am.GrpTRESRunMins.IsUnknown() {
		list := planTresListToAPI(ctx, am.GrpTRESRunMins)
		if len(list) > 0 {
			if tres.Group == nil {
				tres.Group = &client.AssociationMaxTRESGroup{}
			}
			tres.Group.Active = list
			tresSet = true
		}
	}
	if tresSet {
		m.TRES = &tres
		set = true
	}

	// ---- max.per.account.wall_clock (GrpWall) ----
	if !am.GrpWall.IsNull() && !am.GrpWall.IsUnknown() {
		m.Per = &client.AssociationMaxPerNode{
			Account: &client.AssociationMaxPerAccount{
				WallClock: &client.SlurmInt{Number: int(am.GrpWall.ValueInt64()), Set: true},
			},
		}
		set = true
	}

	if !set {
		return nil
	}
	return &m
}

// apiAssociationsToState converts API association responses to Terraform
// attr.Value objects suitable for setting in state.
//
// priorAssocMap (keyed by "account|partition") is used to suppress Slurm's
// default and inherited values for Optional fields. If a field was null in
// the prior state we keep it null even if the API returns a default value,
// preventing spurious drift on every subsequent apply.
// When hasPrior is false (import, or first read before any apply), we
// include all API values so imports capture the full server state.
func (r *userResource) apiAssociationsToState(ctx context.Context, assocs []client.Association, priorAssocMap map[string]associationModel, diagnostics *diag.Diagnostics) []attr.Value {
	nullTRES := types.SetNull(tresElemType())

	var result []attr.Value

	for _, a := range assocs {
		// Skip account-level associations (no user)
		if a.User == "" {
			continue
		}

		key := a.Account + "|" + a.Partition
		prior, hasPrior := priorAssocMap[key]

		var partitionVal attr.Value
		if a.Partition != "" {
			partitionVal = types.StringValue(a.Partition)
		} else {
			partitionVal = types.StringNull()
		}

		attrs := map[string]attr.Value{
			"account":   types.StringValue(a.Account),
			"partition": partitionVal,
			// Priority and fairshare
			"fairshare": types.Int64Null(),
			"priority":  types.Int64Null(),
			// Default settings
			"default_qos": types.StringNull(),
			"qos":         types.ListNull(types.StringType),
			// Max job-count
			"max_jobs":        types.Int64Null(),
			"max_jobs_accrue": types.Int64Null(),
			"max_submit_jobs": types.Int64Null(),
			// Max wall-clock
			"max_wall_pj": types.Int64Null(),
			// Max TRES
			"max_tres_per_job":      nullTRES,
			"max_tres_per_node":     nullTRES,
			"max_tres_mins_per_job": nullTRES,
			// Grp job-count
			"grp_jobs":        types.Int64Null(),
			"grp_jobs_accrue": types.Int64Null(),
			"grp_submit_jobs": types.Int64Null(),
			// Grp wall-clock
			"grp_wall": types.Int64Null(),
			// Grp TRES
			"grp_tres":          nullTRES,
			"grp_tres_mins":     nullTRES,
			"grp_tres_run_mins": nullTRES,
		}

		// fairshare — Slurm always returns shares_raw=1 for users without an
		// explicit setting (1 is the default). During import (no prior state)
		// we skip that default to avoid drift against configs that omit fairshare.
		// When prior state exists we follow the usual null-preservation rule.
		if a.SharesRaw != nil {
			if hasPrior {
				if !prior.Fairshare.IsNull() {
					attrs["fairshare"] = types.Int64Value(int64(*a.SharesRaw))
				}
			} else if *a.SharesRaw != 1 {
				attrs["fairshare"] = types.Int64Value(int64(*a.SharesRaw))
			}
		}

		// priority
		if a.Priority != nil && a.Priority.Set && (!hasPrior || !prior.Priority.IsNull()) {
			attrs["priority"] = types.Int64Value(int64(a.Priority.Number))
		}

		// default_qos
		if a.Default != nil && a.Default.QOS != "" && (!hasPrior || !prior.DefaultQOS.IsNull()) {
			attrs["default_qos"] = types.StringValue(a.Default.QOS)
		}

		// qos list — only propagate when we have prior state. On import (hasPrior=false)
		// we skip qos to avoid reconcile failures: Slurm rejects clearing a qos list when
		// default_qos would become inaccessible (e.g. account has no allowed_qos of its own).
		if len(a.QOS) > 0 && hasPrior && !prior.QOS.IsNull() {
			qosVal, diags := types.ListValueFrom(ctx, types.StringType, a.QOS)
			diagnostics.Append(diags...)
			attrs["qos"] = qosVal
		}

		// max.jobs.*
		if a.Max != nil && a.Max.Jobs != nil {
			j := a.Max.Jobs
			if j.Active != nil && j.Active.Set && (!hasPrior || !prior.MaxJobs.IsNull()) {
				attrs["max_jobs"] = types.Int64Value(int64(j.Active.Number))
			}
			if j.Accruing != nil && j.Accruing.Set && (!hasPrior || !prior.MaxJobsAccrue.IsNull()) {
				attrs["max_jobs_accrue"] = types.Int64Value(int64(j.Accruing.Number))
			}
			if j.Total != nil && j.Total.Set && (!hasPrior || !prior.MaxSubmitJobs.IsNull()) {
				attrs["max_submit_jobs"] = types.Int64Value(int64(j.Total.Number))
			}
			if j.Per != nil {
				if j.Per.WallClock != nil && j.Per.WallClock.Set && (!hasPrior || !prior.MaxWallPJ.IsNull()) {
					attrs["max_wall_pj"] = types.Int64Value(int64(j.Per.WallClock.Number))
				}
				if j.Per.Count != nil && j.Per.Count.Set && (!hasPrior || !prior.GrpJobs.IsNull()) {
					attrs["grp_jobs"] = types.Int64Value(int64(j.Per.Count.Number))
				}
				if j.Per.Accruing != nil && j.Per.Accruing.Set && (!hasPrior || !prior.GrpJobsAccrue.IsNull()) {
					attrs["grp_jobs_accrue"] = types.Int64Value(int64(j.Per.Accruing.Number))
				}
				if j.Per.Submitted != nil && j.Per.Submitted.Set && (!hasPrior || !prior.GrpSubmitJobs.IsNull()) {
					attrs["grp_submit_jobs"] = types.Int64Value(int64(j.Per.Submitted.Number))
				}
			}
		}

		// max.per.account.wall_clock (GrpWall)
		if a.Max != nil && a.Max.Per != nil && a.Max.Per.Account != nil &&
			a.Max.Per.Account.WallClock != nil && a.Max.Per.Account.WallClock.Set &&
			(!hasPrior || !prior.GrpWall.IsNull()) {
			attrs["grp_wall"] = types.Int64Value(int64(a.Max.Per.Account.WallClock.Number))
		}

		// max.tres.*
		if a.Max != nil && a.Max.TRES != nil {
			t := a.Max.TRES
			if len(t.Total) > 0 && (!hasPrior || !prior.GrpTRES.IsNull()) {
				attrs["grp_tres"] = apiTresListToSet(ctx, t.Total, diagnostics)
			}
			if t.Group != nil {
				if len(t.Group.Minutes) > 0 && (!hasPrior || !prior.GrpTRESMins.IsNull()) {
					attrs["grp_tres_mins"] = apiTresListToSet(ctx, t.Group.Minutes, diagnostics)
				}
				if len(t.Group.Active) > 0 && (!hasPrior || !prior.GrpTRESRunMins.IsNull()) {
					attrs["grp_tres_run_mins"] = apiTresListToSet(ctx, t.Group.Active, diagnostics)
				}
			}
			if t.Per != nil {
				if len(t.Per.Job) > 0 && (!hasPrior || !prior.MaxTRESPerJob.IsNull()) {
					attrs["max_tres_per_job"] = apiTresListToSet(ctx, t.Per.Job, diagnostics)
				}
				if len(t.Per.Node) > 0 && (!hasPrior || !prior.MaxTRESPerNode.IsNull()) {
					attrs["max_tres_per_node"] = apiTresListToSet(ctx, t.Per.Node, diagnostics)
				}
			}
			if t.Minutes != nil && t.Minutes.Per != nil {
				if len(t.Minutes.Per.Job) > 0 && (!hasPrior || !prior.MaxTRESMinsPerJob.IsNull()) {
					attrs["max_tres_mins_per_job"] = apiTresListToSet(ctx, t.Minutes.Per.Job, diagnostics)
				}
			}
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
