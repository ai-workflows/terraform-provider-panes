package provider_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccAgentResource_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read
			{
				Config: `
resource "panes_agent" "test" {
  name              = "tf-test-agent"
  template_id       = "custom"
  model             = "chatgpt:gpt-5.4"
  system_prompt     = "You are a test agent."
  autopilot_prompt  = "Do nothing. Wait for instructions."
  done_for_now_enabled = true
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("panes_agent.test", "id"),
					resource.TestCheckResourceAttr("panes_agent.test", "name", "tf-test-agent"),
					resource.TestCheckResourceAttr("panes_agent.test", "model", "chatgpt:gpt-5.4"),
				),
			},
			// Update
			{
				Config: `
resource "panes_agent" "test" {
  name              = "tf-test-agent-renamed"
  template_id       = "custom"
  model             = "chatgpt:gpt-5.4"
  system_prompt     = "You are an updated test agent."
  autopilot_prompt  = "Still do nothing."
  done_for_now_enabled = false
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("panes_agent.test", "name", "tf-test-agent-renamed"),
					resource.TestCheckResourceAttr("panes_agent.test", "done_for_now_enabled", "false"),
				),
			},
			// Import
			{
				ResourceName:      "panes_agent.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
