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

func TestAccFolderResource(t *testing.T) {
	root := acctest.RandomWithPrefix("tftest")
	sameID := statecheck.CompareValue(compare.ValuesSame())
	newID := statecheck.CompareValue(compare.ValuesDiffer())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckFoldersDestroyed(root, root+"-moved"),
		Steps: []resource.TestStep{
			{
				Config: testAccFolderConfig(root, "${grouper_folder.root.name}:a", `description = "first"`),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("grouper_folder.child", tfjsonpath.New("display_extension"), knownvalue.StringExact("a")),
					statecheck.ExpectKnownValue("grouper_folder.child", tfjsonpath.New("description"), knownvalue.StringExact("first")),
					sameID.AddStateValue("grouper_folder.child", tfjsonpath.New("id")),
				},
			},
			{
				// Renaming within the same parent keeps the folder
				Config: testAccFolderConfig(root, "${grouper_folder.root.name}:b", `display_extension = "B"`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("grouper_folder.child", plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("grouper_folder.child", tfjsonpath.New("name"), knownvalue.StringExact(root+":b")),
					statecheck.ExpectKnownValue("grouper_folder.child", tfjsonpath.New("display_extension"), knownvalue.StringExact("B")),
					statecheck.ExpectKnownValue("grouper_folder.child", tfjsonpath.New("description"), knownvalue.StringExact("")),
					sameID.AddStateValue("grouper_folder.child", tfjsonpath.New("id")),
					newID.AddStateValue("grouper_folder.child", tfjsonpath.New("id")),
				},
			},
			{
				// Grouper keeps the old name as an alias, which lookups must not follow
				Config: testAccFolderConfig(root, "${grouper_folder.root.name}:b", `display_extension = "B"`) + fmt.Sprintf(`
data "grouper_folder" "old_name" {
  name = "%s:a"
}
`, root),
				ExpectError: regexp.MustCompile(`Folder not found`),
			},
			{
				// Moving to another parent replaces the folder
				Config: testAccFolderConfig(root, "${grouper_folder.moved.name}:b", ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("grouper_folder.child", plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("grouper_folder.child", tfjsonpath.New("display_extension"), knownvalue.StringExact("b")),
					newID.AddStateValue("grouper_folder.child", tfjsonpath.New("id")),
				},
			},
			{
				ResourceName:      "grouper_folder.child",
				ImportState:       true,
				ImportStateId:     root + "-moved:b",
				ImportStateVerify: true,
			},
		},
	})
}

// Two parent folders plus a child, whose name and extra attributes vary by step
func testAccFolderConfig(root, child, attributes string) string {
	return fmt.Sprintf(`
resource "grouper_folder" "root" {
  name = %q
}

resource "grouper_folder" "moved" {
  name = "%s-moved"
}

resource "grouper_folder" "child" {
  name = "%s"
  %s
}
`, root, root, child, attributes)
}

func testAccCheckFoldersDestroyed(names ...string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		client := grouper.NewClient(os.Getenv("GROUPER_URL"), os.Getenv("GROUPER_USERNAME"), os.Getenv("GROUPER_PASSWORD"))
		for _, name := range names {
			stem, err := client.FindStem(context.Background(), name)
			if err != nil {
				return err
			}
			if stem != nil {
				return fmt.Errorf("folder %s still exists", name)
			}
		}
		return nil
	}
}
