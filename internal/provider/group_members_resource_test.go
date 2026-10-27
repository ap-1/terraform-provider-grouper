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
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccGroupMembersResource(t *testing.T) {
	root := acctest.RandomWithPrefix("tftest")
	members := "grouper_group_members.team"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckGroupsDestroyed(root+":team", root+":leads"),
		Steps: []resource.TestStep{
			{
				Config: testAccGroupMembersConfig(root, `
  subjects = ["charlie", "lucy"]
  groups   = [grouper_group.leads.id]
`),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(members, tfjsonpath.New("subjects"), knownvalue.SetExact([]knownvalue.Check{
						knownvalue.StringExact("charlie"),
						knownvalue.StringExact("lucy"),
					})),
					statecheck.ExpectKnownValue(members, tfjsonpath.New("groups"), knownvalue.SetSizeExact(1)),
				},
			},
			{
				// Omitting groups removes the nested group
				Config: testAccGroupMembersConfig(root, `
  subjects = ["linus"]
`),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(members, tfjsonpath.New("subjects"), knownvalue.SetExact([]knownvalue.Check{
						knownvalue.StringExact("linus"),
					})),
					statecheck.ExpectKnownValue(members, tfjsonpath.New("groups"), knownvalue.SetSizeExact(0)),
				},
			},
			{
				// An unknown person fails without changing the group
				Config: testAccGroupMembersConfig(root, `
  subjects = ["linus", "snoopy"]
`),
				ExpectError: regexp.MustCompile(`can't find subjects snoopy`),
			},
			{
				Config: testAccGroupMembersConfig(root, `
  subjects = ["linus"]
`),
				PlanOnly: true,
			},
			{
				// Members added outside OpenTofu are removed
				PreConfig: func() { testAccAddMember(t, root+":team", "charlie") },
				Config: testAccGroupMembersConfig(root, `
  subjects = ["linus"]
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(members, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(members, tfjsonpath.New("subjects"), knownvalue.SetExact([]knownvalue.Check{
						knownvalue.StringExact("linus"),
					})),
				},
			},
			{
				ResourceName:                         members,
				ImportState:                          true,
				ImportStateId:                        root + ":team",
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "group",
			},
		},
	})
}

// A team group with members set by attributes, and a leads group to nest in it
func testAccGroupMembersConfig(root, attributes string) string {
	return fmt.Sprintf(`
resource "grouper_folder" "root" {
  name = %q
}

resource "grouper_group" "team" {
  name = "${grouper_folder.root.name}:team"
}

resource "grouper_group" "leads" {
  name = "${grouper_folder.root.name}:leads"
}

resource "grouper_group_members" "team" {
  group = grouper_group.team.id
%s}
`, root, attributes)
}

// Adds a person to a group directly, keeping its other members
func testAccAddMember(t *testing.T, groupName, subjectID string) {
	ctx := context.Background()
	client := grouper.NewClient(os.Getenv("GROUPER_URL"), os.Getenv("GROUPER_USERNAME"), os.Getenv("GROUPER_PASSWORD"))

	group, err := client.FindGroup(ctx, groupName)
	if err != nil || group == nil {
		t.Fatalf("finding %s: %v", groupName, err)
	}
	members, err := client.GroupMembers(ctx, group.UUID)
	if err != nil {
		t.Fatal(err)
	}

	members = append(members, grouper.Subject{ID: subjectID, SourceID: os.Getenv("GROUPER_SUBJECT_SOURCE")})
	if err := client.SetGroupMembers(ctx, group.UUID, members); err != nil {
		t.Fatal(err)
	}
}
