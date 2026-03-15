package provider_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccSandboxResource_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read
			{
				Config: `
resource "panes_sandbox" "test" {
  compute_class = "standard"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("panes_sandbox.test", "id"),
					resource.TestCheckResourceAttr("panes_sandbox.test", "status", "running"),
				),
			},
			// Import
			{
				ResourceName:            "panes_sandbox.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"instance_type", "nested_virt", "disk_size", "zone", "project"},
			},
		},
	})
}
