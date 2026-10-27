resource "grouper_group_privileges" "director" {
  group         = grouper_group.members.id
  subject_group = grouper_group.director.id
  privileges    = ["update", "read"]
}

resource "grouper_group_privileges" "charlie" {
  group          = grouper_group.members.id
  subject        = "charlie"
  subject_source = "ldap"
  privileges     = ["read"]
}

# Grouper's built-in subject for everyone
resource "grouper_group_privileges" "everyone" {
  group          = grouper_group.members.id
  subject        = "GrouperAll"
  subject_source = "g:isa"
  privileges     = ["view"]
}
