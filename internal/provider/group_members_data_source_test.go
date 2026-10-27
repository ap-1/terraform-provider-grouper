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

func TestAccGroupMembersDataSource(t *testing.T) {
	root := acctest.RandomWithPrefix("tftest")
	jdbc := func(ids ...string) knownvalue.Check {
		checks := make([]knownvalue.Check, len(ids))
		for i, id := range ids {
			checks[i] = knownvalue.StringExact(id)
		}
		return knownvalue.MapExact(map[string]knownvalue.Check{"jdbc": knownvalue.SetExact(checks)})
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckGroupsDestroyed(root+":outer", root+":inner"),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "grouper_folder" "root" {
  name = %q
}

resource "grouper_group" "outer" {
  name = "${grouper_folder.root.name}:outer"
}

resource "grouper_group" "inner" {
  name = "${grouper_folder.root.name}:inner"
}

resource "grouper_group_members" "outer" {
  group    = grouper_group.outer.id
  subjects = { jdbc = ["charlie", "lucy"] }
  groups   = [grouper_group.inner.id]
}

resource "grouper_group_members" "inner" {
  group    = grouper_group.inner.id
  subjects = { jdbc = ["lucy", "linus"] }
}

data "grouper_group_members" "all" {
  group      = grouper_group.outer.id
  depends_on = [grouper_group_members.outer, grouper_group_members.inner]
}

data "grouper_group_members" "immediate" {
  group      = grouper_group.outer.id
  immediate  = true
  depends_on = [grouper_group_members.outer, grouper_group_members.inner]
}
`, root),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("data.grouper_group_members.all", tfjsonpath.New("subjects"), jdbc("charlie", "lucy", "linus")),
					statecheck.ExpectKnownValue("data.grouper_group_members.immediate", tfjsonpath.New("subjects"), jdbc("charlie", "lucy")),
					statecheck.CompareValuePairs("data.grouper_group_members.all", tfjsonpath.New("groups").AtSliceIndex(0), "grouper_group.inner", tfjsonpath.New("id"), compare.ValuesSame()),
					statecheck.ExpectKnownValue("data.grouper_group_members.all", tfjsonpath.New("groups"), knownvalue.SetSizeExact(1)),
					statecheck.ExpectKnownValue("data.grouper_group_members.immediate", tfjsonpath.New("groups"), knownvalue.SetSizeExact(1)),
				},
			},
			{
				Config:      `data "grouper_group_members" "missing" { group = "00000000000000000000000000000000" }`,
				ExpectError: regexp.MustCompile(`Group not found`),
			},
		},
	})
}
