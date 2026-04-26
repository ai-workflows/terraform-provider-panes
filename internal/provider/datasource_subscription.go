package provider

import (
	"context"
	"fmt"

	"github.com/ai-workflows/terraform-provider-panes/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &SubscriptionDataSource{}

// SubscriptionDataSource looks up a ChatGPT seat by ID or label.
//
// Originally this called Panes's /api/subscriptions endpoint, which was
// itself a read-only façade over proxy-router's /api/v1/accounts. The
// indirection was a holdover from the seat-pool migration; it added an
// extra hop and a Panes dependency for no reason. tfprov#23 cut it out:
// the data source now calls proxy-router directly via the
// `proxy_router_url` provider config.
//
// The data shape, attribute names, and semantics are unchanged so
// existing consumers (sre/fleet/monitoring/portal/platform-eng + live
// engagement repos) work without changes.
type SubscriptionDataSource struct {
	client *client.ProxyRouterClient
}

type SubscriptionDataSourceModel struct {
	ID       types.String `tfsdk:"id"`
	Label    types.String `tfsdk:"label"`
	Provider types.String `tfsdk:"service_provider"`
	PlanTier types.String `tfsdk:"plan_tier"`
	Status   types.String `tfsdk:"status"`
}

func NewSubscriptionDataSource() datasource.DataSource {
	return &SubscriptionDataSource{}
}

func (d *SubscriptionDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_subscription"
}

func (d *SubscriptionDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up a ChatGPT seat from proxy-router by ID or label. Seats are created and authenticated via the proxy-router CLI (`pnpm cli add-account`); this data source is read-only.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Account ID (UUID). Specify either id or label.",
				Optional:    true,
				Computed:    true,
			},
			"label": schema.StringAttribute{
				Description: "Account label (e.g. `sub-6e1b8344`). Specify either id or label.",
				Optional:    true,
				Computed:    true,
			},
			"service_provider": schema.StringAttribute{
				Description: "Service provider (e.g. chatgpt).",
				Computed:    true,
			},
			"plan_tier": schema.StringAttribute{
				Description: "Plan tier (e.g. pro, plus).",
				Computed:    true,
			},
			"status": schema.StringAttribute{
				Description: "Account status (active, pending_auth, expired, disabled).",
				Computed:    true,
			},
		},
	}
}

func (d *SubscriptionDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderClients)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Data Source Configure Type", fmt.Sprintf("Expected *ProviderClients, got: %T", req.ProviderData))
		return
	}
	if pd.ProxyRouter == nil {
		resp.Diagnostics.AddError(
			"proxy-router client not configured",
			"Set proxy_router_url on the provider block (or PROXY_ROUTER_URL env var) to use the panes_subscription data source. Earlier versions routed this through PANES_TOKEN; that path was retired in tfprov#23.",
		)
		return
	}
	d.client = pd.ProxyRouter
}

func (d *SubscriptionDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config SubscriptionDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	accounts, err := d.client.ListAccounts(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error listing proxy-router accounts", err.Error())
		return
	}

	var match *client.ProxyRouterAccount

	if !config.ID.IsNull() && config.ID.ValueString() != "" {
		for i := range accounts {
			if accounts[i].ID == config.ID.ValueString() {
				match = &accounts[i]
				break
			}
		}
	} else if !config.Label.IsNull() && config.Label.ValueString() != "" {
		for i := range accounts {
			if accounts[i].Label == config.Label.ValueString() {
				match = &accounts[i]
				break
			}
		}
	} else {
		resp.Diagnostics.AddError("Missing filter", "Specify either id or label to look up a seat.")
		return
	}

	if match == nil {
		resp.Diagnostics.AddError("Subscription not found", "No proxy-router account matched the given id or label.")
		return
	}

	config.ID = types.StringValue(match.ID)
	config.Label = types.StringValue(match.Label)
	config.Provider = types.StringValue(match.Provider)
	config.PlanTier = types.StringValue(match.Tier)
	config.Status = types.StringValue(match.Status)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
