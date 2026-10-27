package provider

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/ap-1/terraform-provider-grouper/grouper"
	"github.com/hashicorp/terraform-plugin-testing/compare"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccGroupResource(t *testing.T) {
	root := acctest.RandomWithPrefix("tftest")
	sameID := statecheck.CompareValue(compare.ValuesSame())
	newID := statecheck.CompareValue(compare.ValuesDiffer())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckGroupsDestroyed(root+":members", root+":leads", root+"-moved:leads"),
		Steps: []resource.TestStep{
			{
				Config: testAccGroupConfig(root, "${grouper_folder.root.name}:members", `description = "first"`),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("grouper_group.test", tfjsonpath.New("display_extension"), knownvalue.StringExact("members")),
					statecheck.ExpectKnownValue("grouper_group.test", tfjsonpath.New("description"), knownvalue.StringExact("first")),
					sameID.AddStateValue("grouper_group.test", tfjsonpath.New("id")),
				},
			},
			{
				// Renaming within the same folder keeps the group
				Config: testAccGroupConfig(root, "${grouper_folder.root.name}:leads", `display_extension = "Leads"`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("grouper_group.test", plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("grouper_group.test", tfjsonpath.New("name"), knownvalue.StringExact(root+":leads")),
					statecheck.ExpectKnownValue("grouper_group.test", tfjsonpath.New("display_extension"), knownvalue.StringExact("Leads")),
					statecheck.ExpectKnownValue("grouper_group.test", tfjsonpath.New("description"), knownvalue.StringExact("")),
					sameID.AddStateValue("grouper_group.test", tfjsonpath.New("id")),
					newID.AddStateValue("grouper_group.test", tfjsonpath.New("id")),
				},
			},
			{
				// Grouper keeps the old name as an alias, which imports must not follow
				ResourceName:  "grouper_group.test",
				ImportState:   true,
				ImportStateId: root + ":members",
				ExpectError:   regexp.MustCompile(`Group not found`),
			},
			{
				// Moving to another folder replaces the group
				Config: testAccGroupConfig(root, "${grouper_folder.moved.name}:leads", ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("grouper_group.test", plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("grouper_group.test", tfjsonpath.New("display_extension"), knownvalue.StringExact("leads")),
					newID.AddStateValue("grouper_group.test", tfjsonpath.New("id")),
				},
			},
			{
				ResourceName:      "grouper_group.test",
				ImportState:       true,
				ImportStateId:     root + "-moved:leads",
				ImportStateVerify: true,
			},
		},
	})
}

// Two folders plus a group, whose name and extra attributes vary by step
func testAccGroupConfig(root, group, attributes string) string {
	return fmt.Sprintf(`
resource "grouper_folder" "root" {
  name = %q
}

resource "grouper_folder" "moved" {
  name = "%s-moved"
}

resource "grouper_group" "test" {
  name = "%s"
  %s
}
`, root, root, group, attributes)
}

func testAccCheckGroupsDestroyed(names ...string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		client := grouper.NewClient(os.Getenv("GROUPER_URL"), os.Getenv("GROUPER_USERNAME"), os.Getenv("GROUPER_PASSWORD"))
		for _, name := range names {
			group, err := client.FindGroup(context.Background(), name)
			if err != nil {
				return err
			}
			if group != nil {
				return fmt.Errorf("group %s still exists", name)
			}
		}
		return nil
	}
}

func TestAccGroupComposite(t *testing.T) {
	root := acctest.RandomWithPrefix("tftest")
	comp := "grouper_group.comp"
	leftIsLeft := statecheck.CompareValuePairs(comp, tfjsonpath.New("composite").AtMapKey("left_group"), "grouper_group.left", tfjsonpath.New("id"), compare.ValuesSame())
	rightIsRight := statecheck.CompareValuePairs(comp, tfjsonpath.New("composite").AtMapKey("right_group"), "grouper_group.right", tfjsonpath.New("id"), compare.ValuesSame())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckGroupsDestroyed(root+":left", root+":right", root+":comp"),
		Steps: []resource.TestStep{
			{
				Config: testAccCompositeConfig(root, `
  composite = {
    type        = "complement"
    left_group  = grouper_group.left.id
    right_group = grouper_group.right.id
  }
`),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(comp, tfjsonpath.New("composite").AtMapKey("type"), knownvalue.StringExact("complement")),
					leftIsLeft,
					rightIsRight,
				},
			},
			{
				Config: testAccCompositeConfig(root, `
  composite = {
    type        = "intersection"
    left_group  = grouper_group.left.id
    right_group = grouper_group.right.id
  }
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(comp, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(comp, tfjsonpath.New("composite").AtMapKey("type"), knownvalue.StringExact("intersection")),
				},
			},
			{
				ResourceName:      comp,
				ImportState:       true,
				ImportStateId:     root + ":comp",
				ImportStateVerify: true,
			},
			{
				Config: testAccCompositeConfig(root, `
  composite = {
    type        = "difference"
    left_group  = grouper_group.left.id
    right_group = grouper_group.right.id
  }
`),
				ExpectError: regexp.MustCompile(`value must be one of`),
			},
			{
				// Removing composite makes it an ordinary group again
				Config: testAccCompositeConfig(root, ""),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(comp, tfjsonpath.New("composite"), knownvalue.Null()),
				},
			},
		},
	})
}

// Two operand groups and a comp group with the given extra attributes
func testAccCompositeConfig(root, attributes string) string {
	return fmt.Sprintf(`
resource "grouper_folder" "root" {
  name = %q
}

resource "grouper_group" "left" {
  name = "${grouper_folder.root.name}:left"
}

resource "grouper_group" "right" {
  name = "${grouper_folder.root.name}:right"
}

resource "grouper_group" "comp" {
  name = "${grouper_folder.root.name}:comp"
%s}
`, root, attributes)
}
