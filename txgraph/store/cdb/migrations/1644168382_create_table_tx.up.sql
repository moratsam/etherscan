create table if not exists tx (
	hash varchar(65) primary key,
	status int not null,
	block int not null,
	timestamp timestamp not null,
	"from" varchar(40) not null references wallet(address) on delete cascade,
	"to" varchar(40) not null references wallet(address) on delete cascade,
	value numeric not null default 0,
	transaction_fee numeric not null,
	data bytea
);

create index if not exists idx_tx_from on tx("from");
create index if not exists idx_tx_to on tx("to");
