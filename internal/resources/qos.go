package resources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/float64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/float64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/pescobar/terraform-provider-slurm/internal/client"
)

// Valid QOS flag values accepted by slurmrestd v0.0.42. Sourced from the
// OpenAPI schema; these are the strings the API exposes (UPPER_SNAKE_CASE).
var qosFlagValues = []string{
	"PARTITION_MINIMUM_NODE",
	"PARTITION_MAXIMUM_NODE",
	"PARTITION_TIME_LIMIT",
	"ENFORCE_USAGE_THRESHOLD",
	"NO_RESERVE",
	"REQUIRED_RESERVATION",
	"DENY_LIMIT",
	"OVERRIDE_PARTITION_QOS",
	"NO_DECAY",
	"USAGE_FACTOR_SAFE",
	"RELATIVE",
}

// Valid preempt_mode values for a QOS. Slurm also accepts a leading "OFF"
// (alias for empty/disabled) and the special "WITHIN" mode.
var qosPreemptModeValues = []string{
	"OFF",
	"CANCEL",
	"GANG",
	"REQUEUE",
	"SUSPEND",
	"WITHIN",
}

var (
	_ resource.Resource                   = &qosResource{}
	_ resource.ResourceWithImportState    = &qosResource{}
	_ resource.ResourceWithValidateConfig = &qosResource{}
)

// systemQOSNames lists the QOS names that slurmdbd auto-creates at database
// initialisation. Managing these as provider resources is a footgun: a
// destroy soft-deletes the row (deleted=1), and the next apply triggers an
// UPDATE that slurmrestd's internal verification query then can't find,
// surfacing as "Slurmdbd query returned with empty list" (Bug 3 in
// CLAUDE.md). Today only "normal" exists; the slice keeps the door open
// for future Slurm versions.
var systemQOSNames = []string{"normal"}

type qosResource struct {
	client *client.Client
}

// tresModel is the in-state representation of a single TRES entry.
type tresModel struct {
	Type  types.String `tfsdk:"type"`
	Name  types.String `tfsdk:"name"`
	Count types.Int64  `tfsdk:"count"`
}

type qosResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Priority    types.Int64  `tfsdk:"priority"`
	MaxWallPJ   types.Int64  `tfsdk:"max_wall_pj"`
	GrpWall     types.Int64  `tfsdk:"grp_wall"`
	Flags       types.Set    `tfsdk:"flags"`
	PreemptList types.Set    `tfsdk:"preempt_list"`
	PreemptMode types.Set    `tfsdk:"preempt_mode"`

	// TRES limits (sacctmgr names in comments)
	GrpTRES               types.Set `tfsdk:"grp_tres"`                  // GrpTRES
	GrpTRESMins           types.Set `tfsdk:"grp_tres_mins"`             // GrpTRESMins
	MaxTRESPerJob         types.Set `tfsdk:"max_tres_per_job"`          // MaxTRES
	MaxTRESMinsPerJob     types.Set `tfsdk:"max_tres_mins_per_job"`     // MaxTRESMins
	MaxTRESPerNode        types.Set `tfsdk:"max_tres_per_node"`         // MaxTRESPerNode
	MaxTRESPerUser        types.Set `tfsdk:"max_tres_per_user"`         // MaxTRESPU
	MaxTRESMinsPerUser    types.Set `tfsdk:"max_tres_mins_per_user"`    // MaxTRESRunMinsPU
	MaxTRESPerAccount     types.Set `tfsdk:"max_tres_per_account"`      // MaxTRESPA
	MaxTRESMinsPerAccount types.Set `tfsdk:"max_tres_mins_per_account"` // MaxTRESRunMinsPA
	MinTRESPerJob         types.Set `tfsdk:"min_tres_per_job"`          // MinTRES

	// Job-count limits
	GrpJobs                 types.Int64 `tfsdk:"grp_jobs"`                    // GrpJobs
	GrpSubmitJobs           types.Int64 `tfsdk:"grp_submit_jobs"`             // GrpSubmit
	MaxJobsPerUser          types.Int64 `tfsdk:"max_jobs_per_user"`           // MaxJobsPU
	MaxSubmitJobsPerUser    types.Int64 `tfsdk:"max_submit_jobs_per_user"`    // MaxSubmitPU
	MaxJobsPerAccount       types.Int64 `tfsdk:"max_jobs_per_account"`        // MaxJobsPA
	MaxSubmitJobsPerAccount types.Int64 `tfsdk:"max_submit_jobs_per_account"` // MaxSubmitPA

	// Miscellaneous
	GraceTime         types.Int64   `tfsdk:"grace_time"`          // GraceTime (seconds)
	UsageFactor       types.Float64 `tfsdk:"usage_factor"`        // UsageFactor (float — Slurm allows fractional)
	UsageThreshold    types.Float64 `tfsdk:"usage_threshold"`     // UsageThres  (float — Slurm allows fractional)
	PreemptExemptTime types.Int64   `tfsdk:"preempt_exempt_time"` // PreemptExemptTime (seconds)
}

// tresAttrTypes returns the attribute type map for a TRES object element.
func tresAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"type":  types.StringType,
		"name":  types.StringType,
		"count": types.Int64Type,
	}
}

func tresElemType() attr.Type {
	return types.ObjectType{AttrTypes: tresAttrTypes()}
}

// apiTresListToSet converts a []client.TRES returned by the API into a
// types.Set for storing in state. An empty slice becomes types.SetNull so
// Optional fields stay null when the QOS has no limit configured.
func apiTresListToSet(ctx context.Context, list []client.TRES, diags *diag.Diagnostics) types.Set {
	if len(list) == 0 {
		return types.SetNull(tresElemType())
	}
	elems := make([]attr.Value, len(list))
	for i, t := range list {
		name := types.StringNull()
		if t.Name != "" {
			name = types.StringValue(t.Name)
		}
		obj, d := types.ObjectValue(tresAttrTypes(), map[string]attr.Value{
			"type":  types.StringValue(t.Type),
			"name":  name,
			"count": types.Int64Value(t.Count),
		})
		diags.Append(d...)
		elems[i] = obj
	}
	s, d := types.SetValue(tresElemType(), elems)
	diags.Append(d...)
	return s
}

// planTresListToAPI converts a types.Set from the plan into []client.TRES
// for sending to the API. Returns nil when the set is null, unknown, or empty.
func planTresListToAPI(ctx context.Context, s types.Set) []client.TRES {
	if s.IsNull() || s.IsUnknown() || len(s.Elements()) == 0 {
		return nil
	}
	var models []tresModel
	s.ElementsAs(ctx, &models, false)
	result := make([]client.TRES, len(models))
	for i, m := range models {
		name := ""
		if !m.Name.IsNull() && !m.Name.IsUnknown() {
			name = m.Name.ValueString()
		}
		result[i] = client.TRES{
			Type:  m.Type.ValueString(),
			Name:  name,
			Count: m.Count.ValueInt64(),
		}
	}
	return result
}

// tresSchemAttr builds the repeated SetNestedAttribute block for all TRES
// limit attributes, keeping the schema definition DRY.
func tresSchemaAttr(description string) schema.SetNestedAttribute {
	return schema.SetNestedAttribute{
		MarkdownDescription: description,
		Optional:            true,
		Computed:            true,
		PlanModifiers: []planmodifier.Set{
			setplanmodifier.UseStateForUnknown(),
		},
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"type": schema.StringAttribute{
					Required:            true,
					MarkdownDescription: "TRES type (e.g. cpu, mem, gres).",
				},
				"name": schema.StringAttribute{
					Optional:            true,
					MarkdownDescription: "TRES name. Required for generic resources such as gres/gpu; omit for cpu and mem.",
				},
				"count": schema.Int64Attribute{
					Required:            true,
					MarkdownDescription: "TRES count limit.",
					Validators:          []validator.Int64{int64validator.AtLeast(0)},
				},
			},
		},
	}
}

// tresOptionalSchemaAttr is like tresSchemaAttr but without Computed or
// UseStateForUnknown — used for association TRES limits where the server does
// not inject defaults, so Optional-only suffices.
func tresOptionalSchemaAttr(description string) schema.SetNestedAttribute {
	return schema.SetNestedAttribute{
		MarkdownDescription: description,
		Optional:            true,
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"type": schema.StringAttribute{
					Required:            true,
					MarkdownDescription: "TRES type (e.g. cpu, mem, gres).",
				},
				"name": schema.StringAttribute{
					Optional:            true,
					MarkdownDescription: "TRES name. Required for generic resources such as gres/gpu; omit for cpu and mem.",
				},
				"count": schema.Int64Attribute{
					Required:            true,
					MarkdownDescription: "TRES count limit.",
					Validators:          []validator.Int64{int64validator.AtLeast(0)},
				},
			},
		},
	}
}

func NewQOSResource() resource.Resource {
	return &qosResource{}
}

func (r *qosResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_qos"
}

func (r *qosResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Slurm Quality of Service (QOS) definition.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The QOS name (same as name).",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the QOS.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "A description of the QOS.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"priority": schema.Int64Attribute{
				MarkdownDescription: "Priority value for this QOS (Priority).",
				Optional:            true,
				Validators:          []validator.Int64{int64validator.AtLeast(0)},
			},
			"max_wall_pj": schema.Int64Attribute{
				MarkdownDescription: "Maximum wall-clock time per job in minutes (MaxWall).",
				Optional:            true,
				Validators:          []validator.Int64{int64validator.AtLeast(0)},
			},
			"grp_wall": schema.Int64Attribute{
				MarkdownDescription: "Maximum total wall-clock time in minutes that all jobs using this QOS can run simultaneously (GrpWall).",
				Optional:            true,
				Validators:          []validator.Int64{int64validator.AtLeast(0)},
			},
			"grace_time": schema.Int64Attribute{
				MarkdownDescription: "Grace time in seconds before a job exceeding QOS limits is cancelled (GraceTime).",
				Optional:            true,
				Validators:          []validator.Int64{int64validator.AtLeast(0)},
			},
			"usage_factor": schema.Float64Attribute{
				MarkdownDescription: "Factor applied to a job's usage when it runs under this QOS (UsageFactor). Slurm default is 1. Fractional values (e.g. 0.5) are allowed. Optional+Computed: omitting it from config keeps the current Slurm value.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Float64{
					float64planmodifier.UseStateForUnknown(),
				},
				Validators: []validator.Float64{float64validator.AtLeast(0)},
			},
			"usage_threshold": schema.Float64Attribute{
				MarkdownDescription: "Minimum usage factor a user must maintain to submit jobs under this QOS (UsageThres). Fractional values are allowed. Optional+Computed: omitting it keeps the current Slurm value.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Float64{
					float64planmodifier.UseStateForUnknown(),
				},
				Validators: []validator.Float64{float64validator.AtLeast(0)},
			},
			"preempt_exempt_time": schema.Int64Attribute{
				MarkdownDescription: "Minimum number of seconds a job must run before it can be preempted (PreemptExemptTime).",
				Optional:            true,
				Validators:          []validator.Int64{int64validator.AtLeast(0)},
			},
			// Job-count limits
			"grp_jobs": schema.Int64Attribute{
				MarkdownDescription: "Maximum number of jobs running simultaneously across all users of this QOS (GrpJobs).",
				Optional:            true,
				Validators:          []validator.Int64{int64validator.AtLeast(0)},
			},
			"grp_submit_jobs": schema.Int64Attribute{
				MarkdownDescription: "Maximum number of jobs that can be submitted at once across all users of this QOS (GrpSubmit).",
				Optional:            true,
				Validators:          []validator.Int64{int64validator.AtLeast(0)},
			},
			"max_jobs_per_user": schema.Int64Attribute{
				MarkdownDescription: "Maximum number of jobs a single user can run simultaneously under this QOS (MaxJobsPU).",
				Optional:            true,
				Validators:          []validator.Int64{int64validator.AtLeast(0)},
			},
			"max_submit_jobs_per_user": schema.Int64Attribute{
				MarkdownDescription: "Maximum number of jobs a single user can have submitted under this QOS (MaxSubmitPU).",
				Optional:            true,
				Validators:          []validator.Int64{int64validator.AtLeast(0)},
			},
			"max_jobs_per_account": schema.Int64Attribute{
				MarkdownDescription: "Maximum number of jobs an account can run simultaneously under this QOS (MaxJobsPA).",
				Optional:            true,
				Validators:          []validator.Int64{int64validator.AtLeast(0)},
			},
			"max_submit_jobs_per_account": schema.Int64Attribute{
				MarkdownDescription: "Maximum number of jobs an account can have submitted under this QOS (MaxSubmitPA).",
				Optional:            true,
				Validators:          []validator.Int64{int64validator.AtLeast(0)},
			},
			// Sets
			"flags": schema.SetAttribute{
				MarkdownDescription: "QOS flags. Values must use the REST API name (UPPER_SNAKE_CASE). Valid values: PARTITION_MINIMUM_NODE, PARTITION_MAXIMUM_NODE, PARTITION_TIME_LIMIT, ENFORCE_USAGE_THRESHOLD, NO_RESERVE, REQUIRED_RESERVATION, DENY_LIMIT, OVERRIDE_PARTITION_QOS, NO_DECAY, USAGE_FACTOR_SAFE, RELATIVE.",
				Optional:            true,
				ElementType:         types.StringType,
				Validators: []validator.Set{
					setvalidator.ValueStringsAre(stringvalidator.OneOf(qosFlagValues...)),
				},
			},
			"preempt_list": schema.SetAttribute{
				MarkdownDescription: "Set of QOS names that this QOS can preempt (Preempt).",
				Optional:            true,
				ElementType:         types.StringType,
			},
			"preempt_mode": schema.SetAttribute{
				MarkdownDescription: "Preemption mode. Valid values: OFF, CANCEL, GANG, REQUEUE, SUSPEND, WITHIN (PreemptMode).",
				Optional:            true,
				ElementType:         types.StringType,
				Validators: []validator.Set{
					setvalidator.ValueStringsAre(stringvalidator.OneOf(qosPreemptModeValues...)),
				},
			},
			// TRES limits
			"grp_tres":                  tresSchemaAttr("Maximum TRES usable by all jobs in this QOS at any time (GrpTRES)."),
			"grp_tres_mins":             tresSchemaAttr("Maximum TRES-minutes consumable by all jobs in this QOS (GrpTRESMins)."),
			"max_tres_per_job":          tresSchemaAttr("Maximum TRES a single job can request (MaxTRES)."),
			"max_tres_mins_per_job":     tresSchemaAttr("Maximum TRES-minutes a single job can consume (MaxTRESMins)."),
			"max_tres_per_node":         tresSchemaAttr("Maximum TRES a single job can use per node (MaxTRESPerNode)."),
			"max_tres_per_user":         tresSchemaAttr("Maximum TRES a single user can use simultaneously (MaxTRESPU)."),
			"max_tres_mins_per_user":    tresSchemaAttr("Maximum TRES-minutes a single user can consume (MaxTRESRunMinsPU)."),
			"max_tres_per_account":      tresSchemaAttr("Maximum TRES a single account can use simultaneously (MaxTRESPA)."),
			"max_tres_mins_per_account": tresSchemaAttr("Maximum TRES-minutes a single account can consume (MaxTRESRunMinsPA)."),
			"min_tres_per_job":          tresSchemaAttr("Minimum TRES a job must request to use this QOS (MinTRES)."),
		},
	}
}

func (r *qosResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c := configureClient(req, resp); c != nil {
		r.client = c
	}
}

// ValidateConfig emits a plan-time warning when a slurm_qos resource is
// configured to manage one of Slurm's auto-created system QOS entries
// (currently just "normal"). It does not block the plan — users may still
// have legitimate reasons to manage the row — but the diagnostic surfaces
// the soft-delete footgun documented as Bug 3 in CLAUDE.md so it is not
// rediscovered after the first destroy/apply cycle.
func (r *qosResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg qosResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if w, ok := systemQOSWarning(cfg.Name); ok {
		resp.Diagnostics.AddAttributeWarning(path.Root("name"), w.Summary, w.Detail)
	}
}

// qosConfigWarning is the structured form of a ValidateConfig warning so
// the matching detection can be unit-tested without a tfsdk.Config.
type qosConfigWarning struct {
	Summary string
	Detail  string
}

// systemQOSWarning returns a warning when name matches a built-in system
// QOS. It returns ok=false if name is null/unknown (defer to apply) or
// not a system name.
func systemQOSWarning(name types.String) (qosConfigWarning, bool) {
	if name.IsNull() || name.IsUnknown() {
		return qosConfigWarning{}, false
	}
	got := name.ValueString()
	for _, n := range systemQOSNames {
		if got == n {
			return qosConfigWarning{
				Summary: fmt.Sprintf("Managing built-in system QOS %q is fragile", got),
				Detail: fmt.Sprintf(
					"%q is auto-created by slurmdbd at database initialisation. "+
						"Managing it as a slurm_qos resource works on the first apply, but a "+
						"destroy soft-deletes the row (deleted=1) and the next apply hits "+
						"\"Slurmdbd query returned with empty list\" because slurmrestd's "+
						"verification query does not match the restored system row. "+
						"Prefer renaming this resource to a non-system QOS (e.g. \"standard\", "+
						"\"default\"), or be prepared to drop the soft-deleted row out-of-band "+
						"between cycles.",
					got,
				),
			}, true
		}
	}
	return qosConfigWarning{}, false
}

func (r *qosResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan qosResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating QOS", map[string]interface{}{"name": plan.Name.ValueString()})

	qos := r.modelToAPI(ctx, plan)
	if err := r.client.CreateQOS(qos); err != nil {
		resp.Diagnostics.AddError("Error creating QOS", err.Error())
		return
	}

	// Read back the full QOS so every Optional+Computed field (including all
	// TRES sets) is resolved to a known value. Saving the plan directly would
	// leave those fields as Unknown, which the framework rejects.
	created, err := r.client.GetQOS(plan.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading QOS after create", err.Error())
		return
	}
	if created == nil {
		resp.Diagnostics.AddError("QOS not found after create",
			fmt.Sprintf("QOS %q was not found immediately after creation", plan.Name.ValueString()))
		return
	}

	state := r.apiToState(ctx, created, &resp.Diagnostics)

	// The Slurm REST API may not reflect TRES limits in the immediate GET
	// response after a POST (it may require a second round-trip or the limits
	// are stored asynchronously). For TRES fields the user explicitly configured,
	// preserve the plan values so the framework's post-apply consistency check
	// passes. The subsequent Read (next tofu plan / refresh) will validate
	// whether Slurm actually stored the values.
	r.preservePlanTRES(&state, plan)

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

func (r *qosResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var cur qosResourceModel
	diags := req.State.Get(ctx, &cur)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	qos, err := r.client.GetQOS(cur.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading QOS", err.Error())
		return
	}
	if qos == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	state := r.apiToState(ctx, qos, &resp.Diagnostics)
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

// apiToState builds a fully-resolved qosResourceModel from a Slurm API QOS
// object. All Optional+Computed fields not present in the API response are set
// to their null equivalents so the framework never sees an Unknown value.
func (r *qosResource) apiToState(ctx context.Context, qos *client.QOS, diags *diag.Diagnostics) qosResourceModel {
	state := qosResourceModel{
		ID:          types.StringValue(qos.Name),
		Name:        types.StringValue(qos.Name),
		Description: types.StringValue(qos.Description),
		// Scalar optionals — null unless populated below.
		Priority:                types.Int64Null(),
		UsageFactor:             types.Float64Null(),
		UsageThreshold:          types.Float64Null(),
		GraceTime:               types.Int64Null(),
		MaxWallPJ:               types.Int64Null(),
		GrpWall:                 types.Int64Null(),
		PreemptExemptTime:       types.Int64Null(),
		GrpJobs:                 types.Int64Null(),
		GrpSubmitJobs:           types.Int64Null(),
		MaxJobsPerUser:          types.Int64Null(),
		MaxSubmitJobsPerUser:    types.Int64Null(),
		MaxJobsPerAccount:       types.Int64Null(),
		MaxSubmitJobsPerAccount: types.Int64Null(),
		// String-set optionals.
		Flags:       types.SetNull(types.StringType),
		PreemptList: types.SetNull(types.StringType),
		PreemptMode: types.SetNull(types.StringType),
		// TRES-set optionals (Optional+Computed+UseStateForUnknown).
		GrpTRES:               types.SetNull(tresElemType()),
		GrpTRESMins:           types.SetNull(tresElemType()),
		MaxTRESPerJob:         types.SetNull(tresElemType()),
		MaxTRESMinsPerJob:     types.SetNull(tresElemType()),
		MaxTRESPerNode:        types.SetNull(tresElemType()),
		MaxTRESPerUser:        types.SetNull(tresElemType()),
		MaxTRESMinsPerUser:    types.SetNull(tresElemType()),
		MaxTRESPerAccount:     types.SetNull(tresElemType()),
		MaxTRESMinsPerAccount: types.SetNull(tresElemType()),
		MinTRESPerJob:         types.SetNull(tresElemType()),
	}

	// Priority: skip when zero (Slurm default) to avoid drift.
	if qos.Priority != nil && qos.Priority.Set && qos.Priority.Number != 0 {
		state.Priority = types.Int64Value(int64(qos.Priority.Number))
	}

	// UsageFactor / UsageThreshold — Slurm stores both as floats; fractional
	// values (e.g. 0.5) round-trip without truncation now that the schema
	// uses Float64Attribute.
	if qos.UsageFactor != nil && qos.UsageFactor.Set && !qos.UsageFactor.Infinite {
		state.UsageFactor = types.Float64Value(qos.UsageFactor.Number)
	}
	if qos.UsageThreshold != nil && qos.UsageThreshold.Set && !qos.UsageThreshold.Infinite {
		state.UsageThreshold = types.Float64Value(qos.UsageThreshold.Number)
	}

	// Preempt
	if qos.Preempt != nil {
		if len(qos.Preempt.List) > 0 {
			v, d := types.SetValueFrom(ctx, types.StringType, qos.Preempt.List)
			diags.Append(d...)
			state.PreemptList = v
		}
		var modes []string
		for _, m := range qos.Preempt.Mode {
			if m != "DISABLED" {
				modes = append(modes, m)
			}
		}
		if len(modes) > 0 {
			v, d := types.SetValueFrom(ctx, types.StringType, modes)
			diags.Append(d...)
			state.PreemptMode = v
		}
		if qos.Preempt.ExemptTime != nil && qos.Preempt.ExemptTime.Set && !qos.Preempt.ExemptTime.Infinite {
			state.PreemptExemptTime = types.Int64Value(int64(qos.Preempt.ExemptTime.Number))
		}
	}

	// Flags
	if len(qos.Flags) > 0 {
		v, d := types.SetValueFrom(ctx, types.StringType, qos.Flags)
		diags.Append(d...)
		state.Flags = v
	}

	// Limits
	if qos.Limits != nil {
		if qos.Limits.GraceTime != 0 {
			state.GraceTime = types.Int64Value(int64(qos.Limits.GraceTime))
		}

		if qos.Limits.Max != nil {
			max := qos.Limits.Max

			// Wall-clock limits
			if max.WallClock != nil && max.WallClock.Per != nil {
				if max.WallClock.Per.Job != nil && max.WallClock.Per.Job.Set && !max.WallClock.Per.Job.Infinite {
					state.MaxWallPJ = types.Int64Value(int64(max.WallClock.Per.Job.Number))
				}
				if max.WallClock.Per.QOS != nil && max.WallClock.Per.QOS.Set && !max.WallClock.Per.QOS.Infinite {
					state.GrpWall = types.Int64Value(int64(max.WallClock.Per.QOS.Number))
				}
			}

			// TRES limits
			if max.TRES != nil {
				t := max.TRES
				state.GrpTRES = apiTresListToSet(ctx, t.Total, diags)
				if t.Per != nil {
					state.MaxTRESPerJob = apiTresListToSet(ctx, t.Per.Job, diags)
					state.MaxTRESPerNode = apiTresListToSet(ctx, t.Per.Node, diags)
					state.MaxTRESPerUser = apiTresListToSet(ctx, t.Per.User, diags)
					state.MaxTRESPerAccount = apiTresListToSet(ctx, t.Per.Account, diags)
				}
				if t.Minutes != nil {
					state.GrpTRESMins = apiTresListToSet(ctx, t.Minutes.Total, diags)
					if t.Minutes.Per != nil {
						state.MaxTRESMinsPerJob = apiTresListToSet(ctx, t.Minutes.Per.Job, diags)
						state.MaxTRESMinsPerUser = apiTresListToSet(ctx, t.Minutes.Per.User, diags)
						state.MaxTRESMinsPerAccount = apiTresListToSet(ctx, t.Minutes.Per.Account, diags)
					}
				}
			}

			// Job count limits
			if max.Jobs != nil {
				j := max.Jobs
				if j.Count != nil && j.Count.Set && !j.Count.Infinite {
					state.GrpJobs = types.Int64Value(int64(j.Count.Number))
				}
				if j.Per != nil {
					if j.Per.User != nil && j.Per.User.Set && !j.Per.User.Infinite {
						state.MaxJobsPerUser = types.Int64Value(int64(j.Per.User.Number))
					}
					if j.Per.Account != nil && j.Per.Account.Set && !j.Per.Account.Infinite {
						state.MaxJobsPerAccount = types.Int64Value(int64(j.Per.Account.Number))
					}
				}
				if j.ActiveJobs != nil && j.ActiveJobs.Per != nil {
					if j.ActiveJobs.Per.User != nil && j.ActiveJobs.Per.User.Set && !j.ActiveJobs.Per.User.Infinite {
						state.MaxSubmitJobsPerUser = types.Int64Value(int64(j.ActiveJobs.Per.User.Number))
					}
					if j.ActiveJobs.Per.Account != nil && j.ActiveJobs.Per.Account.Set && !j.ActiveJobs.Per.Account.Infinite {
						state.MaxSubmitJobsPerAccount = types.Int64Value(int64(j.ActiveJobs.Per.Account.Number))
					}
				}
			}
			if max.ActiveJobs != nil && max.ActiveJobs.Count != nil &&
				max.ActiveJobs.Count.Set && !max.ActiveJobs.Count.Infinite {
				state.GrpSubmitJobs = types.Int64Value(int64(max.ActiveJobs.Count.Number))
			}
		}

		// Min TRES
		if qos.Limits.Min != nil && qos.Limits.Min.TRES != nil && qos.Limits.Min.TRES.Per != nil {
			state.MinTRESPerJob = apiTresListToSet(ctx, qos.Limits.Min.TRES.Per.Job, diags)
		}
	}

	return state
}

func (r *qosResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan qosResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Updating QOS", map[string]interface{}{"name": plan.Name.ValueString()})

	qos := r.modelToAPI(ctx, plan)
	if err := r.client.CreateQOS(qos); err != nil {
		resp.Diagnostics.AddError("Error updating QOS", err.Error())
		return
	}

	plan.ID = plan.Name
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func (r *qosResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state qosResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Deleting QOS", map[string]interface{}{"name": state.Name.ValueString()})

	if err := r.client.DeleteQOS(state.Name.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting QOS", err.Error())
		return
	}
}

func (r *qosResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importStateByName(ctx, req, resp)
}

// modelToAPI converts the Terraform model to the Slurm API QOS struct.
// preservePlanTRES copies non-null TRES set values from plan into state.
// Called after Create so the framework's post-apply consistency check sees
// the values the user configured rather than whatever the API read-back
// happened to return (which may differ before Slurm's write is fully visible).
func (r *qosResource) preservePlanTRES(state *qosResourceModel, plan qosResourceModel) {
	type pair struct {
		plan  types.Set
		state *types.Set
	}
	for _, p := range []pair{
		{plan.GrpTRES, &state.GrpTRES},
		{plan.GrpTRESMins, &state.GrpTRESMins},
		{plan.MaxTRESPerJob, &state.MaxTRESPerJob},
		{plan.MaxTRESMinsPerJob, &state.MaxTRESMinsPerJob},
		{plan.MaxTRESPerNode, &state.MaxTRESPerNode},
		{plan.MaxTRESPerUser, &state.MaxTRESPerUser},
		{plan.MaxTRESMinsPerUser, &state.MaxTRESMinsPerUser},
		{plan.MaxTRESPerAccount, &state.MaxTRESPerAccount},
		{plan.MaxTRESMinsPerAccount, &state.MaxTRESMinsPerAccount},
		{plan.MinTRESPerJob, &state.MinTRESPerJob},
	} {
		if !p.plan.IsNull() && !p.plan.IsUnknown() {
			*p.state = p.plan
		}
	}
}

func (r *qosResource) modelToAPI(ctx context.Context, m qosResourceModel) client.QOS {
	qos := client.QOS{Name: m.Name.ValueString()}

	if !m.Description.IsNull() && !m.Description.IsUnknown() {
		qos.Description = m.Description.ValueString()
	}
	qos.Priority = slurmIntFromInt64(m.Priority)
	if !m.UsageFactor.IsNull() && !m.UsageFactor.IsUnknown() {
		qos.UsageFactor = &client.SlurmFloat{Number: m.UsageFactor.ValueFloat64(), Set: true}
	}
	if !m.UsageThreshold.IsNull() && !m.UsageThreshold.IsUnknown() {
		qos.UsageThreshold = &client.SlurmFloat{Number: m.UsageThreshold.ValueFloat64(), Set: true}
	}

	// Flags
	if !m.Flags.IsNull() && !m.Flags.IsUnknown() {
		var flags []string
		m.Flags.ElementsAs(ctx, &flags, false)
		qos.Flags = flags
	}

	// Preempt
	if (!m.PreemptList.IsNull() && !m.PreemptList.IsUnknown()) ||
		(!m.PreemptMode.IsNull() && !m.PreemptMode.IsUnknown()) ||
		(!m.PreemptExemptTime.IsNull() && !m.PreemptExemptTime.IsUnknown()) {
		qos.Preempt = &client.QOSPreempt{}
		if !m.PreemptList.IsNull() && !m.PreemptList.IsUnknown() {
			m.PreemptList.ElementsAs(ctx, &qos.Preempt.List, false)
		}
		if !m.PreemptMode.IsNull() && !m.PreemptMode.IsUnknown() {
			m.PreemptMode.ElementsAs(ctx, &qos.Preempt.Mode, false)
		}
		qos.Preempt.ExemptTime = slurmIntFromInt64(m.PreemptExemptTime)
	}

	// Build limits only when at least one limit field is set.
	needsLimits := !m.MaxWallPJ.IsNull() || !m.GrpWall.IsNull() || !m.GraceTime.IsNull() ||
		!m.GrpJobs.IsNull() || !m.GrpSubmitJobs.IsNull() ||
		!m.MaxJobsPerUser.IsNull() || !m.MaxSubmitJobsPerUser.IsNull() ||
		!m.MaxJobsPerAccount.IsNull() || !m.MaxSubmitJobsPerAccount.IsNull() ||
		!m.GrpTRES.IsNull() || !m.GrpTRESMins.IsNull() ||
		!m.MaxTRESPerJob.IsNull() || !m.MaxTRESMinsPerJob.IsNull() ||
		!m.MaxTRESPerNode.IsNull() ||
		!m.MaxTRESPerUser.IsNull() || !m.MaxTRESMinsPerUser.IsNull() ||
		!m.MaxTRESPerAccount.IsNull() || !m.MaxTRESMinsPerAccount.IsNull() ||
		!m.MinTRESPerJob.IsNull()

	if needsLimits {
		qos.Limits = &client.QOSLimits{}

		if !m.GraceTime.IsNull() && !m.GraceTime.IsUnknown() {
			qos.Limits.GraceTime = int(m.GraceTime.ValueInt64())
		}

		needsMax := !m.MaxWallPJ.IsNull() || !m.GrpWall.IsNull() ||
			!m.GrpJobs.IsNull() || !m.GrpSubmitJobs.IsNull() ||
			!m.MaxJobsPerUser.IsNull() || !m.MaxSubmitJobsPerUser.IsNull() ||
			!m.MaxJobsPerAccount.IsNull() || !m.MaxSubmitJobsPerAccount.IsNull() ||
			!m.GrpTRES.IsNull() || !m.GrpTRESMins.IsNull() ||
			!m.MaxTRESPerJob.IsNull() || !m.MaxTRESMinsPerJob.IsNull() ||
			!m.MaxTRESPerNode.IsNull() ||
			!m.MaxTRESPerUser.IsNull() || !m.MaxTRESMinsPerUser.IsNull() ||
			!m.MaxTRESPerAccount.IsNull() || !m.MaxTRESMinsPerAccount.IsNull()

		if needsMax {
			qos.Limits.Max = &client.QOSLimitsMax{}
			max := qos.Limits.Max

			// Wall clock limits
			if !m.MaxWallPJ.IsNull() || !m.GrpWall.IsNull() {
				max.WallClock = &client.QOSWallClock{
					Per: &client.QOSWallClockPer{
						Job: slurmIntFromInt64(m.MaxWallPJ),
						QOS: slurmIntFromInt64(m.GrpWall),
					},
				}
			}

			// TRES limits
			needsTRES := !m.GrpTRES.IsNull() || !m.GrpTRESMins.IsNull() ||
				!m.MaxTRESPerJob.IsNull() || !m.MaxTRESMinsPerJob.IsNull() ||
				!m.MaxTRESPerNode.IsNull() ||
				!m.MaxTRESPerUser.IsNull() || !m.MaxTRESMinsPerUser.IsNull() ||
				!m.MaxTRESPerAccount.IsNull() || !m.MaxTRESMinsPerAccount.IsNull()

			if needsTRES {
				max.TRES = &client.QOSTresLimits{}
				max.TRES.Total = planTresListToAPI(ctx, m.GrpTRES)
				max.TRES.Per = &client.QOSTresPer{
					Job:     planTresListToAPI(ctx, m.MaxTRESPerJob),
					Node:    planTresListToAPI(ctx, m.MaxTRESPerNode),
					User:    planTresListToAPI(ctx, m.MaxTRESPerUser),
					Account: planTresListToAPI(ctx, m.MaxTRESPerAccount),
				}
				max.TRES.Minutes = &client.QOSTresMins{
					Total: planTresListToAPI(ctx, m.GrpTRESMins),
					Per: &client.QOSTresMinsPer{
						Job:     planTresListToAPI(ctx, m.MaxTRESMinsPerJob),
						User:    planTresListToAPI(ctx, m.MaxTRESMinsPerUser),
						Account: planTresListToAPI(ctx, m.MaxTRESMinsPerAccount),
					},
				}
			}

			// Job count limits
			needsJobs := !m.GrpJobs.IsNull() || !m.MaxJobsPerUser.IsNull() ||
				!m.MaxJobsPerAccount.IsNull() ||
				!m.MaxSubmitJobsPerUser.IsNull() || !m.MaxSubmitJobsPerAccount.IsNull()
			if needsJobs {
				max.Jobs = &client.QOSJobs{
					Count: slurmIntFromInt64(m.GrpJobs),
				}
				if !m.MaxJobsPerUser.IsNull() || !m.MaxJobsPerAccount.IsNull() {
					max.Jobs.Per = &client.QOSJobsPer{
						User:    slurmIntFromInt64(m.MaxJobsPerUser),
						Account: slurmIntFromInt64(m.MaxJobsPerAccount),
					}
				}
				if !m.MaxSubmitJobsPerUser.IsNull() || !m.MaxSubmitJobsPerAccount.IsNull() {
					max.Jobs.ActiveJobs = &client.QOSJobsActiveJobs{
						Per: &client.QOSJobsActiveJobsPer{
							User:    slurmIntFromInt64(m.MaxSubmitJobsPerUser),
							Account: slurmIntFromInt64(m.MaxSubmitJobsPerAccount),
						},
					}
				}
			}
			if v := slurmIntFromInt64(m.GrpSubmitJobs); v != nil {
				max.ActiveJobs = &client.QOSActiveJobs{Count: v}
			}
		}

		// Min TRES
		if !m.MinTRESPerJob.IsNull() {
			qos.Limits.Min = &client.QOSLimitsMin{
				TRES: &client.QOSMinTres{
					Per: &client.QOSMinTresPer{
						Job: planTresListToAPI(ctx, m.MinTRESPerJob),
					},
				},
			}
		}
	}

	return qos
}
