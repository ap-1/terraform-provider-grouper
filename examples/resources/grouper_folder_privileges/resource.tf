resource "grouper_folder_privileges" "director" {
  folder        = grouper_folder.apps.id
  subject_group = grouper_group.director.id
  privileges    = ["create"]
}
