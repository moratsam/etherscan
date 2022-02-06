create table if not exists wallet (
	address varchar(40) primary key,
	crawled boolean not null default false
);
