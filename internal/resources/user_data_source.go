package resources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dsschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/pescobar/terraform-provider-slurm/internal/client"
)

var (
	_ datasource.DataSource              = &userDataSource{}
	_ datasource.DataSourceWithConfigure = &userDataSource{}
)

type userDataSource struct {
	client *client.Client
}

func NewUserDataSource() datasource.DataSource {
	return &userDataSource{}
}

func (d *userDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (d *userDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dsschema.Schema{
		MarkdownDescription: "Reads an existing Slurm user by name, including every association and its limits.",
		Attributes: map[string]dsschema.Attribute{
			"id":              dsschema.StringAttribute{MarkdownDescription: "The user name (same as name).", Computed: true},
			"name":            dsschema.StringAttribute{MarkdownDescription: "The Slurm user name to look up.", Required: true},
			"admin_level":     dsschema.StringAttribute{MarkdownDescription: "Admin level: None, Operator, or Administrator.", Computed: true},
			"default_account": dsschema.StringAttribute{MarkdownDescription: "The user's default Slurm account, derived from the association marked is_default.", Computed: true},
			"default_wc_key":  dsschema.StringAttribute{MarkdownDescription: "Default workload characterization key.", Computed: true},

			// SetNestedAttribute (not Block) so the framework can represent
			// "unknown set" before the data source has been read. With
			// SetNestedBlock, OpenTofu/Terraform fold the unknown nested
			// collection to an empty set at plan time, breaking any output
			// that indexes or filters into it (e.g. `[for a in
			// data.slurm_user.x.association : a.fairshare if ...][0]`).
			"association": dsschema.SetNestedAttribute{
				MarkdownDescription: "All associations for this user, one entry per (account, partition) pair. Every field is read-only.",
				Computed:            true,
				NestedObject: dsschema.NestedAttributeObject{
					Attributes: map[string]dsschema.Attribute{
						"account":         dsschema.StringAttribute{MarkdownDescription: "Slurm account name.", Computed: true},
						"partition":       dsschema.StringAttribute{MarkdownDescription: "Partition (empty for unscoped associations).", Computed: true},
						"fairshare":       dsschema.Int64Attribute{MarkdownDescription: "Fairshare value.", Computed: true},
						"priority":        dsschema.Int64Attribute{MarkdownDescription: "Association-level priority.", Computed: true},
						"default_qos":     dsschema.StringAttribute{MarkdownDescription: "Default QOS.", Computed: true},
						"qos":             dsschema.ListAttribute{MarkdownDescription: "Allowed QOS list.", Computed: true, ElementType: types.StringType},
						"max_jobs":        dsschema.Int64Attribute{MarkdownDescription: "Max running jobs (MaxJobs).", Computed: true},
						"max_jobs_accrue": dsschema.Int64Attribute{MarkdownDescription: "Max pending jobs accruing priority (MaxJobsAccrue).", Computed: true},
						"max_submit_jobs": dsschema.Int64Attribute{MarkdownDescription: "Max submitted jobs (MaxSubmitJobs).", Computed: true},
						"max_wall_pj":     dsschema.Int64Attribute{MarkdownDescription: "Max wall-clock minutes per job (MaxWallDurationPerJob).", Computed: true},

						"max_tres_per_job":      tresDataSourceAttr("Max TRES per job (MaxTRES)."),
						"max_tres_per_node":     tresDataSourceAttr("Max TRES per node per job (MaxTRESPerNode)."),
						"max_tres_mins_per_job": tresDataSourceAttr("Max TRES-minutes per job (MaxTRESMins)."),

						"grp_jobs":        dsschema.Int64Attribute{MarkdownDescription: "Group running jobs limit (GrpJobs).", Computed: true},
						"grp_jobs_accrue": dsschema.Int64Attribute{MarkdownDescription: "Group jobs-accrue limit (GrpJobsAccrue).", Computed: true},
						"grp_submit_jobs": dsschema.Int64Attribute{MarkdownDescription: "Group submit-jobs limit (GrpSubmitJobs).", Computed: true},
						"grp_wall":        dsschema.Int64Attribute{MarkdownDescription: "Group wall-clock running-minutes limit (GrpWall).", Computed: true},

						"grp_tres":          tresDataSourceAttr("Group TRES (GrpTRES)."),
						"grp_tres_mins":     tresDataSourceAttr("Group TRES-minutes (GrpTRESMins)."),
						"grp_tres_run_mins": tresDataSourceAttr("Group TRES run-minutes (GrpTRESRunMins)."),
					},
				},
			},
		},
	}
}

func (d *userDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T.", req.ProviderData),
		)
		return
	}
	d.client = c
}

func (d *userDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg userResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	name := cfg.Name.ValueString()

	user, err := d.client.GetUser(name)
	if err != nil {
		resp.Diagnostics.AddError("Error reading user", err.Error())
		return
	}
	if user == nil {
		resp.Diagnostics.AddError(
			"User not found",
			fmt.Sprintf("No Slurm user named %q exists.", name),
		)
		return
	}

	state := userAPIToState(ctx, d.client, user, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// userAPIToState assembles the data-source view of a user. Unlike the
// resource Read path, no prior state exists, so every association is
// populated unconditionally — the data source returns ground truth.
func userAPIToState(ctx context.Context, c *client.Client, user *client.User, diags *diag.Diagnostics) userResourceModel {
	state := userResourceModel{
		ID:             types.StringValue(user.Name),
		Name:           types.StringValue(user.Name),
		DefaultAccount: types.StringNull(),
		DefaultWCKey:   types.StringNull(),
	}
	if len(user.AdminLevel) > 0 {
		state.AdminLevel = types.StringValue(user.AdminLevel[0])
	} else {
		state.AdminLevel = types.StringValue("None")
	}
	if user.Default != nil && user.Default.WCKey != "" {
		state.DefaultWCKey = types.StringValue(user.Default.WCKey)
	}

	assocResp, err := c.GetAssociations(map[string]string{
		"user":    user.Name,
		"cluster": c.Cluster,
	})
	if err != nil {
		diags.AddError("Error reading user associations", err.Error())
		return state
	}

	objs := make([]attr.Value, 0, len(assocResp.Associations))
	for _, a := range assocResp.Associations {
		if a.User == "" {
			continue
		}
		if a.IsDefault {
			state.DefaultAccount = types.StringValue(a.Account)
		}

		fairshare := types.Int64Null()
		if a.SharesRaw != nil {
			fairshare = types.Int64Value(int64(*a.SharesRaw))
		}
		priority := types.Int64Null()
		if a.Priority != nil && a.Priority.Set {
			priority = types.Int64Value(int64(a.Priority.Number))
		}

		defaultQOS := types.StringNull()
		if a.Default != nil && a.Default.QOS != "" {
			defaultQOS = types.StringValue(a.Default.QOS)
		}
		qos := types.ListNull(types.StringType)
		if len(a.QOS) > 0 {
			v, d := types.ListValueFrom(ctx, types.StringType, a.QOS)
			diags.Append(d...)
			qos = v
		}

		jobs := dsAssocJobsScalars(a)
		tres := snapshotAssocMaxTRES(ctx, a.Max, diags)

		attrs := map[string]attr.Value{
			"account":               types.StringValue(a.Account),
			"partition":             types.StringValue(a.Partition),
			"fairshare":             fairshare,
			"priority":              priority,
			"default_qos":           defaultQOS,
			"qos":                   qos,
			"max_jobs":              jobs.maxJobs,
			"max_jobs_accrue":       jobs.maxJobsAccrue,
			"max_submit_jobs":       jobs.maxSubmitJobs,
			"max_wall_pj":           jobs.maxWallPJ,
			"max_tres_per_job":      tres.MaxPerJob,
			"max_tres_per_node":     tres.MaxPerNode,
			"max_tres_mins_per_job": tres.MaxMinsPerJob,
			"grp_jobs":              jobs.grpJobs,
			"grp_jobs_accrue":       jobs.grpJobsAccrue,
			"grp_submit_jobs":       jobs.grpSubmitJobs,
			"grp_wall":              jobs.grpWall,
			"grp_tres":              tres.GrpTotal,
			"grp_tres_mins":         tres.GrpMins,
			"grp_tres_run_mins":     tres.GrpRunMins,
		}
		obj, d := types.ObjectValue(associationModelType(), attrs)
		diags.Append(d...)
		objs = append(objs, obj)
	}

	assocSet, d := types.SetValue(types.ObjectType{AttrTypes: associationModelType()}, objs)
	diags.Append(d...)
	state.Associations = assocSet
	return state
}

// dsAssocJobsScalars extracts every association scalar limit into typed
// values, returning Null for anything the API did not set. Centralising the
// nil-walking keeps userAPIToState readable.
type dsAssocJobs struct {
	maxJobs, maxJobsAccrue, maxSubmitJobs, maxWallPJ types.Int64
	grpJobs, grpJobsAccrue, grpSubmitJobs, grpWall   types.Int64
}

func dsAssocJobsScalars(a client.Association) dsAssocJobs {
	out := dsAssocJobs{
		maxJobs:       types.Int64Null(),
		maxJobsAccrue: types.Int64Null(),
		maxSubmitJobs: types.Int64Null(),
		maxWallPJ:     types.Int64Null(),
		grpJobs:       types.Int64Null(),
		grpJobsAccrue: types.Int64Null(),
		grpSubmitJobs: types.Int64Null(),
		grpWall:       types.Int64Null(),
	}
	// API JSON paths (see comments on AssociationMax in client.go):
	//   MaxJobs       = Max.Jobs.Active
	//   MaxJobsAccrue = Max.Jobs.Accruing
	//   MaxSubmitJobs = Max.Jobs.Total
	//   MaxWallPJ     = Max.Jobs.Per.WallClock
	//   GrpJobs       = Max.Jobs.Per.Count
	//   GrpJobsAccrue = Max.Jobs.Per.Accruing
	//   GrpSubmitJobs = Max.Jobs.Per.Submitted
	//   GrpWall       = Max.Per.Account.WallClock
	if a.Max == nil {
		return out
	}
	if j := a.Max.Jobs; j != nil {
		if j.Active != nil && j.Active.Set {
			out.maxJobs = types.Int64Value(int64(j.Active.Number))
		}
		if j.Accruing != nil && j.Accruing.Set {
			out.maxJobsAccrue = types.Int64Value(int64(j.Accruing.Number))
		}
		if j.Total != nil && j.Total.Set {
			out.maxSubmitJobs = types.Int64Value(int64(j.Total.Number))
		}
		if jp := j.Per; jp != nil {
			if jp.WallClock != nil && jp.WallClock.Set {
				out.maxWallPJ = types.Int64Value(int64(jp.WallClock.Number))
			}
			if jp.Count != nil && jp.Count.Set {
				out.grpJobs = types.Int64Value(int64(jp.Count.Number))
			}
			if jp.Accruing != nil && jp.Accruing.Set {
				out.grpJobsAccrue = types.Int64Value(int64(jp.Accruing.Number))
			}
			if jp.Submitted != nil && jp.Submitted.Set {
				out.grpSubmitJobs = types.Int64Value(int64(jp.Submitted.Number))
			}
		}
	}
	if p := a.Max.Per; p != nil && p.Account != nil && p.Account.WallClock != nil && p.Account.WallClock.Set {
		out.grpWall = types.Int64Value(int64(p.Account.WallClock.Number))
	}
	return out
}
