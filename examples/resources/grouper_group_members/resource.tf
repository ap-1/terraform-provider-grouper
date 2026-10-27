resource "grouper_group_members" "admins" {
  group    = grouper_group.admins.id
  subjects = { ldap = ["charlie", "lucy"] }
  groups   = [grouper_group.leads.id]

  scheduled = [
    { subject = "linus", source = "ldap", ends = "2026-05-15T00:00:00-04:00" },
    { group = grouper_group.graders.id, starts = "2026-08-20T00:00:00-04:00" },
  ]
}
