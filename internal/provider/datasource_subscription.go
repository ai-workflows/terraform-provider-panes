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

type SubscriptionDataSource struct {
	client *client.Client
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
		Description: "Look up a ChatGPT subscription by ID or label. Subscriptions are created and authenticated via the Panes UI.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Subscription ID. Specify either id or label.",
				Optional:    true,
				Computed:    true,
			},
			"label": schema.StringAttribute{
				Description: "Subscription label. Specify either id or label.",
				Optional:    true,
				Computed:    true,
			},
			"service_provider": schema.StringAttribute{
				Description: "Service provider (e.g. chatgpt).",
				Computed:    true,
			},
			"plan_tier": schema.StringAttribute{
				Description: "Subscription plan tier (e.g. pro, plus).",
				Computed:    true,
			},
			"status": schema.StringAttribute{
				Description: "Subscription status (active, pending_auth, rate_limited, disabled).",
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
	if pd.Panes == nil {
		resp.Diagnostics.AddError("Panes client not configured", "Set PANES_TOKEN (or token = ...) to use panes_subscription data source.")
		return
	}
	d.client = pd.Panes
}

func (d *SubscriptionDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config SubscriptionDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	subs, err := d.client.ListSubscriptions(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error listing subscriptions", err.Error())
		return
	}

	var match *client.Subscription

	if !config.ID.IsNull() && config.ID.ValueString() != "" {
		for i := range subs {
			if subs[i].ID == config.ID.ValueString() {
				match = &subs[i]
				break
			}
		}
	} else if !config.Label.IsNull() && config.Label.ValueString() != "" {
		for i := range subs {
			if subs[i].Label == config.Label.ValueString() {
				match = &subs[i]
				break
			}
		}
	} else {
		resp.Diagnostics.AddError("Missing filter", "Specify either id or label to look up a subscription.")
		return
	}

	if match == nil {
		resp.Diagnostics.AddError("Subscription not found", "No subscription matched the given id or label.")
		return
	}

	config.ID = types.StringValue(match.ID)
	config.Label = types.StringValue(match.Label)
	config.Provider = types.StringValue(match.Provider)
	config.PlanTier = types.StringValue(match.PlanTier)
	config.Status = types.StringValue(match.Status)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
