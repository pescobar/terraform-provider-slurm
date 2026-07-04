package resources

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dsschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/pescobar/terraform-provider-slurm/internal/client"
)

var (
	_ datasource.DataSource              = &partitionDataSource{}
	_ datasource.DataSourceWithConfigure = &partitionDataSource{}
)

type partitionDataSource struct {
	client *client.Client
}

func NewPartitionDataSource() datasource.DataSource {
	return &partitionDataSource{}
}

func (d *partitionDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_partition"
}

// partitionDataSourceModel maps the data source schema.
type partitionDataSourceModel struct {
	ID                types.String `tfsdk:"id"`
	Name              types.String `tfsdk:"name"`
	Nodes             types.String `tfsdk:"nodes"`
	TotalNodes        types.Int64  `tfsdk:"total_nodes"`
	TotalCPUs         types.Int64  `tfsdk:"total_cpus"`
	State             types.Set    `tfsdk:"state"`
	Flags             types.Set    `tfsdk:"flags"`
	Alternate         types.String `tfsdk:"alternate"`
	AllowAccounts     types.Set    `tfsdk:"allow_accounts"`
	DenyAccounts      types.Set    `tfsdk:"deny_accounts"`
	AllowGroups       types.Set    `tfsdk:"allow_groups"`
	AllowQOS          types.Set    `tfsdk:"allow_qos"`
	DenyQOS           types.Set    `tfsdk:"deny_qos"`
	QOS               types.String `tfsdk:"qos"`
	MaxTime           types.Int64  `tfsdk:"max_time"`
	DefaultTime       types.Int64  `tfsdk:"default_time"`
	MaxNodesPerJob    types.Int64  `tfsdk:"max_nodes_per_job"`
	PriorityTier      types.Int64  `tfsdk:"priority_tier"`
	PriorityJobFactor types.Int64  `tfsdk:"priority_job_factor"`
	PreemptMode       types.Set    `tfsdk:"preempt_mode"`
	GraceTime         types.Int64  `tfsdk:"grace_time"`
}

func (d *partitionDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dsschema.Schema{
		MarkdownDescription: "Reads an existing Slurm partition by name from slurmctld. " +
			"Partitions are defined in `slurm.conf` (or created dynamically) and are not " +
			"managed by this provider — this data source lets HCL reference partition " +
			"names and properties, e.g. for the `partition` field of a `slurm_user` " +
			"association block, with validation that the partition actually exists.",
		Attributes: map[string]dsschema.Attribute{
			"id":   dsschema.StringAttribute{MarkdownDescription: "The partition name (same as name).", Computed: true},
			"name": dsschema.StringAttribute{MarkdownDescription: "The name of the partition to look up.", Required: true},

			"nodes":       dsschema.StringAttribute{MarkdownDescription: "Node expression associated with the partition (Nodes).", Computed: true},
			"total_nodes": dsschema.Int64Attribute{MarkdownDescription: "Number of nodes in the partition (TotalNodes).", Computed: true},
			"total_cpus":  dsschema.Int64Attribute{MarkdownDescription: "Number of CPUs in the partition (TotalCPUs).", Computed: true},
			"state":       dsschema.SetAttribute{MarkdownDescription: "Partition state (e.g. UP, DOWN, DRAIN, INACTIVE).", Computed: true, ElementType: types.StringType},
			"flags":       dsschema.SetAttribute{MarkdownDescription: "Partition flags (e.g. DEFAULT). Only exposed by API v0.0.45+ (Slurm 26.05); always null on older versions.", Computed: true, ElementType: types.StringType},
			"alternate":   dsschema.StringAttribute{MarkdownDescription: "Alternate partition used when this one is unavailable (Alternate).", Computed: true},

			"allow_accounts": dsschema.SetAttribute{MarkdownDescription: "Accounts allowed to use the partition (AllowAccounts). Null when unrestricted.", Computed: true, ElementType: types.StringType},
			"deny_accounts":  dsschema.SetAttribute{MarkdownDescription: "Accounts denied use of the partition (DenyAccounts).", Computed: true, ElementType: types.StringType},
			"allow_groups":   dsschema.SetAttribute{MarkdownDescription: "Unix groups allowed to use the partition (AllowGroups). Null when unrestricted.", Computed: true, ElementType: types.StringType},
			"allow_qos":      dsschema.SetAttribute{MarkdownDescription: "QOS allowed in the partition (AllowQos). Null when unrestricted.", Computed: true, ElementType: types.StringType},
			"deny_qos":       dsschema.SetAttribute{MarkdownDescription: "QOS denied in the partition (DenyQos).", Computed: true, ElementType: types.StringType},
			"qos":            dsschema.StringAttribute{MarkdownDescription: "QOS whose limits apply to all jobs in the partition (QOS).", Computed: true},

			"max_time":          dsschema.Int64Attribute{MarkdownDescription: "Maximum wall-clock time per job in minutes (MaxTime). Null when unlimited.", Computed: true},
			"default_time":      dsschema.Int64Attribute{MarkdownDescription: "Default wall-clock time per job in minutes (DefaultTime). Null when not set.", Computed: true},
			"max_nodes_per_job": dsschema.Int64Attribute{MarkdownDescription: "Maximum node count per job (MaxNodes). Null when unlimited.", Computed: true},

			"priority_tier":       dsschema.Int64Attribute{MarkdownDescription: "Scheduling priority tier (PriorityTier).", Computed: true},
			"priority_job_factor": dsschema.Int64Attribute{MarkdownDescription: "Job priority factor (PriorityJobFactor).", Computed: true},
			"preempt_mode":        dsschema.SetAttribute{MarkdownDescription: "Preemption mode of the partition (PreemptMode). Only exposed by API v0.0.45+ (Slurm 26.05); always null on older versions.", Computed: true, ElementType: types.StringType},
			"grace_time":          dsschema.Int64Attribute{MarkdownDescription: "Preemption grace time in seconds (GraceTime).", Computed: true},
		},
	}
}

func (d *partitionDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c := configureDataSourceClient(req, resp); c != nil {
		d.client = c
	}
}

// commaSetOrNull converts slurmctld's comma-separated ACL strings ("a,b,c")
// into a string set; the empty string (= unrestricted / unset) becomes null.
func commaSetOrNull(ctx context.Context, s string, diags *diag.Diagnostics) types.Set {
	if s == "" {
		return types.SetNull(types.StringType)
	}
	v, d := types.SetValueFrom(ctx, types.StringType, strings.Split(s, ","))
	diags.Append(d...)
	return v
}

// slurmIntOrNull maps Slurm's tri-state int to Int64: null when unset or
// infinite, else the number.
func slurmIntOrNull(si *client.SlurmInt) types.Int64 {
	if si == nil || !si.Set || si.Infinite {
		return types.Int64Null()
	}
	return types.Int64Value(int64(si.Number))
}

func (d *partitionDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg partitionDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := cfg.Name.ValueString()
	part, err := d.client.GetPartition(ctx, name)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading partition",
			fmt.Sprintf("Could not read partition %q: %s", name, err.Error()),
		)
		return
	}
	if part == nil {
		resp.Diagnostics.AddError(
			"Partition not found",
			fmt.Sprintf("No Slurm partition named %q exists.", name),
		)
		return
	}

	state := partitionDataSourceModel{
		ID:        types.StringValue(part.Name),
		Name:      types.StringValue(part.Name),
		Alternate: types.StringValue(part.Alternate),
		Nodes:     types.StringNull(),
		QOS:       types.StringNull(),

		TotalNodes:        types.Int64Value(0),
		TotalCPUs:         types.Int64Value(0),
		PriorityTier:      types.Int64Value(0),
		PriorityJobFactor: types.Int64Value(0),
		GraceTime:         types.Int64Value(part.GraceTime),

		MaxTime:        types.Int64Null(),
		DefaultTime:    types.Int64Null(),
		MaxNodesPerJob: types.Int64Null(),

		State:         types.SetNull(types.StringType),
		Flags:         types.SetNull(types.StringType),
		PreemptMode:   types.SetNull(types.StringType),
		AllowAccounts: types.SetNull(types.StringType),
		DenyAccounts:  types.SetNull(types.StringType),
		AllowGroups:   types.SetNull(types.StringType),
		AllowQOS:      types.SetNull(types.StringType),
		DenyQOS:       types.SetNull(types.StringType),
	}

	if part.Nodes != nil {
		state.Nodes = types.StringValue(part.Nodes.Configured)
		state.TotalNodes = types.Int64Value(part.Nodes.Total)
	}
	if part.CPUs != nil {
		state.TotalCPUs = types.Int64Value(part.CPUs.Total)
	}
	if part.Priority != nil {
		state.PriorityTier = types.Int64Value(part.Priority.Tier)
		state.PriorityJobFactor = types.Int64Value(part.Priority.JobFactor)
	}
	if part.Maximums != nil {
		state.MaxTime = slurmIntOrNull(part.Maximums.Time)
		state.MaxNodesPerJob = slurmIntOrNull(part.Maximums.Nodes)
	}
	if part.Defaults != nil {
		state.DefaultTime = slurmIntOrNull(part.Defaults.Time)
	}

	setFrom := func(vals []string) types.Set {
		if len(vals) == 0 {
			return types.SetNull(types.StringType)
		}
		v, d := types.SetValueFrom(ctx, types.StringType, vals)
		resp.Diagnostics.Append(d...)
		return v
	}
	if part.Partition != nil {
		state.State = setFrom(part.Partition.State)
	}
	state.Flags = setFrom(part.Flags)
	state.PreemptMode = setFrom(part.PreemptMode)

	if part.Accounts != nil {
		state.AllowAccounts = commaSetOrNull(ctx, part.Accounts.Allowed, &resp.Diagnostics)
		state.DenyAccounts = commaSetOrNull(ctx, part.Accounts.Deny, &resp.Diagnostics)
	}
	if part.Groups != nil {
		state.AllowGroups = commaSetOrNull(ctx, part.Groups.Allowed, &resp.Diagnostics)
	}
	if part.QOS != nil {
		state.AllowQOS = commaSetOrNull(ctx, part.QOS.Allowed, &resp.Diagnostics)
		state.DenyQOS = commaSetOrNull(ctx, part.QOS.Deny, &resp.Diagnostics)
		if part.QOS.Assigned != "" {
			state.QOS = types.StringValue(part.QOS.Assigned)
		}
	}
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}
