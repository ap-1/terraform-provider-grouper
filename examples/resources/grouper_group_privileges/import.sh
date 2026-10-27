# Import by the group's current full name, then the subject: a subject source and subject ID, or a
# group's current full name
tofu import grouper_group_privileges.director 'org:dept:members org:dept:director'
tofu import grouper_group_privileges.charlie 'org:dept:members ldap charlie'
