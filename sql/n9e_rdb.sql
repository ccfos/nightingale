set names utf8;

drop database if exists n9e_rdb;
create database n9e_rdb;
use n9e_rdb;

CREATE TABLE `user`
(
    `id`           int unsigned not null AUTO_INCREMENT,
    `uuid`         varchar(128) not null comment 'use in cookie',
    `username`     varchar(64)  not null comment 'login name, cannot rename',
    `password`     varchar(128) not null default '',
    `dispname`     varchar(32)  not null default '' comment 'display name, chinese name',
    `phone`        varchar(16)  not null default '',
    `email`        varchar(64)  not null default '',
    `im`           varchar(64)  not null default '',
    `portrait`     varchar(2048) not null default '',
    `intro`        varchar(2048) not null default '',
    `organization` varchar(255) not null default '',
    `typ`          tinyint(1)   not null default 0 comment '0: long-term account; 1: temporary account',
    `status`       tinyint(1)   not null default 0 comment '0: active; 1: inactive 2: disable',
    `is_root`      tinyint(1)   not null,
    `leader_id`    int unsigned not null default 0,
    `leader_name`  varchar(32)  not null default '',
    `create_at`    timestamp    not null default CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY (`username`),
    UNIQUE KEY (`uuid`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8;

CREATE TABLE `user_token`
(
    `user_id`  int unsigned not null,
    `username` varchar(128) not null,
    `token`    varchar(128) not null,
    KEY (`user_id`),
    KEY (`username`),
    UNIQUE KEY (`token`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8;

CREATE TABLE `invite`
(
    `id`      int unsigned not null AUTO_INCREMENT,
    `token`   varchar(128) not null,
    `expire`  bigint       not null,
    `creator` varchar(32)  not null,
    PRIMARY KEY (`id`),
    UNIQUE KEY (`token`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8;

CREATE TABLE `team`
(
    `id`           int unsigned not null AUTO_INCREMENT,
    `ident`        varchar(255) not null,
    `name`         varchar(255) not null default '',
    `note`         varchar(255) not null default '',
    `mgmt`         int(1)       not null comment '0: member manage; 1: admin manage',
    `creator`      int unsigned not null,
    `last_updated` timestamp    not null default CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY (`ident`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8;

CREATE TABLE `team_user`
(
    `team_id`  int unsigned not null,
    `user_id`  int unsigned not null,
    `is_admin` tinyint(1)   not null,
    KEY (`team_id`),
    KEY (`user_id`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8;

CREATE TABLE `configs`
(
    `id`           int unsigned not null AUTO_INCREMENT,
    `ckey`         varchar(255) not null,
    `cval`         varchar(255) not null default '',
    `last_updated` timestamp    not null default CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY (`ckey`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8;

CREATE TABLE `node_cate`
(
    `id`         int unsigned not null AUTO_INCREMENT,
    `ident`      char(128)    not null default '' comment 'cluster,service,module,department,product...',
    `name`       varchar(255) not null default '',
    `icon_color` char(7)      not null default '' comment 'e.g. #108AC6',
    `protected`  tinyint(1)   not null default 0 comment 'if =1, cannot delete',
    PRIMARY KEY (`id`),
    KEY (`ident`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8;

insert into node_cate(ident, name, icon_color, protected)
values ('tenant', '租户', '#de83cb', 1);
insert into node_cate(ident, name, icon_color, protected)
values ('organization', '组织', '#ff8e75', 1);
insert into node_cate(ident, name, icon_color, protected)
values ('project', '项目', '#f6bb4a', 1);
insert into node_cate(ident, name, icon_color, protected)
values ('module', '模块', '#6dc448', 1);
insert into node_cate(ident, name, icon_color, protected)
values ('cluster', '集群', '#94c7c6', 1);
insert into node_cate(ident, name, icon_color, protected)
values ('resource', '资源', '#a7aae6', 1);

CREATE TABLE `node_cate_field`
(
    `id`             int unsigned  not null AUTO_INCREMENT,
    `cate`           char(32)      not null default '' comment 'cluster,service,module,department,product...',
    `field_ident`    varchar(255)  not null comment 'english identity',
    `field_name`     varchar(255)  not null comment 'chinese name',
    `field_type`     varchar(64)   not null,
    `field_required` tinyint(1)    not null default 0,
    `field_extra`    varchar(2048) not null default '',
    `last_updated`   timestamp     not null default CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    KEY (`cate`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8;

CREATE TABLE `node_field_value`
(
    `id`          int unsigned  not null AUTO_INCREMENT,
    `node_id`     int unsigned  not null,
    `field_ident` varchar(255)  not null,
    `field_value` varchar(1024) not null default '',
    PRIMARY KEY (`id`),
    KEY (`node_id`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8;

CREATE TABLE `node`
(
    `id`           int unsigned not null AUTO_INCREMENT,
    `pid`          int unsigned not null,
    `ident`        varchar(128) not null,
    `name`         varchar(255) not null default '',
    `note`         varchar(255) not null default '',
    `path`         varchar(255) not null comment 'ident1.ident2.ident3',
    `leaf`         tinyint(1)   not null,
    `cate`         char(128)    not null default '' comment 'cluster,service,module,department,product...',
    `icon_color`   char(7)      not null default '' comment 'e.g. #108AC6',
    `icon_char`    char(1)      not null default '' comment 'cluster->C,service->S,module->M',
    `proxy`        tinyint(1)   not null default 0 comment '0:myself management, 1:other module management',
    `creator`      varchar(64)  not null,
    `last_updated` timestamp    not null default CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY (`path`),
    KEY (`cate`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8;

insert into node(id, ident, name, note, pid, path, leaf, cate, icon_color, icon_char, proxy, creator)
values (1, 'inner', '内置租户', '用于平台管理视角的资源监控', 0, 'inner', 0, 'tenant', '#de83cb', 'T', 0, 'root');

CREATE TABLE `node_trash`
(
    `id`           int unsigned not null,
    `pid`          int unsigned not null,
    `ident`        varchar(128) not null,
    `name`         varchar(255) not null default '',
    `note`         varchar(255) not null default '',
    `path`         varchar(255) not null comment 'ident1.ident2.ident3',
    `leaf`         tinyint(1)   not null,
    `cate`         char(128)    not null default '' comment 'cluster,service,module,department,product...',
    `icon_color`   char(7)      not null default '' comment 'e.g. #108AC6',
    `icon_char`    char(1)      not null default '' comment 'cluster->C,service->S,module->M',
    `proxy`        tinyint(1)   not null default 0 comment '0:myself management, 1:other module management',
    `creator`      varchar(64)  not null,
    `last_updated` timestamp    not null default CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    KEY (`path`),
    KEY (`cate`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8;

CREATE TABLE `node_admin`
(
    `node_id` int unsigned not null,
    `user_id` int unsigned not null,
    KEY (`node_id`),
    KEY (`user_id`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8;

CREATE TABLE `resource`
(
    `id`           int unsigned  not null AUTO_INCREMENT,
    `uuid`         varchar(255)  not null,
    `ident`        varchar(255)  not null,
    `name`         varchar(255)  not null default '',
    `labels`       varchar(255)  not null default '' comment 'e.g. flavor=2c4g300g,region=bj,os=windows',
    `note`         varchar(255)  not null default '',
    `extend`       varchar(1024) not null default '' comment 'json',
    `cate`         varchar(64)   not null comment 'host,vm,container,switch,redis,mongo',
    `tenant`       varchar(128)  not null default '',
    `last_updated` timestamp     not null default CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY (`uuid`),
    UNIQUE KEY (`ident`),
    KEY (`tenant`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8;

CREATE TABLE `node_resource`
(
    `node_id` int unsigned not null,
    `res_id`  int unsigned not null,
    KEY (`node_id`),
    KEY (`res_id`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8;

CREATE TABLE `node_role`
(
    `id`       int unsigned not null AUTO_INCREMENT,
    `node_id`  int unsigned not null,
    `username` varchar(64)  not null,
    `role_id`  int unsigned not null,
    PRIMARY KEY (`id`),
    KEY (`node_id`),
    KEY (`role_id`),
    KEY (`username`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8;

CREATE TABLE `role`
(
    `id`   int unsigned not null AUTO_INCREMENT,
    `name` varchar(128) not null default '',
    `note` varchar(255) not null default '',
    `cate` char(6)      not null default '' comment 'category: global or local',
    PRIMARY KEY (`id`),
    UNIQUE KEY (`name`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8;

CREATE TABLE `role_operation`
(
    `id`        int unsigned not null AUTO_INCREMENT,
    `role_id`   int unsigned not null,
    `operation` varchar(255) not null,
    PRIMARY KEY (`id`),
    KEY (`role_id`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8;

CREATE TABLE `role_global_user`
(
    `role_id` int unsigned not null,
    `user_id` int unsigned not null,
    KEY (`role_id`),
    KEY (`user_id`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8;

CREATE TABLE `login_log`
(
    `id`       int unsigned not null AUTO_INCREMENT,
    `username` varchar(64)  not null,
    `client`   varchar(128) not null comment 'client ip',
    `clock`    bigint       not null comment 'login timestamp',
    `loginout` char(3)      not null comment 'in or out',
    PRIMARY KEY (`id`),
    KEY (`username`),
    KEY (`clock`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8;

CREATE TABLE `operation_log`
(
    `id`       bigint unsigned not null AUTO_INCREMENT,
    `username` varchar(64)     not null,
    `clock`    bigint          not null comment 'operation timestamp',
    `res_cl`   char(16)        not null default '' comment 'resource class',
    `res_id`   varchar(128)    not null default '',
    `detail`   varchar(512)    not null,
    PRIMARY KEY (`id`),
    KEY (`clock`),
    KEY (`res_cl`, `res_id`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8;

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
