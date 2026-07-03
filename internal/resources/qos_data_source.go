package resources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dsschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/pescobar/terraform-provider-slurm/internal/client"
)

var (
	_ datasource.DataSource              = &qosDataSource{}
	_ datasource.DataSourceWithConfigure = &qosDataSource{}
)

type qosDataSource struct {
	client *client.Client
}

func NewQOSDataSource() datasource.DataSource {
	return &qosDataSource{}
}

func (d *qosDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_qos"
}

// tresDataSourceAttr is the data-source counterpart to tresSchemaAttr —
// every attribute is Computed because data sources never accept input.
func tresDataSourceAttr(description string) dsschema.SetNestedAttribute {
	return dsschema.SetNestedAttribute{
		MarkdownDescription: description,
		Computed:            true,
		NestedObject: dsschema.NestedAttributeObject{
			Attributes: map[string]dsschema.Attribute{
				"type":  dsschema.StringAttribute{MarkdownDescription: "TRES type (e.g. cpu, mem, gres).", Computed: true},
				"name":  dsschema.StringAttribute{MarkdownDescription: "TRES name. Set for generic resources such as gres/gpu.", Computed: true},
				"count": dsschema.Int64Attribute{MarkdownDescription: "TRES count limit.", Computed: true},
			},
		},
	}
}

func (d *qosDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dsschema.Schema{
		MarkdownDescription: "Reads an existing Slurm Quality of Service (QOS) by name. Useful for referencing a QOS managed outside Terraform — for example one maintained by a separate team or auto-created by Slurm — without bringing it under provider management.",
		Attributes: map[string]dsschema.Attribute{
			"id":          dsschema.StringAttribute{MarkdownDescription: "The QOS name (same as name).", Computed: true},
			"name":        dsschema.StringAttribute{MarkdownDescription: "The name of the QOS to look up.", Required: true},
			"description": dsschema.StringAttribute{MarkdownDescription: "Description text stored with the QOS.", Computed: true},

			"priority":            dsschema.Int64Attribute{MarkdownDescription: "Priority value (Priority).", Computed: true},
			"max_wall_pj":         dsschema.Int64Attribute{MarkdownDescription: "Maximum wall-clock time per job in minutes (MaxWall).", Computed: true},
			"grp_wall":            dsschema.Int64Attribute{MarkdownDescription: "Maximum cumulative running wall-clock minutes across the QOS (GrpWall).", Computed: true},
			"grace_time":          dsschema.Int64Attribute{MarkdownDescription: "Grace time in seconds before a job exceeding QOS limits is cancelled (GraceTime).", Computed: true},
			"usage_factor":        dsschema.Float64Attribute{MarkdownDescription: "Factor applied to a job's usage when running under this QOS (UsageFactor).", Computed: true},
			"usage_threshold":     dsschema.Float64Attribute{MarkdownDescription: "Minimum usage factor required for job submission (UsageThres).", Computed: true},
			"preempt_exempt_time": dsschema.Int64Attribute{MarkdownDescription: "Minimum runtime before a job can be preempted (PreemptExemptTime).", Computed: true},

			"grp_jobs":                    dsschema.Int64Attribute{MarkdownDescription: "Max running jobs across the QOS (GrpJobs).", Computed: true},
			"grp_submit_jobs":             dsschema.Int64Attribute{MarkdownDescription: "Max submitted jobs across the QOS (GrpSubmit).", Computed: true},
			"max_jobs_per_user":           dsschema.Int64Attribute{MarkdownDescription: "Max running jobs per user (MaxJobsPU).", Computed: true},
			"max_submit_jobs_per_user":    dsschema.Int64Attribute{MarkdownDescription: "Max submitted jobs per user (MaxSubmitPU).", Computed: true},
			"max_jobs_per_account":        dsschema.Int64Attribute{MarkdownDescription: "Max running jobs per account (MaxJobsPA).", Computed: true},
			"max_submit_jobs_per_account": dsschema.Int64Attribute{MarkdownDescription: "Max submitted jobs per account (MaxSubmitPA).", Computed: true},

			"flags":        dsschema.SetAttribute{MarkdownDescription: "QOS flags.", Computed: true, ElementType: types.StringType},
			"preempt_list": dsschema.SetAttribute{MarkdownDescription: "QOS names this QOS can preempt (Preempt).", Computed: true, ElementType: types.StringType},
			"preempt_mode": dsschema.SetAttribute{MarkdownDescription: "Preemption mode (PreemptMode).", Computed: true, ElementType: types.StringType},

			"grp_tres":                  tresDataSourceAttr("Maximum TRES usable by all jobs in this QOS at any time (GrpTRES)."),
			"grp_tres_mins":             tresDataSourceAttr("Maximum TRES-minutes for jobs in this QOS (GrpTRESMins)."),
			"max_tres_per_job":          tresDataSourceAttr("Maximum TRES a single job can request (MaxTRES)."),
			"max_tres_mins_per_job":     tresDataSourceAttr("Maximum TRES-minutes a single job can consume (MaxTRESMins)."),
			"max_tres_per_node":         tresDataSourceAttr("Maximum TRES per node per job (MaxTRESPerNode)."),
			"max_tres_per_user":         tresDataSourceAttr("Maximum TRES per user (MaxTRESPU)."),
			"max_tres_mins_per_user":    tresDataSourceAttr("Maximum TRES run-minutes per user (MaxTRESRunMinsPU)."),
			"max_tres_per_account":      tresDataSourceAttr("Maximum TRES per account (MaxTRESPA)."),
			"max_tres_mins_per_account": tresDataSourceAttr("Maximum TRES run-minutes per account (MaxTRESRunMinsPA)."),
			"min_tres_per_job":          tresDataSourceAttr("Minimum TRES required to run a job (MinTRES)."),
		},
	}
}

func (d *qosDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c := configureDataSourceClient(req, resp); c != nil {
		d.client = c
	}
}

func (d *qosDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg qosResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	qos, err := d.client.GetQOS(cfg.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading QOS",
			fmt.Sprintf("Could not read QOS %q: %s", cfg.Name.ValueString(), err.Error()),
		)
		return
	}
	if qos == nil {
		resp.Diagnostics.AddError(
			"QOS not found",
			fmt.Sprintf("No Slurm QOS named %q exists.", cfg.Name.ValueString()),
		)
		return
	}

	state := qosAPIToState(ctx, qos, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}
