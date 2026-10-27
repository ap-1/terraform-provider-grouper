package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/compare"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccGroupDataSource(t *testing.T) {
	root := acctest.RandomWithPrefix("tftest")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckGroupsDestroyed(root+":left", root+":right", root+":comp"),
		Steps: []resource.TestStep{
			{
				Config: testAccCompositeConfig(root, `
  display_extension = "Comp"
  description       = "Left but not right"
  composite = {
    type        = "complement"
    left_group  = grouper_group.left.id
    right_group = grouper_group.right.id
  }
`) + `
data "grouper_group" "comp" {
  name = grouper_group.comp.name
}

data "grouper_group" "left" {
  name = grouper_group.left.name
}
`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.CompareValuePairs("data.grouper_group.comp", tfjsonpath.New("id"), "grouper_group.comp", tfjsonpath.New("id"), compare.ValuesSame()),
					statecheck.ExpectKnownValue("data.grouper_group.comp", tfjsonpath.New("display_extension"), knownvalue.StringExact("Comp")),
					statecheck.ExpectKnownValue("data.grouper_group.comp", tfjsonpath.New("display_name"), knownvalue.StringExact(root+":Comp")),
					statecheck.ExpectKnownValue("data.grouper_group.comp", tfjsonpath.New("description"), knownvalue.StringExact("Left but not right")),
					statecheck.ExpectKnownValue("data.grouper_group.comp", tfjsonpath.New("composite").AtMapKey("type"), knownvalue.StringExact("complement")),
					statecheck.CompareValuePairs("data.grouper_group.comp", tfjsonpath.New("composite").AtMapKey("left_group"), "grouper_group.left", tfjsonpath.New("id"), compare.ValuesSame()),
					statecheck.ExpectKnownValue("data.grouper_group.left", tfjsonpath.New("composite"), knownvalue.Null()),
				},
			},
			{
				Config:      fmt.Sprintf(`data "grouper_group" "missing" { name = "%s:missing" }`, root),
				ExpectError: regexp.MustCompile(`Group not found`),
			},
		},
	})
}
