create table if not exists block (
	"number" int primary key,
	processed boolean not null default false
);

