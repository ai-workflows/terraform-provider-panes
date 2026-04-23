package provider

import (
	"context"
	"fmt"

	"github.com/ai-workflows/terraform-provider-panes/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &AISAccountResource{}
	_ resource.ResourceWithImportState = &AISAccountResource{}
)

type AISAccountResource struct {
	client *client.Client
}

type AISAccountResourceModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Slug         types.String `tfsdk:"slug"`
	ProviderType types.String `tfsdk:"provider_type"`
	Description  types.String `tfsdk:"description"`
	Status       types.String `tfsdk:"status"`
}

func NewAISAccountResource() resource.Resource {
	return &AISAccountResource{}
}

func (r *AISAccountResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ais_account"
}

// providerTypeValidator validates that provider_type is one of the allowed values.
type providerTypeValidator struct{}

func (v providerTypeValidator) Description(_ context.Context) string {
	return "value must be one of: github, gcp, aws, slack, generic"
}

func (v providerTypeValidator) MarkdownDescription(_ context.Context) string {
	return "value must be one of: `github`, `gcp`, `aws`, `slack`, `generic`"
}

func (v providerTypeValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	val := req.ConfigValue.ValueString()
	allowed := map[string]bool{
		"github":  true,
		"gcp":     true,
		"aws":     true,
		"slack":   true,
		"generic": true,
	}
	if !allowed[val] {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid provider_type",
			fmt.Sprintf("provider_type must be one of: github, gcp, aws, slack, generic. Got: %s", val),
		)
	}
}

func (r *AISAccountResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an AIS account (external service account for agent identity).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "AIS account ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Human-readable account name.",
				Required:    true,
			},
			"slug": schema.StringAttribute{
				Description: "URL-safe slug identifier for the account.",
				Required:    true,
			},
			"provider_type": schema.StringAttribute{
				Description: "Account provider type (github, gcp, aws, slack, generic).",
				Required:    true,
				Validators: []validator.String{
					providerTypeValidator{},
				},
			},
			"description": schema.StringAttribute{
				Description: "Optional description of the account.",
				Optional:    true,
			},
			"status": schema.StringAttribute{
				Description: "Account status.",
				Computed:    true,
			},
		},
	}
}

func (r *AISAccountResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderClients)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", fmt.Sprintf("Expected *ProviderClients, got: %T", req.ProviderData))
		return
	}
	if pd.Panes == nil {
		resp.Diagnostics.AddError("Panes client not configured", "Set PANES_TOKEN (or token = ...) to manage panes_ais_account resources.")
		return
	}
	r.client = pd.Panes
}

func (r *AISAccountResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan AISAccountResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createReq := client.CreateAISAccountRequest{
		Name:         plan.Name.ValueString(),
		Slug:         plan.Slug.ValueString(),
		ProviderType: plan.ProviderType.ValueString(),
		Description:  plan.Description.ValueString(),
	}

	account, err := r.client.CreateAISAccount(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError("Error creating AIS account", err.Error())
		return
	}

	r.mapToState(account, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AISAccountResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state AISAccountResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	account, err := r.client.GetAISAccount(ctx, state.ID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.IsNotFound() {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading AIS account", err.Error())
		return
	}

	r.mapToState(account, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *AISAccountResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan AISAccountResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state AISAccountResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updateReq := client.UpdateAISAccountRequest{
		Name:         plan.Name.ValueString(),
		Slug:         plan.Slug.ValueString(),
		ProviderType: plan.ProviderType.ValueString(),
		Description:  plan.Description.ValueString(),
	}

	account, err := r.client.UpdateAISAccount(ctx, state.ID.ValueString(), updateReq)
	if err != nil {
		resp.Diagnostics.AddError("Error updating AIS account", err.Error())
		return
	}

	r.mapToState(account, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AISAccountResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state AISAccountResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteAISAccount(ctx, state.ID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.IsNotFound() {
			return
		}
		resp.Diagnostics.AddError("Error deleting AIS account", err.Error())
	}
}

func (r *AISAccountResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *AISAccountResource) mapToState(account *client.AISAccount, state *AISAccountResourceModel) {
	state.ID = types.StringValue(account.ID)
	state.Name = types.StringValue(account.Name)
	state.Slug = types.StringValue(account.Slug)
	state.ProviderType = types.StringValue(account.ProviderType)

	if account.Description != "" {
		state.Description = types.StringValue(account.Description)
	} else if state.Description.IsNull() || state.Description.IsUnknown() {
		state.Description = types.StringNull()
	}

	state.Status = types.StringValue(account.Status)
}
