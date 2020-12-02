set names utf8;

drop database if exists n9e_ams;
create database n9e_ams;
use n9e_ams;

CREATE TABLE `host`
(
    `id`     int unsigned not null AUTO_INCREMENT,
    `sn`     char(128)    not null default '',
    `ip`     char(15)     not null,
    `ident`  varchar(128) not null default '',
    `name`   varchar(128) not null default '',
    `cpu`    varchar(255) not null default '',
    `mem`    varchar(255) not null default '',
    `disk`   varchar(255) not null default '',
    `note`   varchar(255) not null default 'different with resource note',
    `cate`   varchar(32)  not null comment 'host,vm,container,switch',
    `tenant` varchar(128) not null default '',
    `clock`  bigint       not null comment 'heartbeat timestamp',
    PRIMARY KEY (`id`),
    UNIQUE KEY (`ip`),
    UNIQUE KEY (`ident`),
    KEY (`sn`),
    KEY (`name`),
    KEY (`tenant`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8;

CREATE TABLE `host_field`
(
    `id`             int unsigned  not null AUTO_INCREMENT,
    `field_ident`    varchar(255)  not null comment 'english identity',
    `field_name`     varchar(255)  not null comment 'chinese name',
    `field_type`     varchar(64)   not null,
    `field_required` tinyint(1)    not null default 0,
    `field_extra`    varchar(2048) not null default '',
    `field_cate`     varchar(255)  not null default 'Default',
    PRIMARY KEY (`id`),
    KEY (`field_cate`, `field_ident`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8;

CREATE TABLE `host_field_value`
(
    `id`          int unsigned  not null AUTO_INCREMENT,
    `host_id`     int unsigned  not null,
    `field_ident` varchar(255)  not null,
    `field_value` varchar(1024) not null default '',
    PRIMARY KEY (`id`),
    KEY (`host_id`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8;

/* 网络设备管理、机柜机架、配件耗材等相关的功能是商业版本才有的，表结构不要放到这里 */
