package provider

import (
	"context"
	"fmt"

	"github.com/ai-workflows/terraform-provider-panes/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &AISAccountDataSource{}

// AISAccountDataSource looks up an AIS account by org + slug. Needed
// because org-level accounts are typically created via the Portal UI
// (not Terraform), and engagement repos want to reference them without
// owning their lifecycle. Closes the lookup gap called out in
// ai-workflows/docs#10 Phase 1 (fleet_account data source).
type AISAccountDataSource struct {
	client *client.Client
}

type AISAccountDataSourceModel struct {
	ID           types.String `tfsdk:"id"`
	OrgID        types.String `tfsdk:"org_id"`
	Slug         types.String `tfsdk:"slug"`
	Name         types.String `tfsdk:"name"`
	ProviderType types.String `tfsdk:"provider_type"`
	Status       types.String `tfsdk:"status"`
	Description  types.String `tfsdk:"description"`
}

func NewAISAccountDataSource() datasource.DataSource {
	return &AISAccountDataSource{}
}

func (d *AISAccountDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ais_account"
}

func (d *AISAccountDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up an AIS account by org_id + slug. Use this when the account was created via the Portal UI but an engagement's agents need to reference it via panes_ais_account_link.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "AIS account id (populated by the lookup).",
				Computed:    true,
			},
			"org_id": schema.StringAttribute{
				Description: "Org id that owns the account.",
				Required:    true,
			},
			"slug": schema.StringAttribute{
				Description: "Slug of the account within the org (e.g. 'github-acme').",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "Display name.",
				Computed:    true,
			},
			"provider_type": schema.StringAttribute{
				Description: "Provider type (github, slack, gcp, aws, generic, …).",
				Computed:    true,
			},
			"status": schema.StringAttribute{
				Description: "Account status (active, revoked).",
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "Account description.",
				Computed:    true,
			},
		},
	}
}

func (d *AISAccountDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	clients, ok := req.ProviderData.(*ProviderClients)
	if !ok {
		resp.Diagnostics.AddError("Unexpected DataSource Configure Type", fmt.Sprintf("Expected *ProviderClients, got: %T", req.ProviderData))
		return
	}
	d.client = clients.Panes
}

func (d *AISAccountDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var model AISAccountDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if d.client == nil {
		resp.Diagnostics.AddError("Client not configured", "Panes/AIS client was not configured on this data source. Ensure the provider is configured with PANES_API_URL + PANES_PAT.")
		return
	}

	accounts, err := d.client.ListAISAccounts(ctx, model.OrgID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to list AIS accounts", err.Error())
		return
	}

	wantSlug := model.Slug.ValueString()
	for _, a := range accounts {
		if a.Slug == wantSlug {
			model.ID = types.StringValue(a.ID)
			model.Name = types.StringValue(a.Name)
			model.ProviderType = types.StringValue(a.ProviderType)
			model.Status = types.StringValue(a.Status)
			if a.Description != "" {
				model.Description = types.StringValue(a.Description)
			} else {
				model.Description = types.StringNull()
			}
			resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
			return
		}
	}

	resp.Diagnostics.AddError(
		"AIS account not found",
		fmt.Sprintf("No AIS account with slug %q found in org %q. Check that the account exists in the Portal UI or was created via panes_ais_account.", wantSlug, model.OrgID.ValueString()),
	)
}
