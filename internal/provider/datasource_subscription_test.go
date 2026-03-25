package provider_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccSubscriptionDataSource_byLabel(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
data "panes_subscription" "test" {
  label = "ChatGPT Pro - chatgpt@aiworkflows.com"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.panes_subscription.test", "id"),
					resource.TestCheckResourceAttr("data.panes_subscription.test", "service_provider", "chatgpt"),
					resource.TestCheckResourceAttr("data.panes_subscription.test", "status", "active"),
				),
			},
		},
	})
}
