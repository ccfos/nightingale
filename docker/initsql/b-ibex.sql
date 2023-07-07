set names utf8mb4;

drop database if exists ibex;
create database ibex;
use ibex;

CREATE TABLE `task_meta`
(
    `id`          bigint unsigned NOT NULL AUTO_INCREMENT,
    `title`       varchar(255)    not null default '',
    `account`     varchar(64)     not null,
    `batch`       int unsigned    not null default 0,
    `tolerance`   int unsigned    not null default 0,
    `timeout`     int unsigned    not null default 0,
    `pause`       varchar(255)    not null default '',
    `script`      text            not null,
    `args`        varchar(512)    not null default '',
    `stdin`       varchar(1024)   not null default '',
    `creator`     varchar(64)     not null default '',
    `created`     timestamp       not null default CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    KEY (`creator`),
    KEY (`created`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

/* start|cancel|kill|pause */
CREATE TABLE `task_action`
(
    `id`     bigint unsigned not null,
    `action` varchar(32)     not null,
    `clock`  bigint          not null default 0,
    PRIMARY KEY (`id`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE `task_scheduler`
(
    `id`        bigint unsigned not null,
    `scheduler` varchar(128)    not null default '',
    KEY (`id`, `scheduler`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE `task_scheduler_health`
(
    `scheduler` varchar(128) not null,
    `clock`     bigint       not null,
    UNIQUE KEY (`scheduler`),
    KEY (`clock`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE `task_host_doing`
(
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `clock`  bigint          not null default 0,
    `action` varchar(16)     not null,
    KEY (`id`),
    KEY (`host`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_0
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_1
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_2
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_3
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_4
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_5
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_6
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_7
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_8
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_9
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_10
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_11
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_12
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_13
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_14
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_15
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_16
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_17
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_18
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_19
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_20
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_21
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_22
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_23
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_24
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_25
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_26
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_27
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_28
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_29
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_30
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_31
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_32
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_33
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_34
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_35
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_36
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_37
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_38
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_39
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_40
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_41
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_42
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_43
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_44
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_45
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_46
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_47
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_48
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_49
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_50
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_51
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_52
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_53
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_54
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_55
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_56
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_57
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_58
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_59
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_60
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_61
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_62
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_63
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_64
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_65
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_66
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_67
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_68
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_69
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_70
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_71
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_72
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_73
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_74
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_75
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_76
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_77
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_78
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_79
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_80
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_81
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_82
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_83
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_84
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_85
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_86
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_87
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_88
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_89
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_90
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_91
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_92
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_93
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_94
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_95
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_96
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_97
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_98
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE task_host_99
(
    `ii`     bigint unsigned NOT NULL AUTO_INCREMENT,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    UNIQUE KEY (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;
