package resources

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dsschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/pescobar/terraform-provider-slurm/internal/client"
)

// The /conf endpoints exist only in API v0.0.45+ (Slurm 26.05). A configured
// api_version that is too old is caught before any HTTP call by the client's
// version pre-check (client.VersionError). Reaching an HTTP 404 therefore
// means the api_version claims support but the server does not provide it —
// i.e. the cluster runs an older Slurm release.
const confVersionHint = "The conf endpoints require Slurm 26.05 or later. The " +
	"configured api_version supports them, but the server returned HTTP 404 — " +
	"the cluster is most likely running a Slurm release older than 26.05."

// confAPIRequirement is appended to both conf data-source descriptions.
const confAPIRequirement = " Requires Slurm 26.05+ (API version v0.0.45+)."

// stringifyConfMap converts a raw conf key → JSON-value map into a Terraform
// map(string) using client.StringifyConfValue for each value.
func stringifyConfMap(ctx context.Context, raw map[string]json.RawMessage, diags *diag.Diagnostics) types.Map {
	flat := make(map[string]string, len(raw))
	for k, v := range raw {
		flat[k] = client.StringifyConfValue(v)
	}
	m, d := types.MapValueFrom(ctx, types.StringType, flat)
	diags.Append(d...)
	return m
}

// notFoundHint returns confVersionHint when err is the endpoint-missing 404,
// otherwise the raw error text.
func notFoundHint(err error) string {
	if client.IsNotFound(err) {
		return confVersionHint
	}
	return err.Error()
}

// ---------------------------------------------------------------------------
// slurm_conf — active slurmctld configuration
// ---------------------------------------------------------------------------

var (
	_ datasource.DataSource              = &confDataSource{}
	_ datasource.DataSourceWithConfigure = &confDataSource{}
)

type confDataSource struct {
	client *client.Client
}

func NewConfDataSource() datasource.DataSource {
	return &confDataSource{}
}

func (d *confDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_conf"
}

type confDataSourceModel struct {
	ID           types.String `tfsdk:"id"`
	SlurmVersion types.String `tfsdk:"slurm_version"`
	ConfPath     types.String `tfsdk:"conf_path"`
	ClusterName  types.String `tfsdk:"cluster_name"`
	Conf         types.Map    `tfsdk:"conf"`
}

func (d *confDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dsschema.Schema{
		MarkdownDescription: "Reads the active slurmctld configuration (the live equivalent of " +
			"`scontrol show config`). Useful for preflight assertions — e.g. checking that " +
			"`AccountingStorageEnforce` includes `associations` — or for referencing cluster " +
			"facts like the Slurm version in outputs." + confAPIRequirement,
		Attributes: map[string]dsschema.Attribute{
			"id":            dsschema.StringAttribute{MarkdownDescription: "Always `slurm_conf`.", Computed: true},
			"slurm_version": dsschema.StringAttribute{MarkdownDescription: "The running Slurm version (e.g. `26.05.1`).", Computed: true},
			"conf_path":     dsschema.StringAttribute{MarkdownDescription: "Path of the slurm.conf the controller loaded.", Computed: true},
			"cluster_name":  dsschema.StringAttribute{MarkdownDescription: "ClusterName from the active configuration.", Computed: true},
			"conf": dsschema.MapAttribute{
				MarkdownDescription: "All configuration keys, using slurm.conf capitalisation " +
					"(e.g. `SchedulerType`). Values are flattened to strings: lists are " +
					"comma-joined, unset tri-state numbers are empty, unlimited values are " +
					"`infinite`.",
				Computed:    true,
				ElementType: types.StringType,
			},
		},
	}
}

func (d *confDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c := configureDataSourceClient(req, resp); c != nil {
		d.client = c
	}
}

func (d *confDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	raw, meta, err := d.client.GetSlurmConf(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading slurmctld configuration", notFoundHint(err))
		return
	}

	state := confDataSourceModel{
		ID:           types.StringValue("slurm_conf"),
		SlurmVersion: types.StringNull(),
		ConfPath:     types.StringNull(),
		ClusterName:  types.StringNull(),
		Conf:         stringifyConfMap(ctx, raw, &resp.Diagnostics),
	}
	if meta != nil {
		state.SlurmVersion = types.StringValue(meta.SlurmVersion)
		state.ConfPath = types.StringValue(meta.ConfPath)
	}
	if v, ok := raw["ClusterName"]; ok {
		state.ClusterName = types.StringValue(client.StringifyConfValue(v))
	}
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// ---------------------------------------------------------------------------
// slurm_dbd_conf — active slurmdbd configuration
// ---------------------------------------------------------------------------

var (
	_ datasource.DataSource              = &dbdConfDataSource{}
	_ datasource.DataSourceWithConfigure = &dbdConfDataSource{}
)

type dbdConfDataSource struct {
	client *client.Client
}

func NewDBDConfDataSource() datasource.DataSource {
	return &dbdConfDataSource{}
}

func (d *dbdConfDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dbd_conf"
}

type dbdConfDataSourceModel struct {
	ID   types.String `tfsdk:"id"`
	Conf types.Map    `tfsdk:"conf"`
}

func (d *dbdConfDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dsschema.Schema{
		MarkdownDescription: "Reads the active slurmdbd configuration (the live equivalent of " +
			"`sacctmgr show config`). Useful for preflight assertions on accounting settings " +
			"such as `TrackWCKey` or the purge windows." + confAPIRequirement,
		Attributes: map[string]dsschema.Attribute{
			"id": dsschema.StringAttribute{MarkdownDescription: "Always `slurm_dbd_conf`.", Computed: true},
			"conf": dsschema.MapAttribute{
				MarkdownDescription: "All configuration keys, using slurmdbd.conf capitalisation " +
					"(e.g. `TrackWCKey`). Values are flattened to strings the same way as " +
					"`slurm_conf`.",
				Computed:    true,
				ElementType: types.StringType,
			},
		},
	}
}

func (d *dbdConfDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c := configureDataSourceClient(req, resp); c != nil {
		d.client = c
	}
}

func (d *dbdConfDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	raw, err := d.client.GetSlurmdbdConf(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading slurmdbd configuration", notFoundHint(err))
		return
	}

	state := dbdConfDataSourceModel{
		ID:   types.StringValue("slurm_dbd_conf"),
		Conf: stringifyConfMap(ctx, raw, &resp.Diagnostics),
	}
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}
