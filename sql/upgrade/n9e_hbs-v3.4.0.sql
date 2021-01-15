set names utf8;
use n9e_hbs;

alter table instance add `region` varchar(32) not null default 'default' after http_port;
