package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/ai-workflows/terraform-provider-panes/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &SandboxResource{}
	_ resource.ResourceWithImportState = &SandboxResource{}
)

type SandboxResource struct {
	client *client.Client
}

type SandboxResourceModel struct {
	ID           types.String `tfsdk:"id"`
	Status       types.String `tfsdk:"status"`
	Image        types.String `tfsdk:"image"`
	ComputeClass types.String `tfsdk:"compute_class"`
	Cloud        types.String `tfsdk:"cloud"`
	InstanceType types.String `tfsdk:"instance_type"`
	NestedVirt   types.Bool   `tfsdk:"nested_virt"`
	DiskSize     types.Int64  `tfsdk:"disk_size"`
	Zone         types.String `tfsdk:"zone"`
	Project      types.String `tfsdk:"project"`
	URL          types.String `tfsdk:"url"`
	VMUrl        types.String `tfsdk:"vm_url"`
}

func NewSandboxResource() resource.Resource {
	return &SandboxResource{}
}

func (r *SandboxResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sandbox"
}

func (r *SandboxResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Panes sandbox (cloud VM).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Sandbox ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"status": schema.StringAttribute{
				Description: "Current sandbox status (creating, running, paused, destroyed, error).",
				Computed:    true,
			},
			"image": schema.StringAttribute{
				Description: "VM image.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("ubuntu-22.04-agent"),
			},
			"compute_class": schema.StringAttribute{
				Description: "Compute class.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("standard-2"),
			},
			"cloud": schema.StringAttribute{
				Description: "Cloud provider (gcp or aws).",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("gcp"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"instance_type": schema.StringAttribute{
				Description: "Cloud instance type (e.g. n2-standard-8).",
				Optional:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"nested_virt": schema.BoolAttribute{
				Description: "Enable nested virtualization.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
				PlanModifiers: []planmodifier.Bool{
					boolRequiresReplace{},
				},
			},
			"disk_size": schema.Int64Attribute{
				Description: "Disk size in GB (10-1000).",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(50),
				PlanModifiers: []planmodifier.Int64{
					int64RequiresReplace{},
				},
			},
			"zone": schema.StringAttribute{
				Description: "Cloud zone (e.g. us-central1-a).",
				Optional:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"project": schema.StringAttribute{
				Description: "GCP project ID.",
				Optional:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"url": schema.StringAttribute{
				Description: "Public sandbox URL.",
				Computed:    true,
			},
			"vm_url": schema.StringAttribute{
				Description: "Internal VM URL (tool-executor endpoint).",
				Computed:    true,
			},
		},
	}
}

func (r *SandboxResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", fmt.Sprintf("Expected *client.Client, got: %T", req.ProviderData))
		return
	}
	r.client = c
}

func (r *SandboxResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan SandboxResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createReq := client.CreateSandboxRequest{
		Image:        plan.Image.ValueString(),
		ComputeClass: plan.ComputeClass.ValueString(),
		Cloud:        plan.Cloud.ValueString(),
		InstanceType: plan.InstanceType.ValueString(),
		NestedVirt:   plan.NestedVirt.ValueBool(),
		DiskSize:     int(plan.DiskSize.ValueInt64()),
		Zone:         plan.Zone.ValueString(),
		Project:      plan.Project.ValueString(),
	}

	sb, err := r.client.CreateSandbox(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError("Error creating sandbox", err.Error())
		return
	}

	// Wait for sandbox to be ready
	sb, err = r.client.WaitForSandbox(ctx, sb.ID, 5*time.Minute)
	if err != nil {
		resp.Diagnostics.AddError("Error waiting for sandbox", err.Error())
		return
	}

	plan.ID = types.StringValue(sb.ID)
	plan.Status = types.StringValue(sb.Status)
	plan.URL = types.StringValue(sb.URL)
	plan.VMUrl = types.StringValue(sb.Metadata.VMUrl)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SandboxResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state SandboxResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	sb, err := r.client.GetSandboxRefresh(ctx, state.ID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.IsNotFound() {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading sandbox", err.Error())
		return
	}

	if sb.Status == "destroyed" {
		resp.State.RemoveResource(ctx)
		return
	}

	state.Status = types.StringValue(sb.Status)
	if sb.Image != "" {
		state.Image = types.StringValue(sb.Image)
	}
	if sb.Compute != "" {
		state.ComputeClass = types.StringValue(sb.Compute)
	}
	if sb.Metadata.Cloud != "" {
		state.Cloud = types.StringValue(sb.Metadata.Cloud)
	}
	state.URL = types.StringValue(sb.URL)
	state.VMUrl = types.StringValue(sb.Metadata.VMUrl)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *SandboxResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Most sandbox attributes require replacement. No in-place updates currently supported.
	resp.Diagnostics.AddError("Update not supported", "Sandbox attributes that change require replacement.")
}

func (r *SandboxResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state SandboxResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteSandbox(ctx, state.ID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.IsNotFound() {
			return // already gone
		}
		resp.Diagnostics.AddError("Error deleting sandbox", err.Error())
	}
}

func (r *SandboxResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// Plan modifier helpers for types that don't have built-in RequiresReplace.

type boolRequiresReplace struct{}

func (m boolRequiresReplace) Description(_ context.Context) string          { return "requires replace" }
func (m boolRequiresReplace) MarkdownDescription(_ context.Context) string  { return "requires replace" }
func (m boolRequiresReplace) PlanModifyBool(_ context.Context, req planmodifier.BoolRequest, resp *planmodifier.BoolResponse) {
	if !req.StateValue.IsNull() && !req.PlanValue.IsNull() && req.StateValue != req.PlanValue {
		resp.RequiresReplace = true
	}
}

type int64RequiresReplace struct{}

func (m int64RequiresReplace) Description(_ context.Context) string          { return "requires replace" }
func (m int64RequiresReplace) MarkdownDescription(_ context.Context) string  { return "requires replace" }
func (m int64RequiresReplace) PlanModifyInt64(_ context.Context, req planmodifier.Int64Request, resp *planmodifier.Int64Response) {
	if !req.StateValue.IsNull() && !req.PlanValue.IsNull() && req.StateValue != req.PlanValue {
		resp.RequiresReplace = true
	}
}
