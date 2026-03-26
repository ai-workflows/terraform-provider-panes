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

type PanesProvider struct {
	version string
}

type PanesProviderModel struct {
	APIURL types.String `tfsdk:"api_url"`
	Token  types.String `tfsdk:"token"`
	OrgID  types.String `tfsdk:"org_id"`
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
		Description: "Terraform provider for Panes — declarative AI agent infrastructure.",
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
		},
	}
}

func (p *PanesProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config PanesProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

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
	if token == "" {
		resp.Diagnostics.AddError(
			"Missing API Token",
			"A Panes personal access token must be provided via the 'token' attribute or the PANES_TOKEN environment variable.",
		)
		return
	}

	orgID := os.Getenv("PANES_ORG_ID")
	if !config.OrgID.IsNull() {
		orgID = config.OrgID.ValueString()
	}

	c := client.New(apiURL, token, orgID)
	resp.DataSourceData = c
	resp.ResourceData = c
}

func (p *PanesProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewAgentResource,
		NewAgentInstanceResource,
		NewSandboxResource,
	}
}

func (p *PanesProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewSubscriptionDataSource,
	}
}
