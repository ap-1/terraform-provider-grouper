data "grouper_group_members" "admins" {
  group = data.grouper_group.admins.id
}
