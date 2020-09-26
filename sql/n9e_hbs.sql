set names utf8;

drop database if exists n9e_hbs;
create database n9e_hbs;
use n9e_hbs;

create table `instance` (
  `id`        int unsigned not null auto_increment,
  `module`    varchar(32) not null,  
  `identity`  varchar(255) not null,
  `rpc_port`  varchar(16) not null,
  `http_port` varchar(16) not null,
  `remark` 	  text,
  `ts`        int unsigned not null,
  primary key (`id`),
  key(`module`,`identity`,`rpc_port`,`http_port`)
) engine=innodb default charset=utf8;

create table `detector` (
  `id`      int unsigned not null auto_increment,
  `module`  varchar(32) not null,  
  `node`    varchar(16) not null,
  `region`  varchar(64) not null,
  `ip`      varchar(255) not null,
  `port`    varchar(16) not null,
  `ts`      int unsigned not null,
  primary key (`id`),
  key(`ip`,`port`)
) engine=innodb default charset=utf8;