set names utf8;

drop database if exists n9e;
create database n9e;
use n9e;

CREATE TABLE `user` (
    `id` bigint unsigned not null auto_increment,
    `username` varchar(64) not null comment 'login name, cannot rename',
    `nickname` varchar(64) not null comment 'display name, chinese name',
    `password` varchar(128) not null,
    `phone` varchar(16) not null default '',
    `email` varchar(64) not null default '',
    `portrait` varchar(255) not null default '' comment 'portrait image url',
    `status` tinyint(1) not null default 0 comment '0: active, 1: disabled',
    `role` varchar(32) not null comment 'Admin | Standard | Guest',
    `contacts` varchar(1024)  default '' comment 'json e.g. {wecom:xx, dingtalk_robot_token:yy}',
    `create_at` bigint not null default 0,
    `create_by` varchar(64) not null default '',
    `update_at` bigint not null default 0,
    `update_by` varchar(64) not null default '',
    PRIMARY KEY (`id`),
    UNIQUE KEY (`username`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8;

CREATE TABLE `user_token` (
    `user_id` bigint unsigned not null,
    `username` varchar(64) not null,
    `token` varchar(128) not null,
    KEY (`user_id`),
    KEY (`username`),
    UNIQUE KEY (`token`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8;

CREATE TABLE `user_group` (
    `id` bigint unsigned not null auto_increment,
    `name` varchar(128) not null default '',
    `note` varchar(255) not null default '',
    `create_at` bigint not null default 0,
    `create_by` varchar(64) not null default '',
    `update_at` bigint not null default 0,
    `update_by` varchar(64) not null default '',
    PRIMARY KEY (`id`),
    KEY (`create_by`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8;

CREATE TABLE `user_group_member` (
    `group_id` bigint unsigned not null,
    `user_id` bigint unsigned not null,
    KEY (`group_id`),
    KEY (`user_id`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8;

CREATE TABLE `configs` (
    `id` bigint unsigned not null auto_increment,
    `ckey` varchar(255) not null,
    `cval` varchar(1024) not null default '',
    PRIMARY KEY (`id`),
    UNIQUE KEY (`ckey`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8;

CREATE TABLE `role` (
    `id` bigint unsigned not null auto_increment,
    `name` varchar(128) not null default '',
    `note` varchar(255) not null default '',
    PRIMARY KEY (`id`),
    UNIQUE KEY (`name`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8;

insert into `role`(name, note) values('Admin', 'Administrator role');
insert into `role`(name, note) values('Standard', 'Ordinary user role');
insert into `role`(name, note) values('Guest', 'Readonly user role');

CREATE TABLE `role_operation`(
    `role_name` varchar(128) not null,
    `operation` varchar(255) not null,
    KEY (`role_name`),
    KEY (`operation`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8;

-- Admin is special, who has no concrete operation but can do anything.
insert into `role_operation`(role_name, operation) values('Standard', 'classpath_create');
insert into `role_operation`(role_name, operation) values('Standard', 'classpath_modify');
insert into `role_operation`(role_name, operation) values('Standard', 'classpath_delete');
insert into `role_operation`(role_name, operation) values('Standard', 'classpath_add_resource');
insert into `role_operation`(role_name, operation) values('Standard', 'classpath_del_resource');
insert into `role_operation`(role_name, operation) values('Standard', 'metric_description_create');
insert into `role_operation`(role_name, operation) values('Standard', 'metric_description_modify');
insert into `role_operation`(role_name, operation) values('Standard', 'metric_description_delete');
insert into `role_operation`(role_name, operation) values('Standard', 'mute_create');
insert into `role_operation`(role_name, operation) values('Standard', 'mute_delete');
insert into `role_operation`(role_name, operation) values('Standard', 'dashboard_create');
insert into `role_operation`(role_name, operation) values('Standard', 'dashboard_modify');
insert into `role_operation`(role_name, operation) values('Standard', 'dashboard_delete');
insert into `role_operation`(role_name, operation) values('Standard', 'alert_rule_group_create');
insert into `role_operation`(role_name, operation) values('Standard', 'alert_rule_group_modify');
insert into `role_operation`(role_name, operation) values('Standard', 'alert_rule_group_delete');
insert into `role_operation`(role_name, operation) values('Standard', 'alert_rule_create');
insert into `role_operation`(role_name, operation) values('Standard', 'alert_rule_modify');
insert into `role_operation`(role_name, operation) values('Standard', 'alert_rule_delete');
insert into `role_operation`(role_name, operation) values('Standard', 'alert_event_delete');
insert into `role_operation`(role_name, operation) values('Standard', 'collect_rule_create');
insert into `role_operation`(role_name, operation) values('Standard', 'collect_rule_modify');
insert into `role_operation`(role_name, operation) values('Standard', 'collect_rule_delete');
insert into `role_operation`(role_name, operation) values('Standard', 'resource_modify');

CREATE TABLE `instance` (
    `service`   varchar(128)    not null,
    `endpoint`  varchar(255)    not null comment 'ip:port',
    `clock`     datetime        not null,
    KEY (`service`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8;

-- if mute_etime < now(), the two mute columns should be reset to 0
CREATE TABLE `resource` (
    `id` bigint unsigned not null auto_increment,
    `ident` varchar(255) not null,
    `alias` varchar(128) not null default '' comment 'auto detect, just for debug',
    `tags` varchar(512) not null default '' comment 'will append to event',
    `note` varchar(255) not null default '',
    `mute_btime` bigint not null default 0 comment 'mute begin time',
    `mute_etime` bigint not null default 0 comment 'mute end time',
    PRIMARY KEY (`id`),
    UNIQUE KEY (`ident`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8;

CREATE TABLE `classpath` (
    `id` bigint unsigned not null auto_increment,
    `path` varchar(512) not null comment 'required. e.g. duokan.tv.engine.x.y.z',
    `note` varchar(255) not null default '',
    `preset`    tinyint(1) not null default 0 comment 'if preset, cannot delete and modify',
    `create_at` bigint not null default 0,
    `create_by` varchar(64) not null default '',
    `update_at` bigint not null default 0,
    `update_by` varchar(64) not null default '',
    PRIMARY KEY (`id`),
    KEY (`path`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8;

-- new resource will bind classpath(id=1) automatically
insert into classpath(id, path, note, preset, create_by, update_by, create_at, update_at) values(1, 'all', 'preset classpath, all resources belong to', 1, 'system', 'system', unix_timestamp(now()), unix_timestamp(now()));

CREATE TABLE `classpath_resource` (
    `id` bigint unsigned not null auto_increment,
    `classpath_id` bigint unsigned not null,
    `res_ident` varchar(255) not null,
    PRIMARY KEY (`id`),
    KEY (`classpath_id`),
    KEY (`res_ident`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8;

CREATE TABLE `classpath_favorite` (
    `id` bigint unsigned not null auto_increment,
    `classpath_id` bigint not null,
    `user_id` bigint not null,
    PRIMARY KEY (`id`),
    KEY (`user_id`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8;

CREATE TABLE `mute` (
    `id` bigint unsigned not null auto_increment,
    `metric` varchar(255) not null comment 'required',
    `res_filters` varchar(4096) not null default 'resource filters',
    `tag_filters` varchar(8192) not null default '',
    `cause` varchar(255) not null default '',
    `btime` bigint not null default 0 comment 'begin time',
    `etime` bigint not null default 0 comment 'end time',
    `create_at` bigint not null default 0,
    `create_by` varchar(64) not null default '',
    PRIMARY KEY (`id`),
    KEY (`metric`),
    KEY (`create_by`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8;

CREATE TABLE `dashboard` (
    `id` bigint unsigned not null auto_increment,
    `name` varchar(255) not null,
    `tags` varchar(255) not null,
    `configs` varchar(4096) comment 'dashboard variables',
    `create_at` bigint not null default 0,
    `create_by` varchar(64) not null default '',
    `update_at` bigint not null default 0,
    `update_by` varchar(64) not null default '',
    PRIMARY KEY (`id`),
    UNIQUE KEY (`name`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8;

CREATE TABLE `dashboard_favorite` (
    `id` bigint unsigned not null auto_increment,
    `dashboard_id` bigint not null comment 'dashboard id',
    `user_id` bigint not null comment 'user id',
    PRIMARY KEY (`id`),
    KEY (`user_id`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8;

-- auto create the first subclass 'Default chart group' of dashboard
CREATE TABLE `chart_group` (
    `id` bigint unsigned not null auto_increment,
    `dashboard_id` bigint unsigned not null,
    `name` varchar(255) not null,
    `weight` int not null default 0,
    PRIMARY KEY (`id`),
    KEY (`dashboard_id`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8;

CREATE TABLE `chart` (
    `id` bigint unsigned not null auto_increment,
    `group_id` bigint unsigned not null comment 'chart group id',
    `configs` varchar(8192),
    `weight` int not null default 0,
    PRIMARY KEY (`id`),
    KEY (`group_id`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8;

CREATE TABLE `chart_tmp` (
    `id` bigint unsigned not null auto_increment,
    `configs` varchar(8192),
    `create_at` bigint not null default 0,
    `create_by` varchar(64) not null default '',
    primary key (`id`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8;

CREATE TABLE `collect_rule` (
    `id` bigint unsigned not null auto_increment,
    `classpath_id` bigint not null,
    `prefix_match` tinyint(1) not null default 0 comment '0: no 1: yes',
    `name` varchar(255) not null default '',
    `note` varchar(255) not null default '',
    `step` int not null,
    `type` varchar(64) not null comment 'e.g. port proc log plugin mysql',
    `data` text not null,
    `append_tags` varchar(255) not null default '' comment 'e.g. mod=n9e',
    `create_at` bigint not null default 0,
    `create_by` varchar(64) not null default '',
    `update_at` bigint not null default 0,
    `update_by` varchar(64) not null default '',
    PRIMARY KEY (`id`),
    KEY (`classpath_id`, `type`),
    KEY (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

CREATE TABLE `alert_rule_group` (
    `id` bigint unsigned not null auto_increment,
    `name` varchar(255) not null,
    `user_group_ids` varchar(255) not null default '' comment 'readwrite user group ids',
    `create_at` bigint not null default 0,
    `create_by` varchar(64) not null default '',
    `update_at` bigint not null default 0,
    `update_by` varchar(64) not null default '',
    PRIMARY KEY (`id`),
    UNIQUE KEY (`name`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8;

insert into alert_rule_group(name,create_at,create_by,update_at,update_by) values('Default Rule Group', unix_timestamp(now()), 'system', unix_timestamp(now()), 'system');

CREATE TABLE `alert_rule_group_favorite` (
    `id` bigint unsigned not null auto_increment,
    `group_id` bigint not null comment 'alert_rule group id',
    `user_id` bigint not null comment 'user id',
    PRIMARY KEY (`id`),
    KEY (`user_id`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8;

CREATE TABLE `alert_rule` (
    `id` bigint unsigned not null auto_increment,
    `group_id` bigint not null default 0 comment 'alert_rule group id',
    `name` varchar(255) not null,
    `note` varchar(255) not null,
    `type` tinyint(1) not null comment '0 n9e 1 promql',
    `status` tinyint(1) not null comment '0 enable 1 disable',
    `alert_duration` int not null comment 'unit:s',
    `expression` varchar(4096) not null comment 'rule expression',
    `enable_stime` char(5) not null default '00:00',
    `enable_etime` char(5) not null default '23:59',
    `enable_days_of_week` varchar(32) not null default '' comment 'split by space: 0 1 2 3 4 5 6',
    `recovery_notify` tinyint(1) not null comment 'whether notify when recovery',
    `priority` tinyint(1) not null,
    `notify_channels` varchar(255) not null default '' comment 'split by space: sms voice email dingtalk wecom',
    `notify_groups` varchar(255) not null default '' comment 'split by space: 233 43',
    `notify_users` varchar(255) not null default '' comment 'split by space: 2 5',
    `callbacks` varchar(255) not null default '' comment 'split by space: http://a.com/api/x http://a.com/api/y',
    `runbook_url` varchar(255),
    `append_tags` varchar(255) not null default '' comment 'split by space: service=n9e mod=api',
    `create_at` bigint not null default 0,
    `create_by` varchar(64) not null default '',
    `update_at` bigint not null default 0,
    `update_by` varchar(64) not null default '',
    PRIMARY KEY (`id`),
    KEY (`group_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
 
CREATE TABLE `alert_event` (
    `id` bigint unsigned not null auto_increment,
    `hash_id` varchar(255) not null comment 'rule_id + point_pk',
    `rule_id` bigint unsigned not null,
    `rule_name` varchar(255) not null,
    `rule_note` varchar(512) not null default 'alert rule note',
    `res_classpaths` varchar(1024) not null default '' comment 'belong classpaths',
    `priority` tinyint(1) not null,
    `status` tinyint(1) not null,
    `is_prome_pull` tinyint(1) not null,
    `history_points` text comment 'metric, history points',
    `trigger_time` bigint not null,
    `notify_channels` varchar(255) not null default '',
    `notify_groups` varchar(255) not null default '',
    `notify_users` varchar(255) not null default '',
    `runbook_url` varchar(255),
    `readable_expression` varchar(1024) not null comment 'e.g. mem.bytes.used.percent(all,60s) > 0',
    `tags` varchar(1024) not null default 'merge data_tags rule_tags and res_tags',
    PRIMARY KEY (`id`),
    KEY (`hash_id`),
    KEY (`rule_id`),
    KEY (`trigger_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

CREATE TABLE `metric_description` (
    `id` bigint unsigned not null auto_increment,
    `metric` varchar(255) not null default '',
    `description` varchar(255) not null default '',
    PRIMARY KEY (`id`),
    UNIQUE KEY (`metric`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

insert into metric_description(metric, description) values('system_cpu_idle', '系统总体CPU空闲率(单位：%)');
insert into metric_description(metric, description) values('system_cpu_util', '系统总体CPU使用率(单位：%)');
