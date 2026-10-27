resource "grouper_group_members" "admins" {
  group    = grouper_group.admins.id
  subjects = ["charlie", "lucy"]
  groups   = [grouper_group.leads.id]
}
