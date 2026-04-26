// Package provider defines the Slurm Terraform provider.
//
// The provider accepts configuration for connecting to a slurmrestd instance
// and exposes resources for managing Slurm accounting entities (clusters,
// accounts, users, QOS).
package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/pescobar/terraform-provider-slurm/internal/client"
	"github.com/pescobar/terraform-provider-slurm/internal/resources"
)

// Ensure the implementation satisfies the expected interfaces.
var _ provider.Provider = &slurmProvider{}

// slurmProvider is the provider implementation.
type slurmProvider struct {
	version string
}

// slurmProviderModel maps the provider schema to a Go struct.
type slurmProviderModel struct {
	Endpoint   types.String `tfsdk:"endpoint"`
	Token      types.String `tfsdk:"token"`
	Cluster    types.String `tfsdk:"cluster"`
	APIVersion types.String `tfsdk:"api_version"`
}

// New returns a function that creates a new provider instance.
// This is the entry point used by the provider server in main.go.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &slurmProvider{
			version: version,
		}
	}
}

// Metadata returns the provider type name.
func (p *slurmProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "slurm"
	resp.Version = p.version
}

// Schema defines the provider configuration attributes.
func (p *slurmProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manage Slurm accounting resources (accounts, users, QOS) via the slurmrestd REST API.",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				Description: "The URL of the slurmrestd REST API (e.g. http://localhost:6820). " +
					"Can also be set with the SLURM_REST_URL environment variable.",
				Optional: true,
			},
			"token": schema.StringAttribute{
				Description: "JWT token for authenticating to slurmrestd. " +
					"Can also be set with the SLURM_JWT_TOKEN environment variable.",
				Optional:  true,
				Sensitive: true,
			},
			"cluster": schema.StringAttribute{
				Description: "The Slurm cluster name. Used to scope association operations. " +
					"Can also be set with the SLURM_CLUSTER environment variable.",
				Optional: true,
			},
			"api_version": schema.StringAttribute{
				Description: "The slurmrestd API version (e.g. v0.0.42). " +
					"Can also be set with the SLURM_API_VERSION environment variable. " +
					"Defaults to v0.0.42 (Slurm 25.05.x).",
				Optional: true,
			},
		},
	}
}

// Configure prepares the API client for resource operations.
// Values from the provider block take precedence over environment variables.
func (p *slurmProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	tflog.Info(ctx, "Configuring Slurm provider")

	var config slurmProviderModel
	diags := req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Resolve each config value: HCL config takes precedence over env var
	endpoint := resolveConfigValue(config.Endpoint, "SLURM_REST_URL", "")
	token := resolveConfigValue(config.Token, "SLURM_JWT_TOKEN", "")
	cluster := resolveConfigValue(config.Cluster, "SLURM_CLUSTER", "")
	apiVersion := resolveConfigValue(config.APIVersion, "SLURM_API_VERSION", "v0.0.42")

	// Validate required fields
	if endpoint == "" {
		resp.Diagnostics.AddError(
			"Missing Slurm REST API endpoint",
			"The provider requires a slurmrestd endpoint. Set it in the provider block "+
				"or via the SLURM_REST_URL environment variable.",
		)
	}
	if token == "" {
		resp.Diagnostics.AddError(
			"Missing Slurm JWT token",
			"The provider requires a JWT token for authentication. Set it in the provider "+
				"block or via the SLURM_JWT_TOKEN environment variable.",
		)
	}
	if cluster == "" {
		resp.Diagnostics.AddError(
			"Missing Slurm cluster name",
			"The provider requires a cluster name. Set it in the provider block "+
				"or via the SLURM_CLUSTER environment variable.",
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating Slurm API client", map[string]interface{}{
		"endpoint":    endpoint,
		"cluster":     cluster,
		"api_version": apiVersion,
	})

	// Create the API client and verify connectivity
	c := client.NewClient(endpoint, token, cluster, apiVersion)

	if err := c.Ping(); err != nil {
		resp.Diagnostics.AddError(
			"Unable to connect to Slurm REST API",
			"The provider failed to connect to slurmrestd at "+endpoint+": "+err.Error(),
		)
		return
	}

	if err := c.EnsureCluster(); err != nil {
		resp.Diagnostics.AddError(
			"Unable to register Slurm cluster",
			"The provider failed to register cluster '"+cluster+"' in slurmdbd: "+err.Error(),
		)
		return
	}

	tflog.Info(ctx, "Slurm provider configured successfully")

	// Make the client available to resources and data sources.
	// The framework passes this via ResourceData/DataSourceData.
	resp.ResourceData = c
	resp.DataSourceData = c
}

// Resources defines the resources implemented by this provider.
func (p *slurmProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		resources.NewAccountResource,
		resources.NewQOSResource,
		resources.NewUserResource,
	}
}

// DataSources defines the data sources implemented by this provider.
// None yet — we can add read-only data sources later.
func (p *slurmProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return nil
}

// resolveConfigValue returns the HCL-configured value if set, otherwise falls
// back to the named environment variable, and finally to the default value.
func resolveConfigValue(configured types.String, envVar, defaultValue string) string {
	if !configured.IsNull() && !configured.IsUnknown() {
		return configured.ValueString()
	}
	if v := os.Getenv(envVar); v != "" {
		return v
	}
	return defaultValue
}
