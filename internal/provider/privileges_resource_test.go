package provider

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/ap-1/terraform-provider-grouper/grouper"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccPrivilegesResources(t *testing.T) {
	root := acctest.RandomWithPrefix("tftest")
	director := "grouper_group_privileges.director"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckGroupsDestroyed(root+":members", root+":director"),
		Steps: []resource.TestStep{
			{
				Config: testAccPrivilegesConfig(root, `["update", "read"]`),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(director, tfjsonpath.New("privileges"), knownvalue.SetExact([]knownvalue.Check{
						knownvalue.StringExact("read"),
						knownvalue.StringExact("update"),
					})),
					statecheck.ExpectKnownValue("grouper_group_privileges.charlie", tfjsonpath.New("privileges"), knownvalue.SetExact([]knownvalue.Check{
						knownvalue.StringExact("optin"),
					})),
					statecheck.ExpectKnownValue("grouper_group_privileges.everyone", tfjsonpath.New("privileges"), knownvalue.SetExact([]knownvalue.Check{
						knownvalue.StringExact("view"),
					})),
					statecheck.ExpectKnownValue("grouper_folder_privileges.director", tfjsonpath.New("privileges"), knownvalue.SetExact([]knownvalue.Check{
						knownvalue.StringExact("create"),
					})),
				},
			},
			{
				Config: testAccPrivilegesConfig(root, `["update"]`),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(director, tfjsonpath.New("privileges"), knownvalue.SetExact([]knownvalue.Check{
						knownvalue.StringExact("update"),
					})),
				},
			},
			{
				// A privilege granted outside OpenTofu to a declared subject is revoked
				PreConfig: func() { testAccGrantAdmin(t, root+":members", root+":director") },
				Config:    testAccPrivilegesConfig(root, `["update"]`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(director, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(director, tfjsonpath.New("privileges"), knownvalue.SetExact([]knownvalue.Check{
						knownvalue.StringExact("update"),
					})),
				},
			},
			{
				ResourceName:  director,
				ImportState:   true,
				ImportStateId: root + ":members " + root + ":director",
				ImportStateCheck: testAccCheckImportedPrivileges(map[string]string{
					"subject":        "",
					"subject_source": "",
					"privileges.#":   "1",
					"privileges.0":   "update",
				}),
			},
			{
				ResourceName:  "grouper_group_privileges.charlie",
				ImportState:   true,
				ImportStateId: root + ":members jdbc charlie",
				ImportStateCheck: testAccCheckImportedPrivileges(map[string]string{
					"subject":        "charlie",
					"subject_source": "jdbc",
					"subject_group":  "",
					"privileges.#":   "1",
					"privileges.0":   "optin",
				}),
			},
			{
				ResourceName:  "grouper_group_privileges.everyone",
				ImportState:   true,
				ImportStateId: root + ":members g:isa GrouperAll",
				ImportStateCheck: testAccCheckImportedPrivileges(map[string]string{
					"subject":        "GrouperAll",
					"subject_source": "g:isa",
					"privileges.#":   "1",
					"privileges.0":   "view",
				}),
			},
		},
	})
}

// Checks the one imported resource's attributes, where an empty value means the attribute is unset
func testAccCheckImportedPrivileges(want map[string]string) resource.ImportStateCheckFunc {
	return func(states []*terraform.InstanceState) error {
		if len(states) != 1 {
			return fmt.Errorf("expected 1 imported resource, got %d", len(states))
		}

		for key, value := range want {
			if got := states[0].Attributes[key]; got != value {
				return fmt.Errorf("imported %s is %q, expected %q", key, got, value)
			}
		}
		return nil
	}
}

func TestAccPrivilegesResourceValidation(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "grouper_group_privileges" "both" {
  group          = "00000000000000000000000000000000"
  subject        = "charlie"
  subject_source = "jdbc"
  subject_group  = "00000000000000000000000000000000"
  privileges     = ["read"]
}
`,
				ExpectError: regexp.MustCompile(`Invalid Attribute Combination`),
			},
			{
				Config: `
resource "grouper_group_privileges" "no_source" {
  group      = "00000000000000000000000000000000"
  subject    = "charlie"
  privileges = ["read"]
}
`,
				ExpectError: regexp.MustCompile(`Invalid Attribute Combination`),
			},
			{
				// stemAdmin is a folder privilege
				Config: `
resource "grouper_group_privileges" "folder_privilege" {
  group          = "00000000000000000000000000000000"
  subject        = "charlie"
  subject_source = "jdbc"
  privileges     = ["stemAdmin"]
}
`,
				ExpectError: regexp.MustCompile(`value must be one of`),
			},
		},
	})
}

// A members group, a director group with privileges on it and its folder, charlie in the director
// group with optin of his own, and everyone with view. charlie's inherited privileges must not
// show up as his own.
func testAccPrivilegesConfig(root, directorPrivileges string) string {
	return fmt.Sprintf(`
resource "grouper_folder" "root" {
  name = %q
}

resource "grouper_group" "members" {
  name = "${grouper_folder.root.name}:members"
}

resource "grouper_group" "director" {
  name = "${grouper_folder.root.name}:director"
}

resource "grouper_group_privileges" "director" {
  group         = grouper_group.members.id
  subject_group = grouper_group.director.id
  privileges    = %s
}

resource "grouper_group_members" "director" {
  group    = grouper_group.director.id
  subjects = { jdbc = ["charlie"] }
}

resource "grouper_group_privileges" "charlie" {
  group          = grouper_group.members.id
  subject        = "charlie"
  subject_source = "jdbc"
  privileges     = ["optin"]
}

resource "grouper_group_privileges" "everyone" {
  group          = grouper_group.members.id
  subject        = "GrouperAll"
  subject_source = "g:isa"
  privileges     = ["view"]
}

resource "grouper_folder_privileges" "director" {
  folder        = grouper_folder.root.id
  subject_group = grouper_group.director.id
  privileges    = ["create"]
}
`, root, directorPrivileges)
}

// Adds admin to the subject group's privileges on the group directly
func testAccGrantAdmin(t *testing.T, groupName, subjectGroupName string) {
	ctx := context.Background()
	client := grouper.NewClient(os.Getenv("GROUPER_URL"), os.Getenv("GROUPER_USERNAME"), os.Getenv("GROUPER_PASSWORD"))

	group, err := client.FindGroup(ctx, groupName)
	if err != nil || group == nil {
		t.Fatalf("finding %s: %v", groupName, err)
	}
	subjectGroup, err := client.FindGroup(ctx, subjectGroupName)
	if err != nil || subjectGroup == nil {
		t.Fatalf("finding %s: %v", subjectGroupName, err)
	}

	subject := grouper.Subject{ID: subjectGroup.UUID, SourceID: grouper.GroupSource}
	if err := client.SetGroupPrivileges(ctx, group.UUID, subject, []string{"update", "admin"}); err != nil {
		t.Fatal(err)
	}
}
