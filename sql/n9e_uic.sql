set names utf8;

drop database if exists n9e_uic;
create database n9e_uic;
use n9e_uic;

CREATE TABLE `user` (
  `id` int unsigned not null AUTO_INCREMENT,
  `username` varchar(32) not null comment 'login name, cannot rename',
  `password` varchar(128) not null default '',
  `dispname` varchar(32) not null default '' comment 'display name, chinese name',
  `phone` varchar(16) not null default '',
  `email` varchar(64) not null default '',
  `im` varchar(64) not null default '',
  `is_root` int(1) not null,
  PRIMARY KEY (`id`),
  UNIQUE KEY (`username`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

CREATE TABLE `invite` (
  `id` int unsigned not null AUTO_INCREMENT,
  `token` varchar(128) not null,
  `expire` bigint not null,
  `creator` varchar(32) not null,
  PRIMARY KEY (`id`),
  UNIQUE KEY (`token`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

CREATE TABLE `team` (
  `id` int unsigned not null AUTO_INCREMENT,
  `ident` varchar(255) not null,
  `name` varchar(255) not null default '',
  `mgmt` int(1) not null comment '0: member manage; 1: admin manage',
  PRIMARY KEY (`id`),
  UNIQUE KEY (`ident`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

CREATE TABLE `team_user` (
  `team_id` int unsigned not null,
  `user_id` int unsigned not null,
  `is_admin` int(1) not null,
  KEY (`team_id`),
  KEY (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
