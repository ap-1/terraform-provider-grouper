package provider

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// Starts this provider in-process for each tofu command a test runs
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"grouper": providerserver.NewProtocol6WithError(New("test")()),
}

func testAccPreCheck(t *testing.T) {
	for _, env := range []string{"GROUPER_URL", "GROUPER_USERNAME", "GROUPER_PASSWORD"} {
		if os.Getenv(env) == "" {
			t.Fatalf("%s must be set for acceptance tests", env)
		}
	}
}
