package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccFolderDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// etc exists in every Grouper
				Config: `data "grouper_folder" "etc" { name = "etc" }`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("data.grouper_folder.etc", tfjsonpath.New("id"), knownvalue.StringRegexp(regexp.MustCompile(`^[0-9a-f]{32}$`))),
					statecheck.ExpectKnownValue("data.grouper_folder.etc", tfjsonpath.New("display_extension"), knownvalue.StringExact("etc")),
					statecheck.ExpectKnownValue("data.grouper_folder.etc", tfjsonpath.New("display_name"), knownvalue.StringExact("etc")),
				},
			},
			{
				Config:      `data "grouper_folder" "missing" { name = "does:not:exist" }`,
				ExpectError: regexp.MustCompile(`Folder not found`),
			},
		},
	})
}

func TestAccProviderMissingURL(t *testing.T) {
	t.Setenv("GROUPER_URL", "")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      `data "grouper_folder" "etc" { name = "etc" }`,
				ExpectError: regexp.MustCompile(`Missing Grouper url`),
			},
		},
	})
}
