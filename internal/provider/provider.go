package provider

import (
	"context"
	"os"

	"github.com/ai-workflows/terraform-provider-panes/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ provider.Provider = &PanesProvider{}

// ProviderClients bundles the clients for each downstream service so
// resources can pick the one they need during Configure().
type ProviderClients struct {
	Panes *client.Client
	Fleet *client.FleetClient
}

type PanesProvider struct {
	version string
}

type PanesProviderModel struct {
	APIURL       types.String `tfsdk:"api_url"`
	Token        types.String `tfsdk:"token"`
	OrgID        types.String `tfsdk:"org_id"`
	FleetAPIURL  types.String `tfsdk:"fleet_api_url"`
	FleetToken   types.String `tfsdk:"fleet_token"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &PanesProvider{version: version}
	}
}

func (p *PanesProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "panes"
	resp.Version = p.version
}

func (p *PanesProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Terraform provider for Panes (AI agent infrastructure) and Fleet (engagement lifecycle).",
		Attributes: map[string]schema.Attribute{
			"api_url": schema.StringAttribute{
				Description: "Panes API URL. Can also be set with the PANES_API_URL environment variable.",
				Optional:    true,
			},
			"token": schema.StringAttribute{
				Description: "Panes personal access token. Can also be set with the PANES_TOKEN environment variable.",
				Optional:    true,
				Sensitive:   true,
			},
			"org_id": schema.StringAttribute{
				Description: "Organization ID to scope all operations to. Can also be set with the PANES_ORG_ID environment variable.",
				Optional:    true,
			},
			"fleet_api_url": schema.StringAttribute{
				Description: "Fleet API URL (for panes_engagement resources). Can also be set with the FLEET_API_URL or FLEET_URL environment variable.",
				Optional:    true,
			},
			"fleet_token": schema.StringAttribute{
				Description: "Fleet portal JWT (for panes_engagement resources). Can also be set with the FLEET_TOKEN environment variable.",
				Optional:    true,
				Sensitive:   true,
			},
		},
	}
}

func (p *PanesProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config PanesProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// ---------- Panes client (required if panes_* resources are used) ----------
	apiURL := os.Getenv("PANES_API_URL")
	if !config.APIURL.IsNull() {
		apiURL = config.APIURL.ValueString()
	}
	if apiURL == "" {
		apiURL = "https://panes.infra.aiworkflows.com"
	}

	token := os.Getenv("PANES_TOKEN")
	if !config.Token.IsNull() {
		token = config.Token.ValueString()
	}

	orgID := os.Getenv("PANES_ORG_ID")
	if !config.OrgID.IsNull() {
		orgID = config.OrgID.ValueString()
	}

	var panesClient *client.Client
	if token != "" {
		panesClient = client.New(apiURL, token, orgID)
	}

	// ---------- Fleet client (required if panes_engagement is used) ----------
	fleetURL := os.Getenv("FLEET_API_URL")
	if fleetURL == "" {
		fleetURL = os.Getenv("FLEET_URL")
	}
	if !config.FleetAPIURL.IsNull() {
		fleetURL = config.FleetAPIURL.ValueString()
	}
	if fleetURL == "" {
		fleetURL = "https://api.fleet.build"
	}

	fleetToken := os.Getenv("FLEET_TOKEN")
	if !config.FleetToken.IsNull() {
		fleetToken = config.FleetToken.ValueString()
	}

	var fleetClient *client.FleetClient
	if fleetToken != "" {
		fleetClient = client.NewFleet(fleetURL, fleetToken, orgID)
	}

	if panesClient == nil && fleetClient == nil {
		resp.Diagnostics.AddError(
			"No provider tokens configured",
			"At least one of PANES_TOKEN (for panes_* resources) or FLEET_TOKEN (for panes_engagement) must be provided.",
		)
		return
	}

	clients := &ProviderClients{Panes: panesClient, Fleet: fleetClient}
	resp.DataSourceData = clients
	resp.ResourceData = clients
}

func (p *PanesProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewAgentResource,
		NewAgentInstanceResource,
		NewSandboxResource,
		NewSubscriptionResource,
		NewAISAccountResource,
		NewAISAccountLinkResource,
		NewEngagementResource,
	}
}

func (p *PanesProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewSubscriptionDataSource,
		NewEngagementDataSource,
	}
}
