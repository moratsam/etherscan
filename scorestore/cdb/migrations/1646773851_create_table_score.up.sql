create table if not exists score (
	id bigserial primary key,
	wallet varchar(40) not null,
	scorer serial not null references scorer(id) on delete cascade,
	value numeric not null,
	timestamp timestamp not null
);
