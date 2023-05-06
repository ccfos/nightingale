use n9e_v5;

insert into `role_operation`(role_name, operation) values('Guest', '/log/explorer');
insert into `role_operation`(role_name, operation) values('Guest', '/trace/explorer');

insert into `role_operation`(role_name, operation) values('Standard', '/log/explorer');
insert into `role_operation`(role_name, operation) values('Standard', '/trace/explorer');
insert into `role_operation`(role_name, operation) values('Standard', '/alert-rules-built-in');
insert into `role_operation`(role_name, operation) values('Standard', '/dashboards-built-in');
insert into `role_operation`(role_name, operation) values('Standard', '/trace/dependencies');

alter table `board` add built_in tinyint(1) not null default 0 comment '0:false 1:true';
alter table `board` add hide tinyint(1) not null default 0 comment '0:false 1:true';

alter table `chart_share` add datasource_id bigint unsigned not null default 0;
alter table `chart_share` drop dashboard_id;

alter table `alert_rule` add datasource_ids varchar(255) not null default '';
alter table `alert_rule` add rule_config text not null comment 'rule_config';
alter table `alert_rule` add annotations text not null comment 'annotations';

alter table `alert_mute` add datasource_ids varchar(255) not null default '';
alter table `alert_mute` add periodic_mutes varchar(4096) not null default '[]';
alter table `alert_mute` add mute_time_type tinyint(1) not null default 0;

alter table `alert_subscribe` add datasource_ids varchar(255) not null default '';
alter table `alert_subscribe` add prod varchar(255) not null default '';
alter table `alert_subscribe` add webhooks text;
alter table `alert_subscribe` add redefine_webhooks tinyint(1) default 0;
alter table `alert_subscribe` add for_duration bigint not null default 0;

alter table `recording_rule` add datasource_ids varchar(255) default '';

alter table `target` modify cluster varchar(128) not null default '';

alter table `alert_cur_event` add datasource_id bigint unsigned not null default 0;
alter table `alert_cur_event` add annotations text not null comment 'annotations';
alter table `alert_cur_event` add rule_config text not null comment 'rule_config';

alter table `alert_his_event` add datasource_id bigint unsigned not null default 0;
alter table `alert_his_event` add annotations text not null comment 'annotations';
alter table `alert_his_event` add rule_config text not null comment 'rule_config';

alter table `alerting_engines` add datasource_id bigint unsigned not null default 0;
alter table `alerting_engines` change cluster engine_cluster varchar(128) not null default '' comment 'n9e engine cluster';

alter table `task_record` add event_id bigint not null comment 'event id' default 0;

CREATE TABLE `datasource`
(
    `id` int unsigned NOT NULL AUTO_INCREMENT,
    `name` varchar(255) not null default '',
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
    `created_at` bigint not null default 0,
    `created_by` varchar(64) not null default '',
    `updated_at` bigint not null default 0,
    `updated_by` varchar(64) not null default '',
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
    PRIMARY KEY (`id`),
    UNIQUE KEY (`channel`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

CREATE TABLE `sso_config` (
    `id` bigint unsigned not null auto_increment,
    `name` varchar(191) not null,
    `content` text not null,
    PRIMARY KEY (`id`),
    UNIQUE KEY (`name`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;