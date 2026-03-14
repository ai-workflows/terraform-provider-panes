package provider_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccSandboxResource_basic(t *testing.T) {
	// Acceptance tests require PANES_TOKEN and a running Panes API.
	// Skip if not configured.
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read
			{
				Config: `
resource "panes_sandbox" "test" {
  cloud         = "gcp"
  instance_type = "n2-standard-2"
  disk_size     = 20
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("panes_sandbox.test", "id"),
					resource.TestCheckResourceAttr("panes_sandbox.test", "cloud", "gcp"),
					resource.TestCheckResourceAttr("panes_sandbox.test", "status", "running"),
					resource.TestCheckResourceAttrSet("panes_sandbox.test", "vm_url"),
				),
			},
			// Import
			{
				ResourceName:      "panes_sandbox.test",
				ImportState:       true,
				ImportStateVerify: true,
				// These fields may not be returned by the API on import
				ImportStateVerifyIgnore: []string{"instance_type", "nested_virt", "disk_size", "zone", "project"},
			},
		},
	})
}
