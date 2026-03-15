package provider_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccAgentInstanceResource_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "panes_agent" "test" {
  name             = "tf-instance-test"
  template_id      = "custom"
  model            = "chatgpt:gpt-5.4"
  autopilot_prompt = "Say hello and then immediately call the done_for_now tool."
}

resource "panes_agent_instance" "test" {
  agent_id = panes_agent.test.id
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("panes_agent_instance.test", "id"),
					resource.TestCheckResourceAttrSet("panes_agent_instance.test", "agent_id"),
					resource.TestCheckResourceAttr("panes_agent_instance.test", "status", "running"),
				),
			},
		},
	})
}
