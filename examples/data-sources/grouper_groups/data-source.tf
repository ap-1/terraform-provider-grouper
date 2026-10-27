data "grouper_groups" "apps" {
  folder    = "org:dept:apps"
  recursive = true
}

# Grants a group read on every group in the folder and its subfolders
resource "grouper_group_privileges" "auditors" {
  for_each = { for g in data.grouper_groups.apps.groups : g.name => g.id }

  group         = each.value
  subject_group = grouper_group.auditors.id
  privileges    = ["read"]
}
