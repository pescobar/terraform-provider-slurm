package resources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dsschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/pescobar/terraform-provider-slurm/internal/client"
)

var (
	_ datasource.DataSource              = &accountDataSource{}
	_ datasource.DataSourceWithConfigure = &accountDataSource{}
)

type accountDataSource struct {
	client *client.Client
}

func NewAccountDataSource() datasource.DataSource {
	return &accountDataSource{}
}

func (d *accountDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_account"
}

func (d *accountDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dsschema.Schema{
		MarkdownDescription: "Reads an existing Slurm account by name, including its account-level association limits.",
		Attributes: map[string]dsschema.Attribute{
			"id":             dsschema.StringAttribute{MarkdownDescription: "The account name (same as name).", Computed: true},
			"name":           dsschema.StringAttribute{MarkdownDescription: "The name of the account to look up.", Required: true},
			"description":    dsschema.StringAttribute{MarkdownDescription: "Account description.", Computed: true},
			"organization":   dsschema.StringAttribute{MarkdownDescription: "Organization the account belongs to.", Computed: true},
			"parent_account": dsschema.StringAttribute{MarkdownDescription: "Parent account name in the Slurm tree.", Computed: true},

			"fairshare":   dsschema.Int64Attribute{MarkdownDescription: "Fairshare value on the account-level association.", Computed: true},
			"default_qos": dsschema.StringAttribute{MarkdownDescription: "Default QOS on the account-level association.", Computed: true},
			"allowed_qos": dsschema.ListAttribute{MarkdownDescription: "Allowed QOS list on the account-level association.", Computed: true, ElementType: types.StringType},
			"max_jobs":    dsschema.Int64Attribute{MarkdownDescription: "Maximum running jobs on the account-level association (MaxJobs).", Computed: true},

			"max_tres_per_job":      tresDataSourceAttr("Max TRES per job (MaxTRES) on the account-level association."),
			"max_tres_per_node":     tresDataSourceAttr("Max TRES per node per job (MaxTRESPerNode)."),
			"max_tres_mins_per_job": tresDataSourceAttr("Max TRES-minutes per job (MaxTRESMins)."),
			"grp_tres":              tresDataSourceAttr("Group TRES (GrpTRES)."),
			"grp_tres_mins":         tresDataSourceAttr("Group TRES-minutes (GrpTRESMins)."),
			"grp_tres_run_mins":     tresDataSourceAttr("Group running TRES-minutes (GrpTRESRunMins)."),
		},
	}
}

func (d *accountDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c := configureDataSourceClient(req, resp); c != nil {
		d.client = c
	}
}

func (d *accountDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg accountResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	name := cfg.Name.ValueString()

	account, err := d.client.GetAccount(ctx, name)
	if err != nil {
		resp.Diagnostics.AddError("Error reading account", err.Error())
		return
	}
	if account == nil {
		resp.Diagnostics.AddError(
			"Account not found",
			fmt.Sprintf("No Slurm account named %q exists.", name),
		)
		return
	}

	state := accountAPIToState(ctx, d.client, account, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// accountAPIToState builds a fully-populated accountResourceModel from the
// Slurm API account record. Unlike the resource Read path, the data source
// has no prior state, so every attribute is populated unconditionally —
// callers want the truth of what's in Slurm, not a null-preserved view.
func accountAPIToState(ctx context.Context, c *client.Client, account *client.Account, diags *diag.Diagnostics) accountResourceModel {
	state := accountResourceModel{
		ID:           types.StringValue(account.Name),
		Name:         types.StringValue(account.Name),
		Description:  types.StringValue(account.Description),
		Organization: types.StringValue(account.Organization),
		Parent:       types.StringValue(account.ParentAccount),

		Fairshare:  types.Int64Null(),
		DefaultQOS: types.StringNull(),
		AllowedQOS: types.ListNull(types.StringType),
		MaxJobs:    types.Int64Null(),

		MaxTRESPerJob:     types.SetNull(tresElemType()),
		MaxTRESPerNode:    types.SetNull(tresElemType()),
		MaxTRESMinsPerJob: types.SetNull(tresElemType()),
		GrpTRES:           types.SetNull(tresElemType()),
		GrpTRESMins:       types.SetNull(tresElemType()),
		GrpTRESRunMins:    types.SetNull(tresElemType()),
	}

	assocResp, err := c.GetAssociations(ctx, map[string]string{
		"account": account.Name,
		"cluster": c.Cluster,
	})
	if err != nil {
		diags.AddWarning(
			"Could not read account associations",
			fmt.Sprintf("account=%s: %s", account.Name, err.Error()),
		)
		return state
	}

	for _, assoc := range assocResp.Associations {
		if assoc.User != "" {
			continue
		}
		if assoc.SharesRaw != nil {
			state.Fairshare = types.Int64Value(int64(*assoc.SharesRaw))
		}
		if assoc.Default != nil && assoc.Default.QOS != "" {
			state.DefaultQOS = types.StringValue(assoc.Default.QOS)
		}
		if len(assoc.QOS) > 0 {
			v, d := types.ListValueFrom(ctx, types.StringType, assoc.QOS)
			diags.Append(d...)
			state.AllowedQOS = v
		}
		if assoc.Max != nil && assoc.Max.Jobs != nil &&
			assoc.Max.Jobs.Active != nil && assoc.Max.Jobs.Active.Set {
			state.MaxJobs = types.Int64Value(int64(assoc.Max.Jobs.Active.Number))
		}
		tres := snapshotAssocMaxTRES(ctx, assoc.Max, diags)
		state.GrpTRES = tres.GrpTotal
		state.GrpTRESMins = tres.GrpMins
		state.GrpTRESRunMins = tres.GrpRunMins
		state.MaxTRESPerJob = tres.MaxPerJob
		state.MaxTRESPerNode = tres.MaxPerNode
		state.MaxTRESMinsPerJob = tres.MaxMinsPerJob
		break
	}
	return state
}
