set names utf8mb4;

-- drop database if exists n9e_v6;
create database n9e_v6;
use n9e_v6;

CREATE TABLE `users` (
    `id` bigint unsigned not null auto_increment,
    `username` varchar(64) not null comment 'login name, cannot rename',
    `nickname` varchar(64) not null comment 'display name, chinese name',
    `password` varchar(128) not null default '',
    `phone` varchar(16) not null default '',
    `email` varchar(64) not null default '',
    `portrait` varchar(255) not null default '' comment 'portrait image url',
    `roles` varchar(255) not null comment 'Admin | Standard | Guest, split by space',
    `contacts` varchar(1024) comment 'json e.g. {wecom:xx, dingtalk_robot_token:yy}',
    `maintainer` tinyint(1) not null default 0,
    `belong` varchar(191) DEFAULT '' COMMENT 'belong',
    `last_active_time` bigint DEFAULT 0 COMMENT 'last_active_time',
    `create_at` bigint not null default 0,
    `create_by` varchar(64) not null default '',
    `update_at` bigint not null default 0,
    `update_by` varchar(64) not null default '',
    PRIMARY KEY (`id`),
    UNIQUE KEY (`username`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

insert into `users`(id, username, nickname, password, roles, create_at, create_by, update_at, update_by) values(1, 'root', '超管', 'root.2020', 'Admin', unix_timestamp(now()), 'system', unix_timestamp(now()), 'system');

CREATE TABLE `user_group` (
    `id` bigint unsigned not null auto_increment,
    `name` varchar(128) not null default '',
    `note` varchar(255) not null default '',
    `create_at` bigint not null default 0,
    `create_by` varchar(64) not null default '',
    `update_at` bigint not null default 0,
    `update_by` varchar(64) not null default '',
    PRIMARY KEY (`id`),
    KEY (`create_by`),
    KEY (`update_at`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

insert into user_group(id, name, create_at, create_by, update_at, update_by) values(1, 'demo-root-group', unix_timestamp(now()), 'root', unix_timestamp(now()), 'root');

CREATE TABLE `user_group_member` (
    `id` bigint unsigned not null auto_increment,
    `group_id` bigint unsigned not null,
    `user_id` bigint unsigned not null,
    KEY (`group_id`),
    KEY (`user_id`),
    PRIMARY KEY(`id`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

insert into user_group_member(group_id, user_id) values(1, 1);

CREATE TABLE `configs` (
    `id` bigint unsigned not null auto_increment,
    `ckey` varchar(191) not null,
    `note` varchar(1024) NOT NULL DEFAULT '' COMMENT 'note',
    `cval` text COMMENT 'config value',
    `external`  bigint DEFAULT 0 COMMENT '0\\:built-in 1\\:external',
    `encrypted` bigint DEFAULT 0 COMMENT '0\\:plaintext 1\\:ciphertext',
    `create_at` bigint DEFAULT 0 COMMENT 'create_at',
    `create_by` varchar(64) NOT NULL DEFAULT '' COMMENT 'cerate_by',
    `update_at` bigint DEFAULT 0 COMMENT 'update_at',
    `update_by` varchar(64) NOT NULL DEFAULT '' COMMENT 'update_by',
    PRIMARY KEY (`id`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

CREATE TABLE `role` (
    `id` bigint unsigned not null auto_increment,
    `name` varchar(191) not null default '',
    `note` varchar(255) not null default '',
    PRIMARY KEY (`id`),
    UNIQUE KEY (`name`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

insert into `role`(name, note) values('Admin', 'Administrator role');
insert into `role`(name, note) values('Standard', 'Ordinary user role');
insert into `role`(name, note) values('Guest', 'Readonly user role');

CREATE TABLE `role_operation`(
    `id` bigint unsigned not null auto_increment,
    `role_name` varchar(128) not null,
    `operation` varchar(191) not null,
    KEY (`role_name`),
    KEY (`operation`),
    PRIMARY KEY(`id`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

-- Admin is special, who has no concrete operation but can do anything.
insert into `role_operation`(role_name, operation) values('Guest', '/metric/explorer');
insert into `role_operation`(role_name, operation) values('Guest', '/object/explorer');
insert into `role_operation`(role_name, operation) values('Guest', '/log/explorer');
insert into `role_operation`(role_name, operation) values('Guest', '/trace/explorer');
insert into `role_operation`(role_name, operation) values('Guest', '/help/version');
insert into `role_operation`(role_name, operation) values('Guest', '/help/contact');

insert into `role_operation`(role_name, operation) values('Standard', '/metric/explorer');
insert into `role_operation`(role_name, operation) values('Standard', '/object/explorer');
insert into `role_operation`(role_name, operation) values('Standard', '/log/explorer');
insert into `role_operation`(role_name, operation) values('Standard', '/trace/explorer');
insert into `role_operation`(role_name, operation) values('Standard', '/help/version');
insert into `role_operation`(role_name, operation) values('Standard', '/help/contact');
insert into `role_operation`(role_name, operation) values('Standard', '/help/servers');
insert into `role_operation`(role_name, operation) values('Standard', '/help/migrate');
insert into `role_operation`(role_name, operation) values('Standard', '/alert-rules-built-in');
insert into `role_operation`(role_name, operation) values('Standard', '/dashboards-built-in');
insert into `role_operation`(role_name, operation) values('Standard', '/trace/dependencies');
insert into `role_operation`(role_name, operation) values('Standard', '/users');
insert into `role_operation`(role_name, operation) values('Standard', '/user-groups');
insert into `role_operation`(role_name, operation) values('Standard', '/user-groups/add');
insert into `role_operation`(role_name, operation) values('Standard', '/user-groups/put');
insert into `role_operation`(role_name, operation) values('Standard', '/user-groups/del');
insert into `role_operation`(role_name, operation) values('Standard', '/busi-groups');
insert into `role_operation`(role_name, operation) values('Standard', '/busi-groups/add');
insert into `role_operation`(role_name, operation) values('Standard', '/busi-groups/put');
insert into `role_operation`(role_name, operation) values('Standard', '/busi-groups/del');
insert into `role_operation`(role_name, operation) values('Standard', '/targets');
insert into `role_operation`(role_name, operation) values('Standard', '/targets/add');
insert into `role_operation`(role_name, operation) values('Standard', '/targets/put');
insert into `role_operation`(role_name, operation) values('Standard', '/targets/del');
insert into `role_operation`(role_name, operation) values('Standard', '/dashboards');
insert into `role_operation`(role_name, operation) values('Standard', '/dashboards/add');
insert into `role_operation`(role_name, operation) values('Standard', '/dashboards/put');
insert into `role_operation`(role_name, operation) values('Standard', '/dashboards/del');
insert into `role_operation`(role_name, operation) values('Standard', '/alert-rules');
insert into `role_operation`(role_name, operation) values('Standard', '/alert-rules/add');
insert into `role_operation`(role_name, operation) values('Standard', '/alert-rules/put');
insert into `role_operation`(role_name, operation) values('Standard', '/alert-rules/del');
insert into `role_operation`(role_name, operation) values('Standard', '/alert-mutes');
insert into `role_operation`(role_name, operation) values('Standard', '/alert-mutes/add');
insert into `role_operation`(role_name, operation) values('Standard', '/alert-mutes/del');
insert into `role_operation`(role_name, operation) values('Standard', '/alert-subscribes');
insert into `role_operation`(role_name, operation) values('Standard', '/alert-subscribes/add');
insert into `role_operation`(role_name, operation) values('Standard', '/alert-subscribes/put');
insert into `role_operation`(role_name, operation) values('Standard', '/alert-subscribes/del');
insert into `role_operation`(role_name, operation) values('Standard', '/alert-cur-events');
insert into `role_operation`(role_name, operation) values('Standard', '/alert-cur-events/del');
insert into `role_operation`(role_name, operation) values('Standard', '/alert-his-events');
insert into `role_operation`(role_name, operation) values('Standard', '/job-tpls');
insert into `role_operation`(role_name, operation) values('Standard', '/job-tpls/add');
insert into `role_operation`(role_name, operation) values('Standard', '/job-tpls/put');
insert into `role_operation`(role_name, operation) values('Standard', '/job-tpls/del');
insert into `role_operation`(role_name, operation) values('Standard', '/job-tasks');
insert into `role_operation`(role_name, operation) values('Standard', '/job-tasks/add');
insert into `role_operation`(role_name, operation) values('Standard', '/job-tasks/put');
insert into `role_operation`(role_name, operation) values('Standard', '/recording-rules');
insert into `role_operation`(role_name, operation) values('Standard', '/recording-rules/add');
insert into `role_operation`(role_name, operation) values('Standard', '/recording-rules/put');
insert into `role_operation`(role_name, operation) values('Standard', '/recording-rules/del');

-- for alert_rule | collect_rule | mute | dashboard grouping
CREATE TABLE `busi_group` (
    `id` bigint unsigned not null auto_increment,
    `name` varchar(191) not null,
    `label_enable` tinyint(1) not null default 0,
    `label_value` varchar(191) not null default '' comment 'if label_enable: label_value can not be blank',
    `create_at` bigint not null default 0,
    `create_by` varchar(64) not null default '',
    `update_at` bigint not null default 0,
    `update_by` varchar(64) not null default '',
    PRIMARY KEY (`id`),
    UNIQUE KEY (`name`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

insert into busi_group(id, name, create_at, create_by, update_at, update_by) values(1, 'Default Busi Group', unix_timestamp(now()), 'root', unix_timestamp(now()), 'root');

CREATE TABLE `busi_group_member` (
    `id` bigint unsigned not null auto_increment,
    `busi_group_id` bigint not null comment 'busi group id',
    `user_group_id` bigint not null comment 'user group id',
    `perm_flag` char(2) not null comment 'ro | rw',
    PRIMARY KEY (`id`),
    KEY (`busi_group_id`),
    KEY (`user_group_id`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

insert into busi_group_member(busi_group_id, user_group_id, perm_flag) values(1, 1, 'rw');

-- for dashboard new version
CREATE TABLE `board` (
    `id` bigint unsigned not null auto_increment,
    `group_id` bigint not null default 0 comment 'busi group id',
    `name` varchar(191) not null,
    `ident` varchar(200) not null default '',
    `tags` varchar(255) not null comment 'split by space',
    `public` tinyint(1) not null default 0 comment '0:false 1:true',
    `built_in` tinyint(1) not null default 0 comment '0:false 1:true',
    `hide` tinyint(1) not null default 0 comment '0:false 1:true',
    `create_at` bigint not null default 0,
    `create_by` varchar(64) not null default '',
    `update_at` bigint not null default 0,
    `update_by` varchar(64) not null default '',
    `public_cate` bigint NOT NULL NOT NULL DEFAULT 0 COMMENT '0 anonymous 1 login 2 busi',
    PRIMARY KEY (`id`),
    UNIQUE KEY (`group_id`, `name`),
    KEY(`ident`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

-- for dashboard new version
CREATE TABLE `board_payload` (
    `id` bigint unsigned not null comment 'dashboard id',
    `payload` mediumtext not null,
    UNIQUE KEY (`id`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

-- deprecated
CREATE TABLE `dashboard` (
    `id` bigint unsigned not null auto_increment,
    `group_id` bigint not null default 0 comment 'busi group id',
    `name` varchar(191) not null,
    `tags` varchar(255) not null comment 'split by space',
    `configs` varchar(8192) comment 'dashboard variables',
    `create_at` bigint not null default 0,
    `create_by` varchar(64) not null default '',
    `update_at` bigint not null default 0,
    `update_by` varchar(64) not null default '',
    PRIMARY KEY (`id`),
    UNIQUE KEY (`group_id`, `name`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

-- deprecated
-- auto create the first subclass 'Default chart group' of dashboard
CREATE TABLE `chart_group` (
    `id` bigint unsigned not null auto_increment,
    `dashboard_id` bigint unsigned not null,
    `name` varchar(255) not null,
    `weight` int not null default 0,
    PRIMARY KEY (`id`),
    KEY (`dashboard_id`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

-- deprecated
CREATE TABLE `chart` (
    `id` bigint unsigned not null auto_increment,
    `group_id` bigint unsigned not null comment 'chart group id',
    `configs` text,
    `weight` int not null default 0,
    PRIMARY KEY (`id`),
    KEY (`group_id`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

CREATE TABLE `chart_share` (
    `id` bigint unsigned not null auto_increment,
    `cluster` varchar(128) not null,
    `datasource_id` bigint NOT NULL NOT NULL DEFAULT 0 COMMENT 'datasource id',
    `configs` text,
    `create_at` bigint not null default 0,
    `create_by` varchar(64) not null default '',
    primary key (`id`),
    key (`create_at`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

CREATE TABLE `alert_rule` (
    `id` bigint unsigned not null auto_increment,
    `group_id` bigint not null default 0 comment 'busi group id',
    `cate` varchar(128) not null,
    `datasource_ids` varchar(255) not null default '' comment 'datasource ids',
    `cluster` varchar(128) not null,
    `name` varchar(255) not null,
    `note` varchar(1024) not null default '',
    `prod` varchar(255) not null default '',
    `algorithm` varchar(255) not null default '',
    `algo_params` varchar(255),
    `delay` int not null default 0,
    `severity` tinyint(1) not null comment '1:Emergency 2:Warning 3:Notice',
    `disabled` tinyint(1) not null comment '0:enabled 1:disabled',
    `prom_for_duration` int not null comment 'prometheus for, unit:s',
    `rule_config` text not null comment 'rule_config',
    `prom_ql` text not null comment 'promql',
    `prom_eval_interval` int not null comment 'evaluate interval',
    `enable_stime` varchar(255) not null default '00:00',
    `enable_etime` varchar(255) not null default '23:59',
    `enable_days_of_week` varchar(255) not null default '' comment 'split by space: 0 1 2 3 4 5 6',
    `enable_in_bg` tinyint(1) not null default 0 comment '1: only this bg 0: global',
    `notify_recovered` tinyint(1) not null comment 'whether notify when recovery',
    `notify_channels` varchar(255) not null default '' comment 'split by space: sms voice email dingtalk wecom',
    `notify_groups` varchar(255) not null default '' comment 'split by space: 233 43',
    `notify_repeat_step` int not null default 0 comment 'unit: min',
    `notify_max_number` int not null default 0 comment '',
    `recover_duration` int not null default 0 comment 'unit: s',
    `callbacks` varchar(4096) not null default '' comment 'split by space: http://a.com/api/x http://a.com/api/y',
    `runbook_url` varchar(4096),
    `append_tags` varchar(255) not null default '' comment 'split by space: service=n9e mod=api',
    `annotations` text not null comment 'annotations',
    `extra_config` text,
    `notify_rule_ids` varchar(1024) DEFAULT '',
    `notify_version` int DEFAULT 0,
    `create_at` bigint not null default 0,
    `create_by` varchar(64) not null default '',
    `update_at` bigint not null default 0,
    `update_by` varchar(64) not null default '',
    `cron_pattern` varchar(64),
    `datasource_queries` text,
    PRIMARY KEY (`id`),
    KEY (`group_id`),
    KEY (`update_at`)
) ENGINE=InnoDB DEFAULT CHARSET = utf8mb4;

CREATE TABLE `alert_mute` (
    `id` bigint unsigned not null auto_increment,
    `group_id` bigint not null default 0 comment 'busi group id',
    `prod` varchar(255) not null default '',
    `note` varchar(1024) not null default '',
    `cate` varchar(128) not null,
    `cluster` varchar(128) not null,
    `datasource_ids` varchar(255) not null default '' comment 'datasource ids',
    `tags` varchar(4096) default '[]' comment 'json,map,tagkey->regexp|value',
    `cause` varchar(255) not null default '',
    `btime` bigint not null default 0 comment 'begin time',
    `etime` bigint not null default 0 comment 'end time',
    `disabled` tinyint(1) not null default 0 comment '0:enabled 1:disabled',
    `mute_time_type` tinyint(1) not null default 0,
    `periodic_mutes` varchar(4096) not null default '',
    `severities` varchar(32) not null default '',
    `create_at` bigint not null default 0,
    `create_by` varchar(64) not null default '',
    `update_at` bigint not null default 0,
    `update_by` varchar(64) not null default '',
    PRIMARY KEY (`id`),
    KEY (`create_at`),
    KEY (`group_id`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

CREATE TABLE `alert_subscribe` (
    `id` bigint unsigned not null auto_increment,
    `name` varchar(255) not null default '',
    `disabled` tinyint(1) not null default 0 comment '0:enabled 1:disabled',
    `group_id` bigint not null default 0 comment 'busi group id',
    `prod` varchar(255) not null default '',
    `cate` varchar(128) not null,
    `datasource_ids` varchar(255) not null default '' comment 'datasource ids',
    `cluster` varchar(128) not null,
    `rule_id` bigint not null default 0,
    `rule_ids` varchar(1024),
    `severities` varchar(32) not null default '',
    `tags` varchar(4096) not null default '' comment 'json,map,tagkey->regexp|value',
    `redefine_severity` tinyint(1) default 0 comment 'is redefine severity?',
    `new_severity` tinyint(1) not null comment '0:Emergency 1:Warning 2:Notice',
    `redefine_channels` tinyint(1) default 0 comment 'is redefine channels?',
    `new_channels` varchar(255) not null default '' comment 'split by space: sms voice email dingtalk wecom',
    `user_group_ids` varchar(250) not null comment 'split by space 1 34 5, notify cc to user_group_ids',
    `busi_groups` varchar(4096),
    `note` VARCHAR(1024) DEFAULT '' COMMENT 'note',
    `webhooks` text not null,
    `extra_config` text,
    `redefine_webhooks` tinyint(1) default 0,
    `for_duration` bigint not null default 0,
    `notify_rule_ids` varchar(1024) DEFAULT '',
    `notify_version` int DEFAULT 0,
    `create_at` bigint not null default 0,
    `create_by` varchar(64) not null default '',
    `update_at` bigint not null default 0,
    `update_by` varchar(64) not null default '',
    PRIMARY KEY (`id`),
    KEY (`update_at`),
    KEY (`group_id`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

CREATE TABLE `target` (
    `id` bigint unsigned not null auto_increment,
    `group_id` bigint not null default 0 comment 'busi group id',
    `ident` varchar(191) not null comment 'target id',
    `note` varchar(255) not null default '' comment 'append to alert event as field',
    `tags` varchar(512) not null default '' comment 'append to series data as tags, split by space, append external space at suffix',
    `host_tags` text COMMENT 'global labels set in conf file',
    `host_ip` varchar(15) default '' COMMENT 'IPv4 string',
    `agent_version` varchar(255) default '' COMMENT 'agent version',
    `engine_name` varchar(255) DEFAULT '' COMMENT 'engine name',
    `os` VARCHAR(31) DEFAULT '' COMMENT 'os type',
    `update_at` bigint not null default 0,
    PRIMARY KEY (`id`),
    UNIQUE KEY (`ident`),
    KEY (`group_id`),
    INDEX `idx_host_ip` (`host_ip`),
    INDEX `idx_agent_version` (`agent_version`),
    INDEX `idx_engine_name` (`engine_name`),
    INDEX `idx_os` (`os`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;


CREATE TABLE `metric_view` (
    `id` bigint unsigned not null auto_increment,
    `name` varchar(191) not null default '',
    `cate` tinyint(1) not null comment '0: preset 1: custom',
    `configs` varchar(8192) not null default '',
    `create_at` bigint not null default 0,
    `create_by` bigint not null default 0 comment 'user id',
    `update_at` bigint not null default 0,
    PRIMARY KEY (`id`),
    KEY (`create_by`)
) ENGINE=InnoDB DEFAULT CHARSET = utf8mb4;

insert into metric_view(name, cate, configs) values('Host View', 0, '{"filters":[{"oper":"=","label":"__name__","value":"cpu_usage_idle"}],"dynamicLabels":[],"dimensionLabels":[{"label":"ident","value":""}]}');

CREATE TABLE `recording_rule` (
    `id` bigint unsigned not null auto_increment,
    `group_id` bigint not null default '0' comment 'group_id',
    `datasource_ids` varchar(255) not null default '' comment 'datasource ids',
    `cluster` varchar(128) not null,
    `name` varchar(255) not null comment 'new metric name',
    `note` varchar(255) not null comment 'rule note',
    `disabled` tinyint(1) not null default 0 comment '0:enabled 1:disabled',
    `prom_ql` varchar(8192) not null comment 'promql',
    `prom_eval_interval` int not null comment 'evaluate interval',
    `cron_pattern` varchar(255) default '' comment 'cron pattern',
    `append_tags` varchar(255) default '' comment 'split by space: service=n9e mod=api',
    `query_configs` text NOT NULL,
    `create_at` bigint default '0',
    `create_by` varchar(64) default '',
    `update_at` bigint default '0',
    `update_by` varchar(64) default '',
    `datasource_queries` text,
    PRIMARY KEY (`id`),
    KEY `group_id` (`group_id`),
    KEY `update_at` (`update_at`)
) ENGINE=InnoDB DEFAULT CHARSET = utf8mb4;

CREATE TABLE `alert_aggr_view` (
    `id` bigint unsigned not null auto_increment,
    `name` varchar(191) not null default '',
    `rule` varchar(2048) not null default '',
    `cate` tinyint(1) not null comment '0: preset 1: custom',
    `create_at` bigint not null default 0,
    `create_by` bigint not null default 0 comment 'user id',
    `update_at` bigint not null default 0,
    PRIMARY KEY (`id`),
    KEY (`create_by`)
) ENGINE=InnoDB DEFAULT CHARSET = utf8mb4;

insert into alert_aggr_view(name, rule, cate) values('By BusiGroup, Severity', 'field:group_name::field:severity', 0);
insert into alert_aggr_view(name, rule, cate) values('By RuleName', 'field:rule_name', 0);

CREATE TABLE `alert_cur_event` (
    `id` bigint unsigned not null comment 'use alert_his_event.id',
    `cate` varchar(128) not null,
    `datasource_id` bigint not null default 0 comment 'datasource id',
    `cluster` varchar(128) not null,
    `group_id` bigint unsigned not null comment 'busi group id of rule',
    `group_name` varchar(255) not null default '' comment 'busi group name',
    `hash` varchar(64) not null comment 'rule_id + vector_pk',
    `rule_id` bigint unsigned not null,
    `rule_name` varchar(255) not null,
    `rule_note` varchar(2048) not null default 'alert rule note',
    `rule_prod` varchar(255) not null default '',
    `rule_algo` varchar(255) not null default '',
    `severity` tinyint(1) not null comment '0:Emergency 1:Warning 2:Notice',
    `prom_for_duration` int not null comment 'prometheus for, unit:s',
    `prom_ql` varchar(8192) not null comment 'promql',
    `prom_eval_interval` int not null comment 'evaluate interval',
    `callbacks` varchar(2048) not null default '' comment 'split by space: http://a.com/api/x http://a.com/api/y',
    `runbook_url` varchar(255),
    `notify_recovered` tinyint(1) not null comment 'whether notify when recovery',
    `notify_channels` varchar(255) not null default '' comment 'split by space: sms voice email dingtalk wecom',
    `notify_groups` varchar(255) not null default '' comment 'split by space: 233 43',
    `notify_repeat_next` bigint not null default 0 comment 'next timestamp to notify, get repeat settings from rule',
    `notify_cur_number` int not null default 0 comment '',
    `target_ident` varchar(191) not null default '' comment 'target ident, also in tags',
    `target_note` varchar(191) not null default '' comment 'target note',
    `first_trigger_time` bigint,
    `trigger_time` bigint not null,
    `trigger_value` text not null,
    `annotations` text not null comment 'annotations',
    `rule_config` text not null comment 'annotations',
    `tags` varchar(1024) not null default '' comment 'merge data_tags rule_tags, split by ,,',
    `original_tags` text comment 'labels key=val,,k2=v2',
    PRIMARY KEY (`id`),
    KEY (`hash`),
    KEY (`rule_id`),
    KEY (`trigger_time`, `group_id`),
    KEY (`notify_repeat_next`)
) ENGINE=InnoDB DEFAULT CHARSET = utf8mb4;

CREATE TABLE `alert_his_event` (
    `id` bigint unsigned not null AUTO_INCREMENT,
    `is_recovered` tinyint(1) not null,
    `cate` varchar(128) not null,
    `datasource_id` bigint not null default 0 comment 'datasource id',
    `cluster` varchar(128) not null,
    `group_id` bigint unsigned not null comment 'busi group id of rule',
    `group_name` varchar(255) not null default '' comment 'busi group name',
    `hash` varchar(64) not null comment 'rule_id + vector_pk',
    `rule_id` bigint unsigned not null,
    `rule_name` varchar(255) not null,
    `rule_note` varchar(2048) not null default 'alert rule note',
    `rule_prod` varchar(255) not null default '',
    `rule_algo` varchar(255) not null default '',
    `severity` tinyint(1) not null comment '0:Emergency 1:Warning 2:Notice',
    `prom_for_duration` int not null comment 'prometheus for, unit:s',
    `prom_ql` varchar(8192) not null comment 'promql',
    `prom_eval_interval` int not null comment 'evaluate interval',
    `callbacks` varchar(2048) not null default '' comment 'split by space: http://a.com/api/x http://a.com/api/y',
    `runbook_url` varchar(255),
    `notify_recovered` tinyint(1) not null comment 'whether notify when recovery',
    `notify_channels` varchar(255) not null default '' comment 'split by space: sms voice email dingtalk wecom',
    `notify_groups` varchar(255) not null default '' comment 'split by space: 233 43',
    `notify_cur_number` int not null default 0 comment '',
    `target_ident` varchar(191) not null default '' comment 'target ident, also in tags',
    `target_note` varchar(191) not null default '' comment 'target note',
    `first_trigger_time` bigint,
    `trigger_time` bigint not null,
    `trigger_value` text not null,
    `recover_time` bigint not null default 0,
    `last_eval_time` bigint not null default 0 comment 'for time filter',
    `tags` varchar(1024) not null default '' comment 'merge data_tags rule_tags, split by ,,',
    `original_tags` text comment 'labels key=val,,k2=v2',
    `annotations` text not null comment 'annotations',
    `rule_config` text not null comment 'annotations',
    PRIMARY KEY (`id`),
    INDEX `idx_last_eval_time` (`last_eval_time`),
    KEY (`hash`),
    KEY (`rule_id`),
    KEY (`trigger_time`, `group_id`)
) ENGINE=InnoDB DEFAULT CHARSET = utf8mb4;

CREATE TABLE `board_busigroup` (
  `busi_group_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'busi group id',
  `board_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'board id',
  PRIMARY KEY (`busi_group_id`, `board_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE `builtin_components` (
  `id` bigint UNSIGNED NOT NULL AUTO_INCREMENT COMMENT 'unique identifier',
  `ident` varchar(191) NOT NULL,
  `logo` mediumtext COMMENT '''logo of component''',
  `readme` text NOT NULL COMMENT '''readme of component''',
  `created_at` bigint NOT NULL DEFAULT 0 COMMENT '''create time''',
  `created_by` varchar(191) NOT NULL DEFAULT '' COMMENT '''creator''',
  `updated_at` bigint NOT NULL DEFAULT 0 COMMENT '''update time''',
  `updated_by` varchar(191) NOT NULL DEFAULT '' COMMENT '''updater''',
  `disabled` int NOT NULL DEFAULT 0 COMMENT '''is disabled or not''',
  PRIMARY KEY (`id`),
  KEY (`ident`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE `builtin_payloads` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT COMMENT '''unique identifier''',
  `component_id` bigint NOT NULL DEFAULT 0 COMMENT '''component_id of payload''',
  `uuid` bigint(20) NOT NULL COMMENT '''uuid of payload''',
  `type` varchar(191) NOT NULL COMMENT '''type of payload''',
  `component` varchar(191) NOT NULL COMMENT '''component of payload''',
  `cate` varchar(191) NOT NULL COMMENT '''category of payload''',
  `name` varchar(191) NOT NULL COMMENT '''name of payload''',
  `tags` varchar(191) NOT NULL DEFAULT '' COMMENT '''tags of payload''',
  `content` longtext NOT NULL COMMENT '''content of payload''',
  `created_at` bigint(20) NOT NULL DEFAULT 0 COMMENT '''create time''',
  `created_by` varchar(191) NOT NULL DEFAULT '' COMMENT '''creator''',
  `updated_at` bigint(20) NOT NULL DEFAULT 0 COMMENT '''update time''',
  `updated_by` varchar(191) NOT NULL DEFAULT '' COMMENT '''updater''',
  PRIMARY KEY (`id`),
  KEY `idx_component` (`component`),
  KEY `idx_name` (`name`),
  KEY `idx_cate` (`cate`),
  KEY `idx_uuid` (`uuid`),
  KEY `idx_type` (`type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE notification_record (
    `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
    `notify_rule_id` BIGINT NOT NULL DEFAULT 0,
    `event_id`  bigint NOT NULL COMMENT 'event history id',
    `sub_id`  bigint COMMENT 'subscribed rule id',
    `channel` varchar(255) NOT NULL COMMENT 'notification channel name',
    `status` bigint COMMENT 'notification status',
    `target` varchar(1024) NOT NULL COMMENT 'notification target',
    `details` varchar(2048) DEFAULT '' COMMENT 'notification other info',
    `created_at` bigint NOT NULL COMMENT 'create time',
    INDEX idx_evt (event_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE `task_tpl`
(
    `id`        int unsigned NOT NULL AUTO_INCREMENT,
    `group_id`  int unsigned not null comment 'busi group id',
    `title`     varchar(255) not null default '',
    `account`   varchar(64)  not null,
    `batch`     int unsigned not null default 0,
    `tolerance` int unsigned not null default 0,
    `timeout`   int unsigned not null default 0,
    `pause`     varchar(255) not null default '',
    `script`    text         not null,
    `args`      varchar(512) not null default '',
    `tags`      varchar(255) not null default '' comment 'split by space',
    `create_at` bigint not null default 0,
    `create_by` varchar(64) not null default '',
    `update_at` bigint not null default 0,
    `update_by` varchar(64) not null default '',
    PRIMARY KEY (`id`),
    KEY (`group_id`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

CREATE TABLE `task_tpl_host`
(
    `ii`   int unsigned NOT NULL AUTO_INCREMENT,
    `id`   int unsigned not null comment 'task tpl id',
    `host` varchar(128)  not null comment 'ip or hostname',
    PRIMARY KEY (`ii`),
    KEY (`id`, `host`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

CREATE TABLE `task_record`
(
    `id` bigint unsigned not null comment 'ibex task id',
    `event_id` bigint not null comment 'event id' default 0,
    `group_id` bigint not null comment 'busi group id',
    `ibex_address`   varchar(128) not null,
    `ibex_auth_user` varchar(128) not null default '',
    `ibex_auth_pass` varchar(128) not null default '',
    `title`     varchar(255)    not null default '',
    `account`   varchar(64)     not null,
    `batch`     int unsigned    not null default 0,
    `tolerance` int unsigned    not null default 0,
    `timeout`   int unsigned    not null default 0,
    `pause`     varchar(255)    not null default '',
    `script`    text            not null,
    `args`      varchar(512)    not null default '',
    `create_at` bigint not null default 0,
    `create_by` varchar(64) not null default '',
    PRIMARY KEY (`id`),
    KEY (`create_at`, `group_id`),
    KEY (`create_by`),
    INDEX `idx_event_id` (`event_id`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

CREATE TABLE `alerting_engines`
(
    `id` int unsigned NOT NULL AUTO_INCREMENT,
    `instance` varchar(128) not null default '' comment 'instance identification, e.g. 10.9.0.9:9090',
    `datasource_id` bigint not null default 0 comment 'datasource id',
    `engine_cluster` varchar(128) not null default '' comment 'n9e-alert cluster',
    `clock` bigint not null,
    PRIMARY KEY (`id`),
    INDEX `idx_inst` (`instance`),
    INDEX `idx_clock` (`clock`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

CREATE TABLE `datasource`
(
    `id` int unsigned NOT NULL AUTO_INCREMENT,
    `name` varchar(191) not null default '',
    `identifier` varchar(255) not null default '',
    `description` varchar(255) not null default '',
    `category` varchar(255) not null default '',
    `plugin_id` int unsigned not null default 0,
    `plugin_type` varchar(255) not null default '',
    `plugin_type_name` varchar(255) not null default '',
    `cluster_name` varchar(255) not null default '',
    `settings` text not null,
    `status` varchar(255) not null default '',
    `http` varchar(4096) not null default '',
    `auth` varchar(8192) not null default '',
    `is_default` boolean COMMENT 'is default datasource',
    `created_at` bigint not null default 0,
    `created_by` varchar(64) not null default '',
    `updated_at` bigint not null default 0,
    `updated_by` varchar(64) not null default '',
    UNIQUE KEY (`name`),
    PRIMARY KEY (`id`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

CREATE TABLE `builtin_cate` (
    `id` bigint unsigned not null auto_increment,
    `name` varchar(191) not null,
    `user_id` bigint not null default 0,
    PRIMARY KEY (`id`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

CREATE TABLE `notify_tpl` (
    `id` bigint unsigned not null auto_increment,
    `channel` varchar(32) not null,
    `name` varchar(255) not null,
    `content` text not null,
    `create_at` bigint DEFAULT 0 COMMENT 'create_at',
    `create_by` varchar(64) DEFAULT '' COMMENT 'cerate_by',
    `update_at` bigint DEFAULT 0 COMMENT 'update_at',
    `update_by` varchar(64) DEFAULT '' COMMENT 'update_by',
    PRIMARY KEY (`id`),
    UNIQUE KEY (`channel`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

CREATE TABLE `sso_config` (
    `id` bigint unsigned not null auto_increment,
    `name` varchar(191) not null,
    `content` text not null,
    `update_at` bigint DEFAULT 0 COMMENT 'update_at',
    PRIMARY KEY (`id`),
    UNIQUE KEY (`name`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

CREATE TABLE `es_index_pattern` (
    `id` bigint unsigned not null auto_increment,
    `datasource_id` bigint not null default 0 comment 'datasource id',
    `name` varchar(191) not null,
    `time_field` varchar(128) not null default '@timestamp',
    `allow_hide_system_indices` tinyint(1) not null default 0,
    `fields_format` varchar(4096) not null default '',
    `cross_cluster_enabled` int not null default 0,
    `note` varchar(1024) not null default '',
    `create_at` bigint default '0',
    `create_by` varchar(64) default '',
    `update_at` bigint default '0',
    `update_by` varchar(64) default '',
    PRIMARY KEY (`id`),
    UNIQUE KEY (`datasource_id`, `name`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;


CREATE TABLE `builtin_metrics` (
    `id` bigint unsigned NOT NULL AUTO_INCREMENT COMMENT 'unique identifier',
    `collector` varchar(191) NOT NULL COMMENT '''type of collector''',
    `typ` varchar(191) NOT NULL COMMENT '''type of metric''',
    `name` varchar(191) NOT NULL COMMENT '''name of metric''',
    `unit` varchar(191) NOT NULL COMMENT '''unit of metric''',
    `lang` varchar(191) NOT NULL DEFAULT 'zh' COMMENT '''language''',
    `note` varchar(4096) NOT NULL COMMENT '''description of metric''',
    `expression` varchar(4096) NOT NULL COMMENT '''expression of metric''',
    `created_at` bigint NOT NULL DEFAULT 0 COMMENT '''create time''',
    `created_by` varchar(191) NOT NULL DEFAULT '' COMMENT '''creator''',
    `updated_at` bigint NOT NULL DEFAULT 0 COMMENT '''update time''',
    `updated_by` varchar(191) NOT NULL DEFAULT '' COMMENT '''updater''',
    `uuid` bigint NOT NULL DEFAULT 0 COMMENT '''uuid''',
    PRIMARY KEY (`id`),
    UNIQUE KEY `idx_collector_typ_name` (`lang`,`collector`, `typ`, `name`),
    INDEX `idx_uuid` (`uuid`),
    INDEX `idx_collector` (`collector`),
    INDEX `idx_typ` (`typ`),
    INDEX `idx_builtinmetric_name` (`name` ASC),
    INDEX `idx_lang` (`lang`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE `metric_filter` (
  `id` bigint NOT NULL AUTO_INCREMENT COMMENT 'unique identifier',
  `name`  varchar(191) NOT NULL COMMENT '''name of metric filter''',
  `configs`  varchar(4096) NOT NULL COMMENT '''configuration of metric filter''',
  `groups_perm` text,
  `create_at` bigint NOT NULL DEFAULT 0 COMMENT '''create time''',
  `create_by` varchar(191) NOT NULL DEFAULT '' COMMENT '''creator''',
  `update_at` bigint NOT NULL DEFAULT 0 COMMENT '''update time''',
  `update_by` varchar(191) NOT NULL DEFAULT '' COMMENT '''updater''',
  PRIMARY KEY (`id`),
  INDEX `idx_metricfilter_name` (`name` ASC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE `target_busi_group` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `target_ident` varchar(191) NOT NULL,
  `group_id` bigint NOT NULL,
  `update_at` bigint NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_target_group` (`target_ident`,`group_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;


CREATE TABLE `dash_annotation` (
    `id` bigint unsigned not null auto_increment,
    `dashboard_id` bigint not null comment 'dashboard id',
    `panel_id` varchar(191) not null comment 'panel id',
    `tags` text comment 'tags array json string',
    `description` text comment 'annotation description',
    `config` text comment 'annotation config',
    `time_start` bigint not null default 0 comment 'start timestamp',
    `time_end` bigint not null default 0 comment 'end timestamp',
    `create_at` bigint not null default 0 comment 'create time',
    `create_by` varchar(64) not null default '' comment 'creator',
    `update_at` bigint not null default 0 comment 'update time',
    `update_by` varchar(64) not null default '' comment 'updater',
    PRIMARY KEY (`id`),
    KEY `idx_dashboard_id` (`dashboard_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE `user_token` (
    `id` bigint NOT NULL AUTO_INCREMENT,
    `username` varchar(255) NOT NULL DEFAULT '',
    `token_name` varchar(255) NOT NULL DEFAULT '',
    `token` varchar(255) NOT NULL DEFAULT '',
    `create_at` bigint NOT NULL DEFAULT 0,
    `last_used` bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;


CREATE TABLE `notify_rule` (
    `id` bigint unsigned not null auto_increment,
    `name` varchar(255) not null,
    `description` text,
    `enable` tinyint(1) not null default 0,
    `user_group_ids` varchar(255) not null default '',
    `notify_configs` text,
    `pipeline_configs` text,
    `create_at` bigint not null default 0,
    `create_by` varchar(64) not null default '',
    `update_at` bigint not null default 0,
    `update_by` varchar(64) not null default '',
    PRIMARY KEY (`id`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

CREATE TABLE `notify_channel` (
    `id` bigint unsigned not null auto_increment,
    `name` varchar(255) not null,
    `ident` varchar(255) not null,
    `description` text, 
    `enable` tinyint(1) not null default 0,
    `param_config` text,
    `request_type` varchar(50) not null,
    `request_config` text,
    `weight` int not null default 0,
    `create_at` bigint not null default 0,
    `create_by` varchar(64) not null default '',
    `update_at` bigint not null default 0,
    `update_by` varchar(64) not null default '',
    PRIMARY KEY (`id`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

CREATE TABLE `message_template` (
    `id` bigint unsigned not null auto_increment,
    `name` varchar(64) not null,
    `ident` varchar(64) not null,
    `content` text,
    `user_group_ids` varchar(64),
    `notify_channel_ident` varchar(64) not null default '',
    `private` int not null default 0,
    `weight` int not null default 0,
    `create_at` bigint not null default 0,
    `create_by` varchar(64) not null default '',
    `update_at` bigint not null default 0,
    `update_by` varchar(64) not null default '',
    PRIMARY KEY (`id`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

CREATE TABLE `event_pipeline` (
    `id` bigint unsigned not null auto_increment,
    `name` varchar(128) not null,
    `team_ids` text,
    `description` varchar(255) not null default '',
    `filter_enable` tinyint(1) not null default 0,
    `label_filters` text,
    `attribute_filters` text,
    `processors` text,
    `create_at` bigint not null default 0,
    `create_by` varchar(64) not null default '',
    `update_at` bigint not null default 0,
    `update_by` varchar(64) not null default '',
    PRIMARY KEY (`id`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

CREATE TABLE `embedded_product` (
    `id` bigint unsigned NOT NULL AUTO_INCREMENT,
    `name` varchar(255) DEFAULT NULL,
    `url` varchar(255) DEFAULT NULL,
    `is_private` boolean DEFAULT NULL,
    `team_ids` varchar(255),
    `create_at` bigint not null default 0,
    `create_by` varchar(64) not null default '',
    `update_at` bigint not null default 0,
    `update_by` varchar(64) not null default '',
    PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE `task_meta`
(
    `id`          bigint unsigned NOT NULL AUTO_INCREMENT,
    `title`       varchar(255)    not null default '',
    `account`     varchar(64)     not null,
    `batch`       bigint          not null default 0,
    `tolerance`   bigint          not null default 0,
    `timeout`     bigint    not null default 0,
    `pause`       varchar(255)    not null default '',
    `script`      text            not null,
    `args`        varchar(512)    not null default '',
    `stdin`       varchar(1024)   not null default '',
    `creator`     varchar(64)     not null default '',
    `created`     timestamp       not null default CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    KEY `idx_task_meta_creator` (`creator`),
    KEY `idx_task_meta_created` (`created`)
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
    `scheduler` varchar(128) NOT NULL,
    `clock`     bigint not null,
    UNIQUE KEY `idx_task_scheduler_health_scheduler` (`scheduler`),
    KEY (`clock`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;

CREATE TABLE `task_host_doing`
(
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `clock`  bigint          not null default 0,
    `action` varchar(16)     not null,
    KEY `idx_task_host_doing_id` (`id`),
   KEY `idx_task_host_doing_host` (`host`)
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
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
    UNIQUE KEY `idx_id_host` (`id`, `host`),
    PRIMARY KEY (`ii`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4;
