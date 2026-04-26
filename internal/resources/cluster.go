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

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &clusterResource{}
	_ resource.ResourceWithImportState = &clusterResource{}
)

// clusterResource is the resource implementation.
type clusterResource struct {
	client *client.Client
}

// clusterResourceModel maps the resource schema to a Go struct.
type clusterResourceModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
}

// NewClusterResource is a constructor that returns a new resource instance.
func NewClusterResource() resource.Resource {
	return &clusterResource{}
}

func (r *clusterResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster"
}

func (r *clusterResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Slurm cluster registration in the accounting database.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The cluster name (same as name).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the Slurm cluster.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *clusterResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *clusterResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan clusterResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating cluster", map[string]interface{}{
		"name": plan.Name.ValueString(),
	})

	err := r.client.CreateCluster(client.Cluster{
		Name: plan.Name.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating cluster",
			"Could not create cluster: "+err.Error(),
		)
		return
	}

	plan.ID = plan.Name

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func (r *clusterResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state clusterResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	cluster, err := r.client.GetCluster(state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading cluster",
			"Could not read cluster: "+err.Error(),
		)
		return
	}

	if cluster == nil {
		// Resource no longer exists
		resp.State.RemoveResource(ctx)
		return
	}

	state.Name = types.StringValue(cluster.Name)
	state.ID = types.StringValue(cluster.Name)

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

func (r *clusterResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Cluster name is the only attribute and it requires replace,
	// so Update should never be called.
	resp.Diagnostics.AddError(
		"Unexpected update",
		"Cluster resources cannot be updated in place. Name change requires replacement.",
	)
}

func (r *clusterResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state clusterResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Deleting cluster", map[string]interface{}{
		"name": state.Name.ValueString(),
	})

	err := r.client.DeleteCluster(state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error deleting cluster",
			"Could not delete cluster: "+err.Error(),
		)
		return
	}
}

func (r *clusterResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// The import ID is the cluster name
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
