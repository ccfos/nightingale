set names utf8;
use n9e_rdb;

CREATE TABLE `white_list` (
  `id`           bigint unsigned not null AUTO_INCREMENT,
  `start_ip`     varchar(32)      DEFAULT '0'    NOT NULL,
  `end_ip`       varchar(32)      DEFAULT '0'    NOT NULL,
  `start_ip_int` bigint           DEFAULT '0'    NOT NULL,
  `end_ip_int`   bigint           DEFAULT '0'    NOT NULL,
  `start_time`   bigint           DEFAULT '0'    NOT NULL,
  `end_time`     bigint           DEFAULT '0'    NOT NULL,
  `created_at`   bigint           DEFAULT '0'    NOT NULL,
  `updated_at`   bigint           DEFAULT '0'    NOT NULL,
  `creator`      varchar(64)      DEFAULT ''    NOT NULL,
  `updater`      varchar(64)      DEFAULT ''    NOT NULL,
  PRIMARY KEY (`id`),
  KEY (`start_ip_int`, `end_ip_int`),
  KEY (`start_time`, `end_time`),
  KEY (`created_at`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8;

CREATE TABLE `session` (
   `sid`         char(128) NOT NULL,
   `data`        blob NULL,
   `user_name`   varchar(64) DEFAULT '',
   `cookie_name` char(128) DEFAULT '',
   `created_at`  integer unsigned DEFAULT '0',
   `updated_at`  integer unsigned DEFAULT '0' NOT NULL,
   PRIMARY KEY (`sid`),
   KEY (`user_name`),
   KEY (`cookie_name`),
   KEY (`updated_at`)
) ENGINE = InnoDB DEFAULT CHARACTER SET = utf8;

alter table user add `login_err_num` int unsigned not null default 0 after leader_name;

alter table user add `active_begin`  bigint       not null default 0 after login_err_num;
alter table user add `active_end`    bigint       not null default 0 after active_begin;
alter table user add `locked_at`     bigint       not null default 0 after active_end;
alter table user add `updated_at`    bigint       not null default 0 after locked_at;
alter table user add `pwd_updated_at`    bigint       not null default 0 after updated_at;
