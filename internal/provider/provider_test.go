package provider_test

import (
	"github.com/ai-workflows/terraform-provider-panes/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"panes": providerserver.NewProtocol6WithError(provider.New("test")()),
}
