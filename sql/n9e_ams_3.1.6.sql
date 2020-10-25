set names utf8;
use n9e_ams;

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
