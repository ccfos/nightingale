set names utf8;

drop database if exists n9e_mon;
create database n9e_mon;
use n9e_mon;

CREATE TABLE `node` (
  `id` int unsigned not null AUTO_INCREMENT,
  `pid` int unsigned not null,
  `name` varchar(64) not null,
  `path` varchar(255) not null,
  `leaf` int(1) not null,
  `note` varchar(128) not null default '',
  PRIMARY KEY (`id`),
  KEY (`path`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

CREATE TABLE `endpoint` (
  `id` int unsigned not null AUTO_INCREMENT,
  `ident` varchar(255) not null,
  `alias` varchar(255) not null default '',
  PRIMARY KEY (`id`),
  UNIQUE KEY (`ident`),
  KEY (`alias`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

CREATE TABLE `node_endpoint` (
  `node_id` int unsigned not null,
  `endpoint_id` int unsigned not null,
  KEY(`node_id`),
  KEY(`endpoint_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

create table `maskconf` (
  `id` int unsigned not null auto_increment,
  `nid` int unsigned not null,
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
  `enable_stime` varchar(6)  NOT NULL DEFAULT '00:00' COMMENT '策略生效开始时间',
  `enable_etime` varchar(6)  NOT NULL DEFAULT '23:59' COMMENT '策略生效终止时间',
  `enable_days_of_week` varchar(1024) NOT NULL DEFAULT '[0,1,2,3,4,5,6]' COMMENT '策略生效日期',
  `converge` varchar(45) NOT NULL DEFAULT '' COMMENT 'n秒最多报m次警',
  `recovery_notify` int(1) NOT NULL DEFAULT 1 COMMENT '1 发送恢复通知 0不发送恢复通知',
  `priority` int(1) NOT NULL DEFAULT 3 COMMENT '告警等级',
  `notify_group` varchar(255) NOT NULL DEFAULT '' COMMENT '告警通知组',
  `notify_user` varchar(255) NOT NULL DEFAULT '' COMMENT '告警通知人',
  `callback` varchar(1024) NOT NULL DEFAULT '' COMMENT 'callback url',
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
  `creator` varchar(255) NOT NULL DEFAULT '1971-01-01 00:00:00' COMMENT 'creator',
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
  `last_updator` varchar(255) NOT NULL DEFAULT '' COMMENT 'last_updator',
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
  `last_updator` varchar(255) NOT NULL DEFAULT '' COMMENT 'last_updator',
  `last_updated` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_nid` (`nid`),
  KEY `idx_collect_type` (`collect_type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8 COMMENT 'proc collect';

CREATE TABLE `log_collect` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT 'id',
  `nid` bigint(20) unsigned NOT NULL DEFAULT '0' COMMENT 'nid',
  `name` varchar(255) NOT NULL DEFAULT '' COMMENT 'name',
  `tags` varchar(255) NOT NULL DEFAULT '' COMMENT 'tags',
  `collect_type` varchar(64) NOT NULL DEFAULT 'PROC' COMMENT 'type',
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
  `last_updator` varchar(255) NOT NULL DEFAULT '' COMMENT 'last_updator',
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

CREATE TABLE `collect_hist` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT 'id',
  `cid` bigint(20) NOT NULL DEFAULT '0' COMMENT 'collect id',
  `collect_type` varchar(255) NOT NULL DEFAULT '' COMMENT '采集的种类 log,port,proc,plugin',
  `action` varchar(255) NOT NULL DEFAULT '' COMMENT '动作 update, delete',
  `body` text COMMENT '修改之前采集的内容',
  `creator` varchar(255) NOT NULL DEFAULT '1971-01-01 00:00:00' COMMENT 'creator',
  `created` datetime NOT NULL DEFAULT '1971-01-01 00:00:00' COMMENT 'created',
  PRIMARY KEY (`id`),
  KEY `idx_cid` (`cid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8 COMMENT 'hist';