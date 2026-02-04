CREATE TABLE `users` (
    `id` integer primary key autoincrement,
    `username` varchar(64) not null unique,
    `nickname` varchar(64) not null,
    `password` varchar(128) not null default '',
    `phone` varchar(16) not null default '',
    `email` varchar(64) not null default '',
    `portrait` varchar(255) not null default '',
    `roles` varchar(255) not null,
    `contacts` varchar(1024),
    `maintainer` tinyint(1) not null default 0,
    `belong` varchar(16) not null default '',
    `last_active_time` bigint not null default 0,
    `create_at` bigint not null default 0,
    `create_by` varchar(64) not null default '',
    `update_at` bigint not null default 0,
    `update_by` varchar(64) not null default ''
);

CREATE UNIQUE INDEX idx_users_username ON `users` (username);

insert into `users`(id, username, nickname, password, roles, create_at, create_by, update_at, update_by) values(1, 'root', '超管', 'root.2020', 'Admin', strftime('%s', 'now'), 'system', strftime('%s', 'now'), 'system');

CREATE TABLE `user_group` (
    `id` integer primary key autoincrement,
    `name` varchar(128) not null default '',
    `note` varchar(255) not null default '',
    `create_at` bigint not null default 0,
    `create_by` varchar(64) not null default '',
    `update_at` bigint not null default 0,
    `update_by` varchar(64) not null default ''
);
CREATE INDEX `idx_user_group_create_by` ON `user_group` (`create_by` asc);
CREATE INDEX `idx_user_group_update_at` ON `user_group` (`update_at` asc);

insert into user_group(id, name, create_at, create_by, update_at, update_by) values(1, 'demo-root-group', strftime('%s', 'now'), 'root', strftime('%s', 'now'), 'root');

CREATE TABLE `user_group_member` (
    `id` integer primary key autoincrement,
    `group_id` bigint unsigned not null,
    `user_id` bigint unsigned not null
);
CREATE INDEX `idx_user_group_member_group_id` ON `user_group_member` (`group_id` asc);
CREATE INDEX `idx_user_group_member_user_id` ON `user_group_member` (`user_id` asc);

insert into user_group_member(group_id, user_id) values(1, 1);

CREATE TABLE `configs` (
    `id` integer primary key autoincrement,
    `ckey` varchar(191) not null,
    `cval` text not null,
    `note` varchar(1024) not null default '',
    `external` tinyint(1) not null default 0,
    `encrypted` tinyint(1) not null default 0,
    `create_at` bigint not null default 0,
    `create_by` varchar(64) not null default '',
    `update_at` bigint not null default 0,
    `update_by` varchar(64) not null default ''
);

CREATE TABLE `role` (
    `id` integer primary key autoincrement,
    `name` varchar(191) not null unique default '',
    `note` varchar(255) not null default ''
);

insert into `role`(name, note) values('Admin', 'Administrator role');
insert into `role`(name, note) values('Standard', 'Ordinary user role');
insert into `role`(name, note) values('Guest', 'Readonly user role');


CREATE TABLE `role_operation`(
    `id` integer primary key autoincrement,
    `role_name` varchar(128) not null,
    `operation` varchar(191) not null
);
CREATE INDEX `idx_role_operation_role_name` ON `role_operation` (`role_name` asc);
CREATE INDEX `idx_role_operation_operation` ON `role_operation` (`operation` asc);

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

insert into `role_operation`(role_name, operation) values('Admin', '/help/source');
insert into `role_operation`(role_name, operation) values('Admin', '/help/sso');
insert into `role_operation`(role_name, operation) values('Admin', '/help/notification-tpls');
insert into `role_operation`(role_name, operation) values('Admin', '/help/notification-settings');

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
    `id` integer primary key autoincrement,
    `name` varchar(191) not null unique,
    `label_enable` tinyint(1) not null default 0,
    `label_value` varchar(191) not null default '',
    `create_at` bigint not null default 0,
    `create_by` varchar(64) not null default '',
    `update_at` bigint not null default 0,
    `update_by` varchar(64) not null default ''
);

insert into busi_group(id, name, create_at, create_by, update_at, update_by) values(1, 'Default Busi Group', strftime('%s', 'now'), 'root', strftime('%s', 'now'), 'root');

CREATE TABLE `busi_group_member` (
    `id` integer primary key autoincrement,
    `busi_group_id` bigint not null,
    `user_group_id` bigint not null,
    `perm_flag` char(2) not null
);
CREATE INDEX `idx_busi_group_member_busi_group_id` ON `busi_group_member` (`busi_group_id` asc);
CREATE INDEX `idx_busi_group_member_user_group_id` ON `busi_group_member` (`user_group_id` asc);

insert into busi_group_member(busi_group_id, user_group_id, perm_flag) values(1, 1, 'rw');

-- for dashboard new version
CREATE TABLE `board` (
    `id` integer primary key autoincrement,
    `group_id` bigint not null default 0,
    `name` varchar(191) not null,
    `ident` varchar(200) not null default '',
    `tags` varchar(255) not null,
    `public` tinyint(1) not null default 0,
    `built_in` tinyint(1) not null default 0,
    `hide` tinyint(1) not null default 0,
    `create_at` bigint not null default 0,
    `create_by` varchar(64) not null default '',
    `update_at` bigint not null default 0,
    `update_by` varchar(64) not null default '',
    `note` varchar(1024) not null default '',
    `public_cate` bigint not null default 0
);
CREATE UNIQUE INDEX idx_board_group_id_name ON `board` (group_id, name);
CREATE INDEX `idx_board_ident` ON `board` (`ident` asc);

-- for dashboard new version
CREATE TABLE `board_payload` (
    `id` bigint unsigned not null unique,
    `payload` mediumtext not null
);

CREATE TABLE `chart` (
    `id` integer primary key autoincrement,
    `group_id` integer not null,
    `configs` text,
    `weight` integer not null default 0
);

CREATE INDEX idx_chart_group_id ON `chart` (group_id);

CREATE TABLE `chart_share` (
    `id` integer primary key autoincrement,
    `cluster` varchar(128) not null,
    `datasource_id` bigint unsigned not null default 0,
    `configs` text,
    `create_at` bigint not null default 0,
    `create_by` varchar(64) not null default ''
);
CREATE INDEX `idx_chart_share_create_at` ON `chart_share` (`create_at` asc);

CREATE TABLE `alert_rule` (
    `id` integer primary key autoincrement,
    `group_id` bigint not null default 0,
    `cate` varchar(128) not null,
    `datasource_ids` varchar(255) not null default '',
    `cluster` varchar(128) not null,
    `name` varchar(255) not null,
    `note` varchar(1024) not null default '',
    `prod` varchar(255) not null default '',
    `algorithm` varchar(255) not null default '',
    `algo_params` varchar(255),
    `delay` int not null default 0,
    `severity` tinyint(1) not null,
    `disabled` tinyint(1) not null,
    `prom_for_duration` int not null,
    `rule_config` text not null,
    `prom_ql` text not null,
    `prom_eval_interval` int not null,
    `enable_stime` varchar(255) not null default '00:00',
    `enable_etime` varchar(255) not null default '23:59',
    `enable_days_of_week` varchar(255) not null default '',
    `enable_in_bg` tinyint(1) not null default 0,
    `notify_recovered` tinyint(1) not null,
    `notify_channels` varchar(255) not null default '',
    `notify_groups` varchar(255) not null default '',
    `notify_repeat_step` int not null default 0,
    `notify_max_number` int not null default 0,
    `recover_duration` int not null default 0 ,
    `callbacks` varchar(4096) not null default '',
    `runbook_url` varchar(4096),
    `append_tags` varchar(255) not null default '',
    `annotations` text not null,
    `extra_config` text not null,
    `create_at` bigint not null default 0,
    `create_by` varchar(64) not null default '',
    `update_at` bigint not null default 0,
    `update_by` varchar(64) not null default '',
    `cron_pattern` varchar(64),
    `datasource_queries` text
);
CREATE INDEX `idx_alert_rule_group_id` ON `alert_rule` (`group_id` asc);
CREATE INDEX `idx_alert_rule_update_at` ON `alert_rule` (`update_at` asc);

CREATE TABLE `alert_mute` (
    `id` integer primary key autoincrement,
    `group_id` bigint not null default 0,
    `prod` varchar(255) not null default '',
    `note` varchar(1024) not null default '',
    `cate` varchar(128) not null,
    `cluster` varchar(128) not null,
    `datasource_ids` varchar(255) not null default '',
    `tags` varchar(4096) default '[]',
    `cause` varchar(255) not null default '',
    `btime` bigint not null default 0,
    `etime` bigint not null default 0,
    `disabled` tinyint(1) not null default 0,
    `mute_time_type` tinyint(1) not null default 0,
    `periodic_mutes` varchar(4096) not null default '',
    `severities` varchar(32) not null default '',
    `create_at` bigint not null default 0,
    `create_by` varchar(64) not null default '',
    `update_at` bigint not null default 0,
    `update_by` varchar(64) not null default ''
);
CREATE INDEX `idx_alert_mute_create_at` ON `alert_mute` (`create_at` asc);
CREATE INDEX `idx_alert_mute_group_id` ON `alert_mute` (`group_id` asc);

CREATE TABLE `alert_subscribe` (
    `id` integer primary key autoincrement,
    `name` varchar(255) not null default '',
    `disabled` tinyint(1) not null default 0,
    `group_id` bigint not null default 0,
    `prod` varchar(255) not null default '',
    `cate` varchar(128) not null,
    `datasource_ids` varchar(255) not null default '',
    `cluster` varchar(128) not null,
    `rule_id` bigint not null default 0,
    `severities` varchar(32) not null default '',
    `tags` varchar(4096) not null default '',
    `redefine_severity` tinyint(1) default 0,
    `new_severity` tinyint(1) not null,
    `redefine_channels` tinyint(1) default 0,
    `new_channels` varchar(255) not null default '',
    `user_group_ids` varchar(250) not null,
    `busi_groups` VARCHAR(4096) NOT NULL DEFAULT '[]',
    `note` VARCHAR(1024) DEFAULT '',
    `rule_ids` VARCHAR(1024) DEFAULT '',
    `webhooks` text not null,
    `extra_config` text not null,
    `redefine_webhooks` tinyint(1) default 0,
    `for_duration` bigint not null default 0,
    `create_at` bigint not null default 0,
    `create_by` varchar(64) not null default '',
    `update_at` bigint not null default 0,
    `update_by` varchar(64) not null default ''
);
CREATE INDEX `idx_alert_subscribe_update_at` ON `alert_subscribe` (`update_at` asc);
CREATE INDEX `idx_alert_subscribe_group_id` ON `alert_subscribe` (`group_id` asc);


CREATE TABLE `target` (
    `id` integer primary key autoincrement,
    `group_id` bigint not null default 0,
    `ident` varchar(191) not null unique,
    `note` varchar(255) not null default '',
    `tags` varchar(512) not null default '',
    `host_ip` varchar(15) default '',
    `agent_version` varchar(255) default '',
    `host_tags` text,
    `engine_name` varchar(255) default '',
    `os` varchar(31) default '',
    `update_at` bigint not null default 0
);

CREATE INDEX `idx_target_group_id` ON `target` (`group_id` asc);
CREATE UNIQUE INDEX idx_target_ident ON `target` (ident);
CREATE INDEX idx_host_ip ON `target` (host_ip);
CREATE INDEX idx_agent_version ON `target` (agent_version);
CREATE INDEX idx_engine_name ON `target` (engine_name);
CREATE INDEX idx_os ON `target` (os);

CREATE TABLE `metric_view` (
    `id` integer primary key autoincrement,
    `name` varchar(191) not null default '',
    `cate` tinyint(1) not null,
    `configs` varchar(8192) not null default '',
    `create_at` bigint not null default 0,
    `create_by` bigint not null default 0,
    `update_at` bigint not null default 0
);
CREATE INDEX `idx_metric_view_create_by` ON `metric_view` (`create_by` asc);

insert into metric_view(name, cate, configs) values('Host View', 0, '{"filters":[{"oper":"=","label":"__name__","value":"cpu_usage_idle"}],"dynamicLabels":[],"dimensionLabels":[{"label":"ident","value":""}]}');

CREATE TABLE `recording_rule` (
    `id` integer primary key autoincrement,
    `group_id` bigint not null default '0',
    `datasource_ids` varchar(255) not null default '',
    `cluster` varchar(128) not null,
    `name` varchar(255) not null,
    `note` varchar(255) not null,
    `disabled` tinyint(1) not null default 0,
    `prom_ql` varchar(8192) not null,
    `prom_eval_interval` int not null,
    `cron_pattern` varchar(255) default '',
    `append_tags` varchar(255) default '',
    `query_configs` text not null,
    `create_at` bigint default '0',
    `create_by` varchar(64) default '',
    `update_at` bigint default '0',
    `update_by` varchar(64) default '',
    `datasource_queries` text
);
CREATE INDEX `idx_recording_rule_group_id` ON `recording_rule` (`group_id` asc);
CREATE INDEX `idx_recording_rule_update_at` ON `recording_rule` (`update_at` asc);

CREATE TABLE `alert_aggr_view` (
    `id` integer primary key autoincrement,
    `name` varchar(191) not null default '',
    `rule` varchar(2048) not null default '',
    `cate` tinyint(1) not null,
    `create_at` bigint not null default 0,
    `create_by` bigint not null default 0,
    `update_at` bigint not null default 0
);
CREATE INDEX `idx_alert_aggr_view_create_by` ON `alert_aggr_view` (`create_by` asc);

insert into alert_aggr_view(name, rule, cate) values('By BusiGroup, Severity', 'field:group_name::field:severity', 0);
insert into alert_aggr_view(name, rule, cate) values('By RuleName', 'field:rule_name', 0);

CREATE TABLE `alert_cur_event` (
    `id` integer primary key autoincrement,
    `cate` varchar(128) not null,
    `datasource_id` bigint not null default 0,
    `cluster` varchar(128) not null,
    `group_id` bigint unsigned not null,
    `group_name` varchar(255) not null default '',
    `hash` varchar(64) not null,
    `rule_id` bigint unsigned not null,
    `rule_name` varchar(255) not null,
    `rule_note` varchar(2048) not null default 'alert rule note',
    `rule_prod` varchar(255) not null default '',
    `rule_algo` varchar(255) not null default '',
    `severity` tinyint(1) not null,
    `prom_for_duration` int not null,
    `prom_ql` varchar(8192) not null,
    `prom_eval_interval` int not null,
    `callbacks` varchar(255) not null default '',
    `runbook_url` varchar(255),
    `notify_recovered` tinyint(1) not null,
    `notify_channels` varchar(255) not null default '',
    `notify_groups` varchar(255) not null default '',
    `notify_repeat_next` bigint not null default 0,
    `notify_cur_number` int not null default 0,
    `target_ident` varchar(191) not null default '',
    `target_note` varchar(191) not null default '',
    `first_trigger_time` bigint,
    `trigger_time` bigint not null,
    `trigger_value` varchar(2048) not null,
    `annotations` text not null,
    `rule_config` text not null,
    `tags` varchar(1024) not null default ''
);
CREATE INDEX `idx_alert_cur_event_hash` ON `alert_cur_event` (`hash` asc);
CREATE INDEX `idx_alert_cur_event_rule_id` ON `alert_cur_event` (`rule_id` asc);
CREATE INDEX `idx_alert_cur_event_trigger_time_group_id` ON `alert_cur_event` (`trigger_time`, `group_id` asc);
CREATE INDEX `idx_alert_cur_event_notify_repeat_next` ON `alert_cur_event` (`notify_repeat_next` asc);

CREATE TABLE `alert_his_event` (
    `id` integer primary key autoincrement,
    `is_recovered` tinyint(1) not null,
    `cate` varchar(128) not null,
    `datasource_id` bigint not null default 0,
    `cluster` varchar(128) not null,
    `group_id` bigint unsigned not null,
    `group_name` varchar(255) not null default '',
    `hash` varchar(64) not null,
    `rule_id` bigint unsigned not null,
    `rule_name` varchar(255) not null,
    `rule_note` varchar(2048) not null default 'alert rule note',
    `rule_prod` varchar(255) not null default '',
    `rule_algo` varchar(255) not null default '',
    `severity` tinyint(1) not null,
    `prom_for_duration` int not null,
    `prom_ql` varchar(8192) not null,
    `prom_eval_interval` int not null,
    `callbacks` varchar(255) not null default '',
    `runbook_url` varchar(255),
    `notify_recovered` tinyint(1) not null,
    `notify_channels` varchar(255) not null default '',
    `notify_groups` varchar(255) not null default '',
    `notify_cur_number` int not null default 0,
    `target_ident` varchar(191) not null default '',
    `target_note` varchar(191) not null default '',
    `first_trigger_time` bigint,
    `trigger_time` bigint not null,
    `trigger_value` varchar(2048) not null,
    `recover_time` bigint not null default 0,
    `last_eval_time` bigint not null default 0,
    `original_tags` varchar(8192),
    `tags` varchar(1024) not null default '',
    `annotations` text not null,
    `rule_config` text not null
);
CREATE INDEX `idx_alert_his_event_last_eval_time` ON `alert_his_event` (`last_eval_time` asc);
CREATE INDEX `idx_alert_his_event_hash` ON `alert_his_event` (`hash` asc);
CREATE INDEX `idx_alert_his_event_rule_id` ON `alert_his_event` (`rule_id` asc);
CREATE INDEX `idx_alert_his_event_trigger_time_group_id` ON `alert_his_event` (`trigger_time`, `group_id` asc);

CREATE TABLE `board_busigroup` (
  `busi_group_id` bigint(20) NOT NULL DEFAULT '0',
  `board_id` bigint(20) NOT NULL DEFAULT '0',
  primary key (`busi_group_id`, `board_id`)
);

CREATE TABLE `builtin_components` (
  `id` integer primary key autoincrement,
  `ident` varchar(191) not null,
  `logo` varchar(191) not null,
  `readme` text not null,
  `created_at` bigint(20) not null default 0,
  `created_by` varchar(191) not null default '',
  `updated_at` bigint(20) not null default 0,
  `updated_by` varchar(191) not null default ''
);
CREATE INDEX `idx_builtin_components_ident` ON `builtin_components` (`ident` asc);

CREATE TABLE `builtin_payloads` (
  `id` integer primary key autoincrement,
  `component_id` integer not null default 0,
  `uuid` integer not null,
  `type` varchar(191) not null,
  `component` varchar(191) not null,
  `cate` varchar(191) not null,
  `name` varchar(191) not null,
  `tags` varchar(191) not null default '',
  `content` longtext not null,
  `note` varchar(1024) not null default '',
  `created_at` bigint(20) not null default 0,
  `created_by` varchar(191) not null default '',
  `updated_at` bigint(20) not null default 0,
  `updated_by` varchar(191) not null default ''
);
CREATE INDEX `idx_builtin_payloads_component` ON `builtin_payloads` (`component` asc);
CREATE INDEX `idx_builtin_payloads_name` ON `builtin_payloads` (`name` asc);
CREATE INDEX `idx_builtin_payloads_cate` ON `builtin_payloads` (`cate` asc);
CREATE INDEX `idx_builtin_payloads_type` ON `builtin_payloads` (`type` asc);
CREATE INDEX idx_uuid ON `builtin_payloads` (uuid);


CREATE TABLE `notification_record` (
    `id` integer primary key autoincrement,
    `event_id` integer not null,
    `sub_id` integer,
    `channel` varchar(255) not null,
    `status` integer,
    `target` varchar(1024) not null,
    `details` varchar(2048) default '',
    `created_at` integer not null
);
CREATE INDEX idx_evt ON notification_record (event_id);

CREATE TABLE `task_tpl` (
    `id`        integer primary key autoincrement,
    `group_id`  int unsigned not null,
    `title`     varchar(255) not null default '',
    `account`   varchar(64)  not null,
    `batch`     int unsigned not null default 0,
    `tolerance` int unsigned not null default 0,
    `timeout`   int unsigned not null default 0,
    `pause`     varchar(255) not null default '',
    `script`    text         not null,
    `args`      varchar(512) not null default '',
    `tags`      varchar(255) not null default '',
    `create_at` bigint not null default 0,
    `create_by` varchar(64) not null default '',
    `update_at` bigint not null default 0,
    `update_by` varchar(64) not null default ''
);
CREATE INDEX `idx_task_tpl_group_id` ON `task_tpl` (`group_id` asc);

CREATE TABLE `task_tpl_host` (
    `ii`   integer primary key autoincrement,
    `id`   int unsigned not null,
    `host` varchar(128)  not null
);
CREATE INDEX `idx_task_tpl_host_id_host` ON `task_tpl_host` (`id`, `host` asc);

CREATE TABLE `task_record` (
    `id` integer primary key autoincrement,
    `event_id` bigint not null default 0,
    `group_id` bigint not null,
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
    `create_by` varchar(64) not null default ''
);
CREATE INDEX `idx_task_record_create_at_group_id` ON `task_record` (`create_at`, `group_id` asc);
CREATE INDEX `idx_task_record_create_by` ON `task_record` (`create_by` asc);
CREATE INDEX `idx_task_record_event_id` ON `task_record` (`event_id` asc);


CREATE TABLE `alerting_engines` (
    `id` integer primary key autoincrement,
    `instance` varchar(128) not null default '',
    `datasource_id` bigint not null default 0,
    `engine_cluster` varchar(128) not null default '',
    `clock` bigint not null
);

CREATE TABLE `datasource`
(
    `id` integer primary key autoincrement,
    `name` varchar(191) not null default '' unique,
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
    `is_default` tinyint not null default 0,
    `weight` int not null default 0,
    `created_at` bigint not null default 0,
    `created_by` varchar(64) not null default '',
    `updated_at` bigint not null default 0,
    `updated_by` varchar(64) not null default ''
);

CREATE UNIQUE INDEX idx_datasource_name ON datasource (name);

CREATE TABLE `builtin_cate` (
    `id` integer primary key autoincrement,
    `name` varchar(191) not null,
    `user_id` bigint not null default 0
);

CREATE TABLE `notify_tpl` (
    `id` integer primary key autoincrement,
    `channel` varchar(32) not null unique,
    `name` varchar(255) not null,
    `content` text not null,
    `create_at` bigint not null default 0,
    `create_by` varchar(64) not null default '',
    `update_at` bigint not null default 0,
    `update_by` varchar(64) not null default ''
);

CREATE UNIQUE INDEX idx_notify_tpl_channel ON notify_tpl (channel);

CREATE TABLE `sso_config` (
    `id` integer primary key autoincrement,
    `name` varchar(191) not null unique,
    `content` text not null,
    `update_at` bigint not null default 0
);

CREATE UNIQUE INDEX idx_sso_config_name ON sso_config (name);

CREATE TABLE `es_index_pattern` (
    `id` integer primary key autoincrement,
    `datasource_id` bigint not null default 0,
    `name` varchar(191) not null,
    `time_field` varchar(128) not null default '@timestamp',
    `allow_hide_system_indices` tinyint(1) not null default 0,
    `fields_format` varchar(4096) not null default '',
    `cross_cluster_enabled` int not null default 0,
    `create_at` bigint default '0',
    `create_by` varchar(64) default '',
    `update_at` bigint default '0',
    `update_by` varchar(64) default '',
    unique (`datasource_id`, `name`)
);

CREATE UNIQUE INDEX idx_es_index_pattern_datasource_id_name ON es_index_pattern (datasource_id, name);

CREATE TABLE `builtin_metrics` (
    `id` integer primary key autoincrement,
    `collector` varchar(191) NOT NULL,
    `typ` varchar(191) NOT NULL,
    `name` varchar(191) NOT NULL,
    `unit` varchar(191) NOT NULL,
    `lang` varchar(191) NOT NULL DEFAULT '',
    `note` varchar(4096) NOT NULL,
    `expression` varchar(4096) NOT NULL,
    `expression_type` varchar(32) NOT NULL DEFAULT 'promql',
    `metric_type` varchar(191) NOT NULL DEFAULT '',
    `extra_fields` text,
    `created_at` bigint NOT NULL DEFAULT 0,
    `created_by` varchar(191) NOT NULL DEFAULT '',
    `updated_at` bigint NOT NULL DEFAULT 0,
    `updated_by` varchar(191) NOT NULL DEFAULT '',
    `uuid integer` not null default 0
);

CREATE INDEX idx_collector ON builtin_metrics (collector);
CREATE INDEX idx_typ ON builtin_metrics (typ);
CREATE INDEX idx_builtinmetric_name ON builtin_metrics (name);
CREATE INDEX idx_lang ON builtin_metrics (lang);


CREATE TABLE `metric_filter` (
  `id` integer primary key autoincrement,
  `name` varchar(191) NOT NULL,
  `configs` varchar(4096) NOT NULL,
  `groups_perm` text,
  `create_at` bigint NOT NULL DEFAULT '0',
  `create_by` varchar(191) NOT NULL DEFAULT '',
  `update_at` bigint NOT NULL DEFAULT '0',
  `update_by` varchar(191) NOT NULL DEFAULT ''
);
CREATE INDEX `idx_metric_filter_name` ON `metric_filter` (`name` asc);

CREATE TABLE `target_busi_group` (
    `id` integer primary key autoincrement,
    `target_ident` varchar(191) not null,
    `group_id` integer not null,
    `update_at` integer not null
);

CREATE UNIQUE INDEX idx_target_busi_group ON target_busi_group (target_ident, group_id);


CREATE TABLE `dash_annotation` (
    `id` integer primary key autoincrement,
    `dashboard_id` bigint not null,
    `panel_id` varchar(191) not null,
    `tags` text,
    `description` text,
    `config` text,
    `time_start` bigint not null default 0,
    `time_end` bigint not null default 0,
    `create_at` bigint not null default 0,
    `create_by` varchar(64) not null default '',
    `update_at` bigint not null default 0,
    `update_by` varchar(64) not null default ''
);

CREATE TABLE `task_meta`
(
    `id`          integer primary key autoincrement,
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
    `created`     timestamp       not null default CURRENT_TIMESTAMP
);
CREATE INDEX `idx_task_meta_creator` ON `task_meta` (`creator` asc);
CREATE INDEX `idx_task_meta_created` ON `task_meta` (`created` asc);


/* start|cancel|kill|pause */
CREATE TABLE `task_action`
(
    `id`     integer primary key autoincrement,
    `action` varchar(32)     not null,
    `clock`  bigint          not null default 0
);

CREATE TABLE `task_scheduler`
(
    `id`        bigint unsigned not null,
    `scheduler` varchar(128)    not null default ''
);
CREATE INDEX `idx_task_scheduler_id_scheduler` ON `task_scheduler` (`id`, `scheduler` asc);

CREATE TABLE `task_scheduler_health`
(
    `scheduler` varchar(128) not null unique,
    `clock`     bigint       not null
);
CREATE INDEX `idx_task_scheduler_health_clock` ON `task_scheduler_health` (`clock` asc);

CREATE TABLE `task_host_doing`
(
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `clock`  bigint          not null default 0,
    `action` varchar(16)     not null
);
CREATE INDEX `idx_task_host_doing_id` ON `task_host_doing` (`id` asc);
CREATE INDEX `idx_task_host_doing_host` ON `task_host_doing` (`host` asc);

CREATE TABLE task_host_0
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_1
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_2
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_3
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_4
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_5
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_6
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_7
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_8
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_9
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_10
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_11
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_12
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_13
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_14
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_15
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_16
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_17
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_18
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_19
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_20
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_21
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_22
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_23
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_24
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_25
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_26
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_27
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_28
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_29
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_30
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_31
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_32
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_33
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_34
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_35
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_36
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_37
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_38
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_39
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_40
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_41
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_42
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_43
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_44
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_45
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_46
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_47
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_48
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_49
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_50
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_51
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_52
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_53
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_54
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_55
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_56
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_57
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_58
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_59
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_60
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_61
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_62
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_63
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_64
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_65
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_66
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_67
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_68
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_69
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_70
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_71
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_72
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_73
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_74
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_75
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_76
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_77
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_78
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_79
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_80
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_81
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_82
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_83
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_84
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_85
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_86
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_87
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_88
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_89
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_90
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_91
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_92
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_93
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_94
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_95
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_96
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_97
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_98
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);

CREATE TABLE task_host_99
(
    `ii`     integer primary key autoincrement,
    `id`     bigint unsigned not null,
    `host`   varchar(128)    not null,
    `status` varchar(32)     not null,
    `stdout` text,
    `stderr` text,
    unique (`id`, `host`)
);
