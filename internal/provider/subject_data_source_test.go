package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccSubjectDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
data "grouper_subject" "charlie" {
  id     = "charlie"
  source = "jdbc"
}

data "grouper_subject" "everyone" {
  id     = "GrouperAll"
  source = "g:isa"
}
`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("data.grouper_subject.charlie", tfjsonpath.New("name"), knownvalue.StringExact("Charlie Brown")),
					statecheck.ExpectKnownValue("data.grouper_subject.everyone", tfjsonpath.New("name"), knownvalue.StringExact("EveryEntity")),
					statecheck.ExpectKnownValue("data.grouper_subject.everyone", tfjsonpath.New("description"), knownvalue.StringExact("EveryEntity")),
				},
			},
			{
				Config: `
data "grouper_subject" "missing" {
  id     = "snoopy"
  source = "jdbc"
}
`,
				ExpectError: regexp.MustCompile(`Subject not found`),
			},
			{
				// A subject only matches in its own source
				Config: `
data "grouper_subject" "wrong_source" {
  id     = "charlie"
  source = "g:isa"
}
`,
				ExpectError: regexp.MustCompile(`Subject not found`),
			},
		},
	})
}
