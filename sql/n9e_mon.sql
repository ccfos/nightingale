set names utf8;

drop database if exists n9e_mon;
create database n9e_mon;
use n9e_mon;

create table `maskconf` (
  `id` int unsigned not null auto_increment,
  `nid` int unsigned not null,
  `category` int(1) NOT NULL COMMENT '1 机器 2业务',
  `metric` varchar(255) not null,
  `tags` varchar(255) not null default '',
  `cause` varchar(255) not null default '',
  `user` varchar(32) not null default 'operate user',
  `btime` bigint not null default 0 comment 'begin time',
  `etime` bigint not null default 0 comment 'end time',
  primary key (`id`),
  key(`nid`)
) engine=innodb default charset=utf8;

create table `maskconf_endpoints` (
  `id` int unsigned not null auto_increment,
  `mask_id` int unsigned not null,
  `endpoint` varchar(255) not null,
  primary key (`id`),
  key(`mask_id`)
) engine=innodb default charset=utf8;

create table `maskconf_nids` (
  `id` int unsigned not null auto_increment,
  `mask_id` int unsigned not null,
  `nid` varchar(255) not null,
  `path` varchar(255) not null,
  primary key (`id`),
  key(`mask_id`)
) engine=innodb default charset=utf8;

create table `screen` (
  `id` int unsigned not null auto_increment,
  `node_id` int unsigned not null comment 'service tree node id',
  `name` varchar(255) not null,
  `last_updator` varchar(64) not null default '',
  `last_updated` timestamp not null default CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  primary key (`id`),
  key(`node_id`)
) engine=innodb default charset=utf8;

create table `screen_subclass` (
  `id` int unsigned not null auto_increment,
  `screen_id` int unsigned not null,
  `name` varchar(255) not null,
  `weight` int not null default 0,
  primary key (`id`),
  key(`screen_id`)
) engine=innodb default charset=utf8;

create table `chart` (
  `id` int unsigned not null auto_increment,
  `subclass_id` int unsigned not null,
  `configs` varchar(8192),
  `weight` int not null default 0,
  primary key (`id`),
  key(`subclass_id`)
) engine=innodb default charset=utf8;

create table `tmp_chart` (
  `id` int unsigned not null auto_increment,
  `configs` varchar(8192),
  `creator` varchar(64) not null,
  `last_updated` timestamp not null default CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  primary key (`id`)
) engine=innodb default charset=utf8;

create table `event_cur` (
  `id` bigint(20) unsigned not null AUTO_INCREMENT comment 'id',
  `sid` bigint(20) unsigned not null default 0 comment 'sid',
  `sname` varchar(255) not null default '' comment 'name, 报警通知名称',
  `node_path` varchar(255) not null default '' comment 'node path',
  `nid` int unsigned not null default '0' comment 'node id',
  `endpoint` varchar(255) not null default '' comment 'endpoint',
  `endpoint_alias` varchar(255) not null default '' comment 'endpoint alias',
  `cur_node_path` varchar(255) not null default '' comment 'cur_node_path',
  `cur_nid` varchar(45) not null default '' comment 'cur_nid',
  `priority` tinyint(4) not null default 2 comment '优先级',
  `event_type` varchar(45) not null default '' comment 'alert|recovery',
  `category` tinyint(4) not null default 2 comment '1阈值 2智能',
  `status` int(10) not null default 0 comment 'event status',
  `detail` text comment 'counter points pred_points 详情',
  `hashid` varchar(128) not null default '' comment 'sid+counter hash',
  `etime` bigint(20) not null default 0 comment 'event ts',
  `value` varchar(255) not null default '' comment '当前值',
  `users` varchar(512) not null default '[]' comment 'notify users',
  `groups` varchar(512) not null default '[]' comment 'notify groups',
  `runbook` varchar(1024) NOT NULL DEFAULT '' COMMENT 'runbook url',
  `info` varchar(512) not null default '' comment 'strategy info',
  `ignore_alert` int(2) not null default 0 comment 'ignore event',
  `claimants` varchar(512)  not null default '[]' comment 'claimants',
  `need_upgrade` int(2)  not null default 0 comment 'need upgrade',
  `alert_upgrade` text comment 'alert upgrade',
  `created` DATETIME not null default '1971-1-1 00:00:00' comment 'created',
  KEY `idx_id` (`id`),
  KEY `idx_sid` (`sid`),
  KEY `idx_hashid` (`hashid`),
  KEY `idx_node_path` (`node_path`),
  KEY `idx_etime` (`etime`)
) engine=innodb default charset=utf8 comment 'event';

create table `event` (
  `id` bigint(20) unsigned not null AUTO_INCREMENT comment 'id',
  `sid` bigint(20) unsigned not null default 0 comment 'sid',
  `sname` varchar(255) not null default '' comment 'name, 报警通知名称',
  `node_path` varchar(255) not null default '' comment 'node path',
  `nid` int unsigned not null default '0' comment 'node id',
  `endpoint` varchar(255) not null default '' comment 'endpoint',
  `endpoint_alias` varchar(255) not null default '' comment 'endpoint alias',
  `cur_node_path` varchar(255) not null default '' comment 'cur_node_path',
  `cur_nid` varchar(45) not null default '' comment 'cur_nid',
  `priority` tinyint(4) not null default 2 comment '优先级',
  `event_type` varchar(45) not null default '' comment 'alert|recovery',
  `category` tinyint(4) not null default 2 comment '1阈值 2智能',
  `status` int(10) not null default 0 comment 'event status',
  `detail` text comment 'counter points pred_points 详情',
  `hashid` varchar(128) not null default '' comment 'sid+counter hash',
  `etime` bigint(20) not null default 0 comment 'event ts',
  `value` varchar(255) not null default '' comment '当前值',
  `users` varchar(512) not null default '[]' comment 'notify users',
  `groups` varchar(512) not null default '[]' comment 'notify groups',
  `runbook` varchar(1024) NOT NULL DEFAULT '' COMMENT 'runbook url',
  `info` varchar(512) not null default '' comment 'strategy info',
  `need_upgrade` int(2)  not null default 0 comment 'need upgrade',
  `alert_upgrade` text not null comment 'alert upgrade',
  `created` DATETIME not null default '1971-1-1 00:00:00' comment 'created',
  PRIMARY KEY (`id`),
  KEY `idx_id` (`id`),
  KEY `idx_sid` (`sid`),
  KEY `idx_hashid` (`hashid`),
  KEY `idx_node_path` (`node_path`),
  KEY `idx_etime` (`etime`),
  KEY `idx_event_type` (`event_type`),
  KEY `idx_status` (`status`)
) engine=innodb default charset=utf8 comment 'event';

CREATE TABLE `stra` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT,
  `name` varchar(255) NOT NULL COMMENT 'strategy name',
  `category` int(1) NOT NULL COMMENT '1 机器 2业务',
  `nid` int(10) NOT NULL COMMENT '服务树节点id',
  `excl_nid` varchar(255) NOT NULL COMMENT '被排除的服务树叶子节点id',
  `alert_dur` int(4) NOT NULL COMMENT '单位秒，持续异常n秒则产生异常event',
  `recovery_dur` int(4) NOT NULL DEFAULT 0 COMMENT '单位秒，持续正常n秒则产生恢复event，0表示立即产生恢复event',
  `exprs` varchar(1024) NOT NULL DEFAULT '' COMMENT '规则表达式',
  `tags` varchar(1024) DEFAULT '' COMMENT 'tags过滤',
  `enable_stime` char(5)  NOT NULL DEFAULT '00:00' COMMENT '策略生效开始时间',
  `enable_etime` char(5)  NOT NULL DEFAULT '23:59' COMMENT '策略生效终止时间',
  `enable_days_of_week` varchar(1024) NOT NULL DEFAULT '[0,1,2,3,4,5,6]' COMMENT '策略生效日期',
  `converge` varchar(45) NOT NULL DEFAULT '' COMMENT 'n秒最多报m次警',
  `recovery_notify` int(1) NOT NULL DEFAULT 1 COMMENT '1 发送恢复通知 0不发送恢复通知',
  `priority` int(1) NOT NULL DEFAULT 3 COMMENT '告警等级',
  `notify_group` varchar(255) NOT NULL DEFAULT '' COMMENT '告警通知组',
  `notify_user` varchar(255) NOT NULL DEFAULT '' COMMENT '告警通知人',
  `callback` varchar(1024) NOT NULL DEFAULT '' COMMENT 'callback url',
  `runbook` varchar(1024) NOT NULL DEFAULT '' COMMENT 'runbook url',
  `work_groups` varchar(255) NOT NULL DEFAULT '' COMMENT 'work_groups',
  `creator` varchar(64) NOT NULL COMMENT '创建者',
  `created` timestamp NOT NULL DEFAULT '1971-01-01 00:00:00' COMMENT 'created',
  `last_updator` varchar(64) NOT NULL DEFAULT '',
  `last_updated` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `need_upgrade` int(2)  not null default 0 comment 'need upgrade',
  `alert_upgrade` text comment 'alert upgrade',
  PRIMARY KEY (`id`),
  KEY `idx_nid` (`nid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

CREATE TABLE `stra_log` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT 'id',
  `sid` bigint(20) NOT NULL DEFAULT '0' COMMENT 'collect id',
  `action` varchar(255) NOT NULL DEFAULT '' COMMENT '动作 update, delete',
  `body` text COMMENT '修改之前采集的内容',
  `creator` varchar(255) NOT NULL DEFAULT '' COMMENT 'creator',
  `created` timestamp NOT NULL DEFAULT '1971-01-01 00:00:00' COMMENT 'created',
  PRIMARY KEY (`id`),
  KEY `idx_sid` (`sid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

CREATE TABLE `port_collect` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT 'id',
  `collect_type` varchar(64) NOT NULL DEFAULT 'PORT' COMMENT 'type',
  `nid` int(10) NOT NULL COMMENT '服务树节点id',
  `name` varchar(255) NOT NULL DEFAULT '' COMMENT 'name',
  `tags` varchar(255) NOT NULL DEFAULT '' COMMENT 'tags',
  `port` int(11) NOT NULL DEFAULT '0' COMMENT 'port',
  `step` int(11) NOT NULL DEFAULT '0' COMMENT '采集周期',
  `timeout` int(11) NOT NULL DEFAULT '0' COMMENT 'connect time',
  `comment` varchar(512) NOT NULL DEFAULT '' COMMENT 'comment',
  `creator` varchar(255) NOT NULL DEFAULT '' COMMENT 'creator',
  `created` datetime NOT NULL COMMENT 'created',
  `last_updator` varchar(128) NOT NULL DEFAULT '' COMMENT 'last_updator',
  `last_updated` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT 'last_updated',
  PRIMARY KEY (`id`),
  KEY `idx_nid` (`nid`),
  KEY `idx_collect_type` (`collect_type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8 COMMENT 'port collect';

CREATE TABLE `proc_collect` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT 'id',
  `nid` bigint(20) unsigned NOT NULL DEFAULT '0' COMMENT 'nid',
  `name` varchar(255) NOT NULL DEFAULT '' COMMENT 'name',
  `tags` varchar(255) NOT NULL DEFAULT '' COMMENT 'tags',
  `collect_type` varchar(64) NOT NULL DEFAULT 'PROC' COMMENT 'type',
  `collect_method` varchar(64) NOT NULL DEFAULT 'name' COMMENT '采集方式',
  `target` varchar(255) NOT NULL DEFAULT '' COMMENT '采集对象',
  `step` int(11) NOT NULL DEFAULT '0' COMMENT '采集周期',
  `comment` varchar(512) NOT NULL DEFAULT '' COMMENT 'comment',
  `creator` varchar(255) NOT NULL DEFAULT '' COMMENT 'creator',
  `created` datetime NOT NULL  COMMENT 'created',
  `last_updator` varchar(128) NOT NULL DEFAULT '' COMMENT 'last_updator',
  `last_updated` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_nid` (`nid`),
  KEY `idx_collect_type` (`collect_type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8 COMMENT 'proc collect';

CREATE TABLE `log_collect` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT 'id',
  `nid` bigint(20) unsigned NOT NULL DEFAULT '0' COMMENT 'nid',
  `name` varchar(255) NOT NULL DEFAULT '' COMMENT 'name',
  `tags` varchar(2048) NOT NULL DEFAULT '' COMMENT 'tags',
  `collect_type` varchar(64) NOT NULL DEFAULT 'LOG' COMMENT 'type',
  `step` int(11) NOT NULL DEFAULT '0' COMMENT '采集周期',
  `file_path` varchar(255) NOT NULL DEFAULT '' COMMENT 'file path',
  `time_format` varchar(128) NOT NULL DEFAULT '' COMMENT 'time format',
  `pattern` varchar(1024) NOT NULL DEFAULT '' COMMENT 'pattern',
  `func` varchar(64) NOT NULL DEFAULT '' COMMENT 'func',
  `degree` tinyint(4) NOT NULL DEFAULT '0' COMMENT 'degree',
  `func_type` varchar(64) NOT NULL DEFAULT '' COMMENT 'func_type',
  `aggregate` varchar(64) NOT NULL DEFAULT '' COMMENT 'aggr',
  `unit` varchar(64) NOT NULL DEFAULT '' COMMENT 'unit',
  `zero_fill` tinyint(4) NOT NULL DEFAULT '0' COMMENT 'zero fill',
  `comment` varchar(512) NOT NULL DEFAULT '' COMMENT 'comment',
  `creator` varchar(255) NOT NULL DEFAULT '' COMMENT 'creator',
  `created` datetime NOT NULL  COMMENT 'created',
  `last_updator` varchar(128) NOT NULL DEFAULT '' COMMENT 'last_updator',
  `last_updated` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_nid` (`nid`),
  KEY `idx_collect_type` (`collect_type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8 COMMENT 'log collect';

CREATE TABLE `plugin_collect` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT 'id',
  `nid` bigint(20) unsigned NOT NULL DEFAULT '0' COMMENT 'nid',
  `name` varchar(255) NOT NULL DEFAULT '' COMMENT 'name',
  `collect_type` varchar(64) NOT NULL DEFAULT 'PROC' COMMENT 'type',
  `step` int(11) NOT NULL DEFAULT '0' COMMENT '采集周期',
  `file_path` varchar(255) NOT NULL COMMENT 'file_path',
  `params` varchar(255) NOT NULL COMMENT 'params',
  `stdin` text NOT NULL COMMENT 'stdin',
  `env` text NOT NULL COMMENT 'env',
  `comment` varchar(512) NOT NULL DEFAULT '' COMMENT 'comment',
  `creator` varchar(255) NOT NULL DEFAULT '' COMMENT 'creator',
  `created` datetime NOT NULL  COMMENT 'created',
  `last_updator` varchar(128) NOT NULL DEFAULT '' COMMENT 'last_updator',
  `last_updated` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_nid` (`nid`),
  KEY `idx_collect_type` (`collect_type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8 COMMENT 'plugin collect';

CREATE TABLE `api_collect` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT 'id',
  `nid` bigint(20) unsigned NOT NULL DEFAULT '0' COMMENT 'nid',
  `name` varchar(255) NOT NULL DEFAULT '' COMMENT 'name',
  `domain` varchar(255) NOT NULL DEFAULT '' COMMENT 'domain',
  `collect_type` varchar(64) NOT NULL DEFAULT 'API' COMMENT 'type',
  `path` varchar(255) NOT NULL DEFAULT '/' COMMENT 'path&querystring',
  `header` varchar(1024) NOT NULL DEFAULT '' COMMENT 'headers',
  `step` int(11) NOT NULL DEFAULT '0' COMMENT 'step',
  `timeout` int(11) NOT NULL DEFAULT '0' COMMENT 'total timeout',
  `protocol` varchar(20) NOT NULL DEFAULT 'http' COMMENT 'protocol',
  `port` varchar(20) NOT NULL DEFAULT '0' COMMENT 'port',
  `method` varchar(10) NOT NULL DEFAULT 'get' COMMENT 'method',
  `max_redirect` smallint(2) NOT NULL DEFAULT '0' COMMENT 'max_redirect',
  `post_body` text COMMENT 'post_body',
  `expected_code` varchar(255) NOT NULL DEFAULT '[]' COMMENT 'expected_code',
  `expected_string` varchar(255) NOT NULL DEFAULT '' COMMENT 'expected_string',
  `unexpected_string` varchar(255) NOT NULL DEFAULT '' COMMENT 'unexpected_string',
  `region` varchar(32) NOT NULL DEFAULT 'default' COMMENT 'region',
  `comment` varchar(512) NOT NULL DEFAULT '' COMMENT 'comment',
  `creator` varchar(255) NOT NULL DEFAULT '' COMMENT 'creator',
  `created` datetime NOT NULL  COMMENT 'created',
  `last_updator` varchar(128) NOT NULL DEFAULT '' COMMENT 'last_updator',
  `last_updated` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_nid` (`nid`),
  KEY `idx_collect_type` (`collect_type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8 COMMENT 'api collect';

CREATE TABLE `snmp_collect` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT 'id',
  `nid` bigint(20) unsigned NOT NULL DEFAULT '0' COMMENT 'nid',
  `collect_type` varchar(64) NOT NULL DEFAULT 'SNMP' COMMENT 'collect_type',
  `oid_type` int(1) NOT NULL DEFAULT 1 COMMENT 'oid_type',
  `module` varchar(255) NOT NULL DEFAULT '' COMMENT 'module',
  `metric` varchar(255) NOT NULL DEFAULT '' COMMENT 'metric',
  `metric_type` varchar(255) NOT NULL DEFAULT '' COMMENT 'metric_type',
  `oid` varchar(255) NOT NULL DEFAULT '' COMMENT 'oid',
  `indexes` text NOT NULL COMMENT 'indexes',
  `port` int(5) NOT NULL DEFAULT 161 COMMENT 'port',
  `step` int(11) NOT NULL DEFAULT '0' COMMENT 'step',
  `timeout` int(11) NOT NULL DEFAULT '0' COMMENT 'total timeout',
  `comment` varchar(512) NOT NULL DEFAULT '' COMMENT 'comment',
  `creator` varchar(255) NOT NULL DEFAULT '' COMMENT 'creator',
  `created` datetime NOT NULL  COMMENT 'created',
  `last_updator` varchar(128) NOT NULL DEFAULT '' COMMENT 'last_updator',
  `last_updated` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_nid` (`nid`),
  KEY `idx_module` (`module`),
  KEY `idx_collect_type` (`collect_type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8 COMMENT 'api collect';

CREATE TABLE `aggr_calc` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT 'id',
  `nid` bigint(20) unsigned NOT NULL DEFAULT '0' COMMENT 'nid',
  `category` int(1) NOT NULL COMMENT '1 机器 2业务',
  `new_metric` varchar(255) NOT NULL DEFAULT '' COMMENT 'new_metric',
  `new_step` int(11) NOT NULL DEFAULT '0' COMMENT 'new_step',
  `groupby` varchar(255) NOT NULL DEFAULT '' COMMENT 'groupby',
  `raw_metrics` text comment 'raw_metrics',
  `global_operator` varchar(32) NOT NULL DEFAULT '' COMMENT 'global_operator',
  `expression` varchar(255) NOT NULL DEFAULT '' COMMENT 'expression',
  `rpn` varchar(255) NOT NULL DEFAULT '' COMMENT 'rpn',
  `status` int(1) NOT NULL COMMENT '',
  `quota` int(10) NOT NULL COMMENT '',
  `comment` varchar(255) NOT NULL DEFAULT '' COMMENT 'comment',
  `creator` varchar(64) NOT NULL COMMENT '创建者',
  `created` timestamp NOT NULL DEFAULT '1971-01-01 00:00:00' COMMENT 'created',
  `last_updator` varchar(64) NOT NULL DEFAULT '',
  `last_updated` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_nid` (`nid`),
  KEY `idx_new_metric` (`new_metric`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8 COMMENT 'aggr_calc';

CREATE TABLE `nginx_log_stra` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT 'id',
  `nid` bigint(20) unsigned NOT NULL DEFAULT '0' COMMENT 'nid',
  `service` varchar(255) NOT NULL DEFAULT '' COMMENT 'service',
  `interval` int(11) NOT NULL DEFAULT '0' COMMENT 'interval',
  `domain` varchar(2048) NOT NULL DEFAULT '' COMMENT 'domain',
  `url_path_prefix` varchar(2048) NOT NULL DEFAULT '' COMMENT 'url_path_prefix',
  `append_tags` varchar(2048) NOT NULL DEFAULT '' COMMENT 'append_tags',
  `creator` varchar(64) NOT NULL COMMENT '创建者',
  `created` timestamp NOT NULL DEFAULT '1971-01-01 00:00:00' COMMENT 'created',
  `last_updator` varchar(64) NOT NULL DEFAULT '',
  `last_updated` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_nid` (`nid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8 COMMENT 'nginx_log_stra';

CREATE TABLE `binlog_stra` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT 'id',
  `nid` bigint(20) unsigned NOT NULL DEFAULT '0' COMMENT 'nid',
  `metric` varchar(255) NOT NULL DEFAULT '' COMMENT 'metric',
  `interval` int(11) NOT NULL DEFAULT '0' COMMENT 'interval',
  `db` varchar(2048) NOT NULL DEFAULT '' COMMENT 'db',
  `column_change` varchar(2048) NOT NULL DEFAULT '' COMMENT 'column_change',
  `tags_column` varchar(2048) NOT NULL DEFAULT '' COMMENT 'tags_column',
  `append_tags` varchar(2048) NOT NULL DEFAULT '' COMMENT 'append_tags',
  `func` varchar(255) NOT NULL DEFAULT '' COMMENT 'func',
  `sql_type` varchar(255) NOT NULL DEFAULT '' COMMENT 'sql_type',
  `value_column` varchar(255) NOT NULL DEFAULT '' COMMENT 'value_column',
  `creator` varchar(64) NOT NULL COMMENT '创建者',
  `created` timestamp NOT NULL DEFAULT '1971-01-01 00:00:00' COMMENT 'created',
  `last_updator` varchar(64) NOT NULL DEFAULT '',
  `last_updated` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_nid` (`nid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8 COMMENT 'binlog_stra';

CREATE TABLE `collect_hist` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT 'id',
  `cid` bigint(20) unsigned NOT NULL DEFAULT '0' COMMENT 'collect id',
  `collect_type` varchar(255) NOT NULL DEFAULT '' COMMENT '采集的种类 log,port,proc,api',
  `action` varchar(8) NOT NULL DEFAULT '' COMMENT '动作 update, delete',
  `body` text COMMENT '修改之前采集的内容',
  `creator` varchar(128) NOT NULL DEFAULT '' COMMENT 'creator',
  `created` datetime NOT NULL DEFAULT '1971-01-01 00:00:00' COMMENT 'created',
  PRIMARY KEY (`id`),
  KEY `idx_cid` (`cid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8 COMMENT 'hist';


CREATE TABLE `api_collect_sid` (
  `id`  bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT 'id',
  `sid` bigint(20) NOT NULL DEFAULT '0' COMMENT 'stra id',
  `cid` bigint(20) NOT NULL DEFAULT '0' COMMENT 'collect id',
  PRIMARY KEY (`id`),
  KEY  (`sid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
