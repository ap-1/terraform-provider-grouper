resource "grouper_group_privileges" "director" {
  group         = grouper_group.members.id
  subject_group = grouper_group.director.id
  privileges    = ["update", "read"]
}
