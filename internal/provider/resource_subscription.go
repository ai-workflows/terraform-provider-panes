package provider

import (
	"context"
	"fmt"

	"github.com/ai-workflows/terraform-provider-panes/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &SubscriptionResource{}
	_ resource.ResourceWithImportState = &SubscriptionResource{}
)

type SubscriptionResource struct {
	client *client.Client
}

type SubscriptionResourceModel struct {
	ID       types.String `tfsdk:"id"`
	Label    types.String `tfsdk:"label"`
	Provider types.String `tfsdk:"service_provider"`
	Tier     types.String `tfsdk:"tier"`
	Status   types.String `tfsdk:"status"`
}

func NewSubscriptionResource() resource.Resource {
	return &SubscriptionResource{}
}

func (r *SubscriptionResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_subscription"
}

func (r *SubscriptionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Panes LLM subscription (e.g. ChatGPT account for agent inference).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Subscription ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"label": schema.StringAttribute{
				Description: "Human-readable label (e.g. 'ChatGPT Pro - team@company.com').",
				Required:    true,
			},
			"service_provider": schema.StringAttribute{
				Description: "LLM provider (chatgpt).",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("chatgpt"),
			},
			"tier": schema.StringAttribute{
				Description: "Subscription tier (pro, plus).",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("pro"),
			},
			"status": schema.StringAttribute{
				Description: "Subscription status (pending_auth, active, rate_limited, disabled).",
				Computed:    true,
			},
		},
	}
}

func (r *SubscriptionResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderClients)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", fmt.Sprintf("Expected *ProviderClients, got: %T", req.ProviderData))
		return
	}
	if pd.Panes == nil {
		resp.Diagnostics.AddError("Panes client not configured", "Set PANES_TOKEN (or token = ...) to manage panes_subscription resources.")
		return
	}
	r.client = pd.Panes
}

func (r *SubscriptionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan SubscriptionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	sub, err := r.client.CreateSubscription(ctx, client.CreateSubscriptionRequest{
		Label:    plan.Label.ValueString(),
		Provider: plan.Provider.ValueString(),
		Tier:     plan.Tier.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating subscription", err.Error())
		return
	}

	r.mapToState(sub, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SubscriptionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state SubscriptionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	sub, err := r.client.GetSubscription(ctx, state.ID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.IsNotFound() {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading subscription", err.Error())
		return
	}

	r.mapToState(sub, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *SubscriptionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan SubscriptionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state SubscriptionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	sub, err := r.client.UpdateSubscription(ctx, state.ID.ValueString(), client.UpdateSubscriptionRequest{
		Label: plan.Label.ValueString(),
		Tier:  plan.Tier.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error updating subscription", err.Error())
		return
	}

	r.mapToState(sub, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SubscriptionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state SubscriptionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteSubscription(ctx, state.ID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.IsNotFound() {
			return
		}
		resp.Diagnostics.AddError("Error deleting subscription", err.Error())
	}
}

func (r *SubscriptionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *SubscriptionResource) mapToState(sub *client.Subscription, state *SubscriptionResourceModel) {
	state.ID = types.StringValue(sub.ID)
	state.Label = types.StringValue(sub.Label)
	state.Provider = types.StringValue(sub.Provider)

	tier := sub.Tier
	if tier == "" {
		tier = sub.PlanTier // fallback for orchestrator-sourced data
	}
	if tier != "" {
		state.Tier = types.StringValue(tier)
	}
	state.Status = types.StringValue(sub.Status)
}
