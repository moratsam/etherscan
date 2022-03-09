create table if not exists score (
	id bigserial primary key,
	wallet varchar(40) not null,
	scorer varchar(73) not null references scorer(name) on delete cascade,
	value numeric not null,
	timestamp timestamp not null
);
