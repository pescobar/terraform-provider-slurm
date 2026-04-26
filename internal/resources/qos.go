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

	"github.com/pabloqc/terraform-provider-slurm/internal/client"
)

var (
	_ resource.Resource                = &qosResource{}
	_ resource.ResourceWithImportState = &qosResource{}
)

type qosResource struct {
	client *client.Client
}

type qosResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Priority    types.Int64  `tfsdk:"priority"`
	MaxWallPJ   types.Int64  `tfsdk:"max_wall_pj"`
	Flags       types.List   `tfsdk:"flags"`
	PreemptList types.List   `tfsdk:"preempt_list"`
	PreemptMode types.List   `tfsdk:"preempt_mode"`
}

func NewQOSResource() resource.Resource {
	return &qosResource{}
}

func (r *qosResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_qos"
}

func (r *qosResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Slurm Quality of Service (QOS) definition.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The QOS name (same as name).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the QOS.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"description": schema.StringAttribute{
				Description: "A description of the QOS.",
				Optional:    true,
				Computed:    true,
			},
			"priority": schema.Int64Attribute{
				Description: "The priority value for this QOS.",
				Optional:    true,
			},
			"max_wall_pj": schema.Int64Attribute{
				Description: "Maximum wall clock time per job in minutes.",
				Optional:    true,
			},
			"flags": schema.ListAttribute{
				Description: "QOS flags.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"preempt_list": schema.ListAttribute{
				Description: "List of QOS names that this QOS can preempt.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"preempt_mode": schema.ListAttribute{
				Description: "Preemption mode (e.g. CANCEL, REQUEUE).",
				Optional:    true,
				ElementType: types.StringType,
			},
		},
	}
}

func (r *qosResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *qosResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan qosResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating QOS", map[string]interface{}{
		"name": plan.Name.ValueString(),
	})

	qos := r.modelToAPI(ctx, plan)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.CreateQOS(qos); err != nil {
		resp.Diagnostics.AddError("Error creating QOS", err.Error())
		return
	}

	plan.ID = plan.Name

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func (r *qosResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state qosResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	qos, err := r.client.GetQOS(state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading QOS", err.Error())
		return
	}
	if qos == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	state.ID = types.StringValue(qos.Name)
	state.Name = types.StringValue(qos.Name)
	state.Description = types.StringValue(qos.Description)

	if qos.Priority != nil && qos.Priority.Set {
		state.Priority = types.Int64Value(int64(qos.Priority.Number))
	}
	if qos.Limits != nil && qos.Limits.Max != nil && qos.Limits.Max.WallClock != nil && qos.Limits.Max.WallClock.Per != nil && qos.Limits.Max.WallClock.Per.Job != nil && qos.Limits.Max.WallClock.Per.Job.Set {
		state.MaxWallPJ = types.Int64Value(int64(qos.Limits.Max.WallClock.Per.Job.Number))
	}
	if len(qos.Flags) > 0 {
		flagsVal, d := types.ListValueFrom(ctx, types.StringType, qos.Flags)
		resp.Diagnostics.Append(d...)
		state.Flags = flagsVal
	}
	if qos.Preempt != nil {
		if len(qos.Preempt.List) > 0 {
			preemptVal, d := types.ListValueFrom(ctx, types.StringType, qos.Preempt.List)
			resp.Diagnostics.Append(d...)
			state.PreemptList = preemptVal
		}
		if len(qos.Preempt.Mode) > 0 {
			modeVal, d := types.ListValueFrom(ctx, types.StringType, qos.Preempt.Mode)
			resp.Diagnostics.Append(d...)
			state.PreemptMode = modeVal
		}
	}

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

func (r *qosResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan qosResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Updating QOS", map[string]interface{}{
		"name": plan.Name.ValueString(),
	})

	qos := r.modelToAPI(ctx, plan)
	if resp.Diagnostics.HasError() {
		return
	}

	// Slurm uses POST for both create and update (upsert)
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

	tflog.Debug(ctx, "Deleting QOS", map[string]interface{}{
		"name": state.Name.ValueString(),
	})

	if err := r.client.DeleteQOS(state.Name.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting QOS", err.Error())
		return
	}
}

func (r *qosResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// modelToAPI converts the Terraform model to the Slurm API QOS struct.
func (r *qosResource) modelToAPI(ctx context.Context, model qosResourceModel) client.QOS {
	qos := client.QOS{
		Name: model.Name.ValueString(),
	}
	if !model.Description.IsNull() && !model.Description.IsUnknown() {
		qos.Description = model.Description.ValueString()
	}
	if !model.Priority.IsNull() && !model.Priority.IsUnknown() {
		qos.Priority = &client.SlurmInt{
			Number: int(model.Priority.ValueInt64()),
			Set:    true,
		}
	}
	if !model.MaxWallPJ.IsNull() && !model.MaxWallPJ.IsUnknown() {
		qos.Limits = &client.QOSLimits{
			Max: &client.QOSLimitsMax{
				WallClock: &client.QOSWallClock{
					Per: &client.QOSWallClockPer{
						Job: &client.SlurmInt{
							Number: int(model.MaxWallPJ.ValueInt64()),
							Set:    true,
						},
					},
				},
			},
		}
	}
	if !model.Flags.IsNull() && !model.Flags.IsUnknown() {
		var flags []string
		model.Flags.ElementsAs(ctx, &flags, false)
		qos.Flags = flags
	}

	if (!model.PreemptList.IsNull() && !model.PreemptList.IsUnknown()) ||
		(!model.PreemptMode.IsNull() && !model.PreemptMode.IsUnknown()) {
		qos.Preempt = &client.QOSPreempt{}
		if !model.PreemptList.IsNull() && !model.PreemptList.IsUnknown() {
			var list []string
			model.PreemptList.ElementsAs(ctx, &list, false)
			qos.Preempt.List = list
		}
		if !model.PreemptMode.IsNull() && !model.PreemptMode.IsUnknown() {
			var mode []string
			model.PreemptMode.ElementsAs(ctx, &mode, false)
			qos.Preempt.Mode = mode
		}
	}

	return qos
}
