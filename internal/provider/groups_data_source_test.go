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
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccGroupsDataSource(t *testing.T) {
	root := acctest.RandomWithPrefix("tftest")
	names := func(names ...string) knownvalue.Check {
		checks := make([]knownvalue.Check, len(names))
		for i, name := range names {
			checks[i] = knownvalue.ObjectPartial(map[string]knownvalue.Check{"name": knownvalue.StringExact(root + ":" + name)})
		}
		return knownvalue.ListExact(checks)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckGroupsDestroyed(root+":b", root+":a", root+":sub:c"),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "grouper_folder" "root" {
  name = %q
}

resource "grouper_folder" "sub" {
  name = "${grouper_folder.root.name}:sub"
}

resource "grouper_group" "b" {
  name = "${grouper_folder.root.name}:b"
}

resource "grouper_group" "a" {
  name = "${grouper_folder.root.name}:a"
  composite = {
    type        = "union"
    left_group  = grouper_group.b.id
    right_group = grouper_group.c.id
  }
}

resource "grouper_group" "c" {
  name = "${grouper_folder.sub.name}:c"
}

data "grouper_groups" "one_level" {
  folder     = grouper_folder.root.name
  depends_on = [grouper_group.a, grouper_group.b, grouper_group.c]
}

data "grouper_groups" "recursive" {
  folder     = grouper_folder.root.name
  recursive  = true
  depends_on = [grouper_group.a, grouper_group.b, grouper_group.c]
}
`, root),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("data.grouper_groups.one_level", tfjsonpath.New("groups"), names("a", "b")),
					statecheck.ExpectKnownValue("data.grouper_groups.recursive", tfjsonpath.New("groups"), names("a", "b", "sub:c")),
					statecheck.CompareValuePairs("data.grouper_groups.one_level", tfjsonpath.New("groups").AtSliceIndex(0).AtMapKey("id"), "grouper_group.a", tfjsonpath.New("id"), compare.ValuesSame()),
					statecheck.ExpectKnownValue("data.grouper_groups.one_level", tfjsonpath.New("groups").AtSliceIndex(0).AtMapKey("composite").AtMapKey("type"), knownvalue.StringExact("union")),
					statecheck.ExpectKnownValue("data.grouper_groups.one_level", tfjsonpath.New("groups").AtSliceIndex(1).AtMapKey("composite"), knownvalue.Null()),
				},
			},
			{
				// Grouper lists nothing for a missing folder, so this has to fail rather than come back empty
				Config:      fmt.Sprintf(`data "grouper_groups" "missing" { folder = "%s:missing" }`, root),
				ExpectError: regexp.MustCompile(`Folder not found`),
			},
		},
	})
}

// More groups than fit in one page
func TestAccGroupsDataSourcePaging(t *testing.T) {
	root := acctest.RandomWithPrefix("tftest")
	const count = 101

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig: func() { testAccCreateGroups(t, root, count) },
				Config:    fmt.Sprintf(`data "grouper_groups" "all" { folder = %q }`, root),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("data.grouper_groups.all", tfjsonpath.New("groups"), knownvalue.ListSizeExact(count)),
					statecheck.ExpectKnownValue("data.grouper_groups.all", tfjsonpath.New("groups").AtSliceIndex(count-1).AtMapKey("name"), knownvalue.StringExact(fmt.Sprintf("%s:g%03d", root, count-1))),
				},
			},
		},
	})
}

// Creates a folder of count groups directly, deleting them when the test ends
func testAccCreateGroups(t *testing.T, folderName string, count int) {
	ctx := context.Background()
	client := grouper.NewClient(os.Getenv("GROUPER_URL"), os.Getenv("GROUPER_USERNAME"), os.Getenv("GROUPER_PASSWORD"))

	folder, err := client.CreateStem(ctx, grouper.Stem{Name: folderName})
	if err != nil {
		t.Fatal(err)
	}
	var created []*grouper.Group
	t.Cleanup(func() {
		for _, group := range created {
			if err := client.DeleteGroup(ctx, group.UUID); err != nil {
				t.Error(err)
			}
		}
		if err := client.DeleteStem(ctx, folder.UUID); err != nil {
			t.Error(err)
		}
	})

	for i := range count {
		group, err := client.CreateGroup(ctx, grouper.Group{Name: fmt.Sprintf("%s:g%03d", folderName, i)})
		if err != nil {
			t.Fatal(err)
		}
		created = append(created, group)
	}
}
