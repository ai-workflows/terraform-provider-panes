package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

func TestManagedAgentOwnerScopeIsRequired(t *testing.T) {
	resourceImpl := NewManagedAgentResource().(*ManagedAgentResource)
	var resp resource.SchemaResponse
	resourceImpl.Schema(context.Background(), resource.SchemaRequest{}, &resp)

	for _, name := range []string{"org_id", "user_id"} {
		attr, ok := resp.Schema.Attributes[name].(schema.StringAttribute)
		if !ok {
			t.Fatalf("%s attribute has unexpected type %T", name, resp.Schema.Attributes[name])
		}
		if !attr.Required {
			t.Errorf("%s must be required", name)
		}
		if attr.Optional || attr.Computed {
			t.Errorf("%s must not be optional or computed", name)
		}
		if len(attr.PlanModifiers) != 1 {
			t.Errorf("%s must have one plan modifier to prevent in-place owner changes; got %d", name, len(attr.PlanModifiers))
		}
	}
}
