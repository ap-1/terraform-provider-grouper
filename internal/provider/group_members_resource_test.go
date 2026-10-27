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
  subjects = { jdbc = ["charlie", "lucy"] }
  groups   = [grouper_group.leads.id]
`),
				ConfigStateChecks: []statecheck.StateCheck{
					testAccExpectSubjects(members, "charlie", "lucy"),
					statecheck.ExpectKnownValue(members, tfjsonpath.New("groups"), knownvalue.SetSizeExact(1)),
				},
			},
			{
				// Omitting groups removes the nested group
				Config: testAccGroupMembersConfig(root, `
  subjects = { jdbc = ["linus"] }
`),
				ConfigStateChecks: []statecheck.StateCheck{
					testAccExpectSubjects(members, "linus"),
					statecheck.ExpectKnownValue(members, tfjsonpath.New("groups"), knownvalue.SetSizeExact(0)),
				},
			},
			{
				// An unknown person fails without changing the group
				Config: testAccGroupMembersConfig(root, `
  subjects = { jdbc = ["linus", "snoopy"] }
`),
				ExpectError: regexp.MustCompile(`can't find subjects snoopy`),
			},
			{
				// So does a known person in the wrong source
				Config: testAccGroupMembersConfig(root, `
  subjects = { jdbc = ["linus"], "g:isa" = ["charlie"] }
`),
				ExpectError: regexp.MustCompile(`can't find subjects charlie`),
			},
			{
				Config: testAccGroupMembersConfig(root, `
  subjects = { jdbc = ["linus"] }
`),
				PlanOnly: true,
			},
			{
				// Members added outside OpenTofu are removed
				PreConfig: func() { testAccAddMember(t, root+":team", "charlie", "jdbc") },
				Config: testAccGroupMembersConfig(root, `
  subjects = { jdbc = ["linus"] }
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(members, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					testAccExpectSubjects(members, "linus"),
				},
			},
			{
				ResourceName:                         members,
				ImportState:                          true,
				ImportStateId:                        root + ":team",
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "group",
			},
			{
				// Removing everyone leaves no source keys behind
				Config: testAccGroupMembersConfig(root, ""),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(members, tfjsonpath.New("subjects"), knownvalue.MapExact(map[string]knownvalue.Check{})),
				},
			},
		},
	})
}

// Expects the resource's subjects to be exactly ids from the jdbc source
func testAccExpectSubjects(address string, ids ...string) statecheck.StateCheck {
	checks := make([]knownvalue.Check, len(ids))
	for i, id := range ids {
		checks[i] = knownvalue.StringExact(id)
	}
	return statecheck.ExpectKnownValue(address, tfjsonpath.New("subjects"), knownvalue.MapExact(map[string]knownvalue.Check{
		"jdbc": knownvalue.SetExact(checks),
	}))
}

func TestAccGroupMembersScheduled(t *testing.T) {
	root := acctest.RandomWithPrefix("tftest")
	members := "grouper_group_members.team"
	scheduled := func(count int) statecheck.StateCheck {
		return statecheck.ExpectKnownValue(members, tfjsonpath.New("scheduled"), knownvalue.SetSizeExact(count))
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckGroupsDestroyed(root+":team", root+":leads"),
		Steps: []resource.TestStep{
			{
				// Offsets other than the server's survive the round trip, along with a nested group
				// that hasn't started yet
				Config: testAccGroupMembersConfig(root, `
  subjects = { jdbc = ["charlie"] }
  scheduled = [
    { subject = "lucy", source = "jdbc", ends = "2031-01-02T03:04:05-05:00" },
    { subject = "linus", source = "jdbc", starts = "2030-01-01T00:00:00Z", ends = "2031-01-01T00:00:00Z" },
    { group = grouper_group.leads.id, starts = "2030-01-01T00:00:00.5Z" },
  ]
`),
				ConfigStateChecks: []statecheck.StateCheck{
					testAccExpectSubjects(members, "charlie"),
					scheduled(3),
					statecheck.ExpectKnownValue(members, tfjsonpath.New("scheduled"), knownvalue.SetPartial([]knownvalue.Check{
						knownvalue.ObjectPartial(map[string]knownvalue.Check{
							"subject": knownvalue.StringExact("lucy"),
							"ends":    knownvalue.StringExact("2031-01-02T03:04:05-05:00"),
						}),
					})),
				},
			},
			{
				// Moving lucy to subjects clears her end date, and linus's has passed
				Config: testAccGroupMembersConfig(root, `
  subjects = { jdbc = ["charlie", "lucy"] }
  scheduled = [
    { subject = "linus", source = "jdbc", ends = "2020-01-01T00:00:00Z" },
  ]
`),
				ConfigStateChecks: []statecheck.StateCheck{
					testAccExpectSubjects(members, "charlie", "lucy"),
					scheduled(1),
				},
			},
			{
				ResourceName:                         members,
				ImportState:                          true,
				ImportStateId:                        root + ":team",
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "group",
			},
			{
				// An ended member is still removed when it leaves the config
				Config: testAccGroupMembersConfig(root, `
  subjects = { jdbc = ["charlie", "lucy"] }
`),
				ConfigStateChecks: []statecheck.StateCheck{
					scheduled(0),
				},
			},
		},
	})
}

func TestAccGroupMembersScheduledNeedsTimeZone(t *testing.T) {
	t.Setenv("GROUPER_TIME_ZONE", "")
	root := acctest.RandomWithPrefix("tftest")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckGroupsDestroyed(root+":team", root+":leads"),
		Steps: []resource.TestStep{
			{
				Config: testAccGroupMembersConfig(root, `
  scheduled = [{ subject = "lucy", source = "jdbc", ends = "2031-01-01T00:00:00Z" }]
`),
				ExpectError: regexp.MustCompile(`need the server's time zone`),
			},
		},
	})
}

func TestAccGroupMembersResourceValidation(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// Groups go in groups
				Config: `
resource "grouper_group_members" "x" {
  group    = "00000000000000000000000000000000"
  subjects = { "g:gsa" = ["00000000000000000000000000000000"] }
}
`,
				ExpectError: regexp.MustCompile(`value must be none of`),
			},
			{
				Config: `
resource "grouper_group_members" "x" {
  group    = "00000000000000000000000000000000"
  subjects = { jdbc = [] }
}
`,
				ExpectError: regexp.MustCompile(`set must contain at least 1 elements`),
			},
			{
				Config: `
resource "grouper_group_members" "x" {
  group     = "00000000000000000000000000000000"
  subjects  = { jdbc = ["charlie"] }
  scheduled = [{ subject = "charlie", source = "jdbc", ends = "2031-01-01T00:00:00Z" }]
}
`,
				ExpectError: regexp.MustCompile(`Duplicate member`),
			},
			{
				Config: `
resource "grouper_group_members" "x" {
  group     = "00000000000000000000000000000000"
  scheduled = [{ subject = "charlie", source = "jdbc", starts = "2031-01-01T00:00:00Z", ends = "2030-01-01T00:00:00Z" }]
}
`,
				ExpectError: regexp.MustCompile(`Membership ends before it starts`),
			},
			{
				// Undated members go in subjects
				Config: `
resource "grouper_group_members" "x" {
  group     = "00000000000000000000000000000000"
  scheduled = [{ subject = "charlie", source = "jdbc" }]
}
`,
				ExpectError: regexp.MustCompile(`Invalid Attribute Combination`),
			},
			{
				Config: `
resource "grouper_group_members" "x" {
  group     = "00000000000000000000000000000000"
  scheduled = [{ subject = "charlie", ends = "2031-01-01T00:00:00Z" }]
}
`,
				ExpectError: regexp.MustCompile(`Invalid Attribute Combination`),
			},
			{
				Config: `
resource "grouper_group_members" "x" {
  group     = "00000000000000000000000000000000"
  scheduled = [{ subject = "charlie", source = "jdbc", ends = "2031-01-01" }]
}
`,
				ExpectError: regexp.MustCompile(`Invalid time`),
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

// Adds a subject to a group directly, keeping its other members
func testAccAddMember(t *testing.T, groupName, subjectID, source string) {
	ctx := context.Background()
	client := grouper.NewClient(os.Getenv("GROUPER_URL"), os.Getenv("GROUPER_USERNAME"), os.Getenv("GROUPER_PASSWORD"))

	group, err := client.FindGroup(ctx, groupName)
	if err != nil || group == nil {
		t.Fatalf("finding %s: %v", groupName, err)
	}
	memberships, err := client.GroupMemberships(ctx, group.UUID)
	if err != nil {
		t.Fatal(err)
	}

	memberships = append(memberships, grouper.Membership{Subject: grouper.Subject{ID: subjectID, SourceID: source}})
	if err := client.SetGroupMemberships(ctx, group.UUID, memberships); err != nil {
		t.Fatal(err)
	}
}
