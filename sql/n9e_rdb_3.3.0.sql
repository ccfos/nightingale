set names utf8;
use n9e_rdb;

CREATE TABLE `login_code`
(
    `username`   varchar(64)  not null comment 'login name, cannot rename',
    `code`       varchar(32)  not null,
    `login_type` varchar(32)  not null,
    `created_at` bigint       not null comment 'created at',
    KEY (`code`),
    KEY (`created_at`),
    UNIQUE KEY (`username`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8;

CREATE TABLE `auth_state` (
  `state`        varchar(128)       DEFAULT ''    NOT NULL,
  `typ`          varchar(32)        DEFAULT ''    NOT NULL COMMENT 'response_type',
  `redirect`     varchar(1024)      DEFAULT ''    NOT NULL,
  `expires_at`   bigint             DEFAULT '0'   NOT NULL,
  PRIMARY KEY (`state`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8;

CREATE TABLE `captcha` (
  `captcha_id`   varchar(128)     NOT NULL,
  `answer`       varchar(128)     DEFAULT ''    NOT NULL,
  `created_at`   bigint           DEFAULT '0'    NOT NULL,
  KEY (`captcha_id`, `answer`),
  KEY (`created_at`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8;

alter table user add column create_at timestamp not null default CURRENT_TIMESTAMP;
update user set create_at = '2020-11-14 17:00:08';

alter table user add `organization` varchar(255) not null default '' after intro;
alter table user add `typ` tinyint(1) not null default 0 comment '0: long-term account; 1: temporary account' after intro;
alter table user add `status` tinyint(1) not null default 0 comment '0: active; 1: inactive 2: disable' after intro;
