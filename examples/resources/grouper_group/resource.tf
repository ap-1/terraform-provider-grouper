resource "grouper_group" "admins" {
  name              = "org:dept:apps:admins"
  display_extension = "Administrators"
  description       = "People who administer the department's applications"
}
