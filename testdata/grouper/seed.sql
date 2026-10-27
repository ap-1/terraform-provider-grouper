-- People for the jdbc subject source, which reads these tables in Grouper's own database

insert into subject (subjectid, subjecttypeid, name)
select id, 'person', name
from (values ('charlie', 'Charlie Brown'), ('lucy', 'Lucy van Pelt'), ('linus', 'Linus van Pelt')) as people(id, name)
where not exists (select 1 from subject where subjectid = people.id);

insert into subjectattribute (subjectid, name, value, searchvalue)
select id, attribute, value, lower(value)
from (values
    ('charlie', 'name', 'Charlie Brown'), ('charlie', 'loginid', 'charlie'),
    ('lucy', 'name', 'Lucy van Pelt'), ('lucy', 'loginid', 'lucy'),
    ('linus', 'name', 'Linus van Pelt'), ('linus', 'loginid', 'linus')
) as attributes(id, attribute, value)
where not exists (select 1 from subjectattribute where subjectid = attributes.id and name = attributes.attribute);
