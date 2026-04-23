package provider

import (
	"context"
	"fmt"

	"github.com/ai-workflows/terraform-provider-panes/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &EngagementDataSource{}

// EngagementDataSource looks up a Fleet engagement by ID or name, so
// engagement repos whose engagements are created out-of-band (e.g. via the
// Portal UI) can reference them in Terraform without TF owning the lifecycle.
type EngagementDataSource struct {
	fleet *client.FleetClient
}

type EngagementDataSourceModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	Status           types.String `tfsdk:"status"`
	Mode             types.String `tfsdk:"mode"`
	SlackChannelID   types.String `tfsdk:"slack_channel_id"`
	SlackChannelName types.String `tfsdk:"slack_channel_name"`
	CommsAgentID     types.String `tfsdk:"comms_agent_id"`
	WorkerAgentIDs   types.List   `tfsdk:"worker_agent_ids"`
}

func NewEngagementDataSource() datasource.DataSource {
	return &EngagementDataSource{}
}

func (d *EngagementDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_engagement"
}

func (d *EngagementDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up a Fleet engagement by id or name. Use this when the engagement was created outside Terraform (e.g. via the Portal UI) but the engagement repo wants to reference it.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Engagement ID. Specify either id or name.",
				Optional:    true,
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "Engagement name. Specify either id or name.",
				Optional:    true,
				Computed:    true,
			},
			"status": schema.StringAttribute{
				Description: "Engagement status reported by Fleet.",
				Computed:    true,
			},
			"mode": schema.StringAttribute{
				Description: "Engagement mode (standard | chat_agent).",
				Computed:    true,
			},
			"slack_channel_id": schema.StringAttribute{
				Description: "Slack channel ID if provisioned.",
				Computed:    true,
			},
			"slack_channel_name": schema.StringAttribute{
				Description: "Slack channel name if configured.",
				Computed:    true,
			},
			"comms_agent_id": schema.StringAttribute{
				Description: "ID of the comms agent for this engagement.",
				Computed:    true,
			},
			"worker_agent_ids": schema.ListAttribute{
				Description: "IDs of the worker agents for this engagement.",
				ElementType: types.StringType,
				Computed:    true,
			},
		},
	}
}

func (d *EngagementDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderClients)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Data Source Configure Type", fmt.Sprintf("Expected *ProviderClients, got: %T", req.ProviderData))
		return
	}
	if pd.Fleet == nil {
		resp.Diagnostics.AddError("Fleet client not configured", "Set FLEET_TOKEN (or fleet_token = ...) to use panes_engagement data source.")
		return
	}
	d.fleet = pd.Fleet
}

func (d *EngagementDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config EngagementDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var match *client.Engagement

	switch {
	case !config.ID.IsNull() && config.ID.ValueString() != "":
		eng, err := d.fleet.GetEngagement(ctx, config.ID.ValueString())
		if err != nil {
			if apiErr, ok := err.(*client.APIError); ok && apiErr.IsNotFound() {
				resp.Diagnostics.AddError("Engagement not found", fmt.Sprintf("No engagement with id %q.", config.ID.ValueString()))
				return
			}
			resp.Diagnostics.AddError("Failed to fetch engagement", err.Error())
			return
		}
		match = eng

	case !config.Name.IsNull() && config.Name.ValueString() != "":
		engs, err := d.fleet.ListEngagements(ctx)
		if err != nil {
			resp.Diagnostics.AddError("Failed to list engagements", err.Error())
			return
		}
		for i := range engs {
			if engs[i].Name == config.Name.ValueString() {
				match = &engs[i]
				break
			}
		}
		if match == nil {
			resp.Diagnostics.AddError("Engagement not found", fmt.Sprintf("No engagement named %q.", config.Name.ValueString()))
			return
		}

	default:
		resp.Diagnostics.AddError("Missing filter", "Specify either id or name to look up an engagement.")
		return
	}

	config.ID = types.StringValue(match.ID)
	config.Name = types.StringValue(match.Name)
	config.Status = types.StringValue(match.Status)
	config.Mode = types.StringValue(resolveMode(match))
	if match.SlackChannelID != "" {
		config.SlackChannelID = types.StringValue(match.SlackChannelID)
	} else {
		config.SlackChannelID = types.StringNull()
	}
	if match.SlackChannelName != "" {
		config.SlackChannelName = types.StringValue(match.SlackChannelName)
	} else {
		config.SlackChannelName = types.StringNull()
	}
	if match.CommsAgentID != "" {
		config.CommsAgentID = types.StringValue(match.CommsAgentID)
	} else {
		config.CommsAgentID = types.StringNull()
	}
	workers := make([]attr.Value, len(match.WorkerAgentIDs))
	for i, id := range match.WorkerAgentIDs {
		workers[i] = types.StringValue(id)
	}
	workerList, _ := types.ListValue(types.StringType, workers)
	config.WorkerAgentIDs = workerList

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
