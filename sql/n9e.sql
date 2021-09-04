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
    `roles` varchar(255) not null comment 'Admin | Standard | Guest',
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
insert into `role_operation`(role_name, operation) values('Standard', 'alert_event_modify');
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
insert into classpath(id, path, note, preset, create_by, update_by, create_at, update_at) values(1, 'all.resources', 'preset classpath, all resources belong to', 1, 'system', 'system', unix_timestamp(now()), unix_timestamp(now()));

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
    `classpath_prefix` varchar(255) not null default '' comment 'classpath prefix',
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
    `tags` varchar(1024) not null default '' comment 'merge data_tags rule_tags and res_tags',
    PRIMARY KEY (`id`),
    KEY (`hash_id`),
    KEY (`rule_id`),
    KEY (`trigger_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

CREATE TABLE `history_alert_event` (
  `id` bigint unsigned not null AUTO_INCREMENT,
  `hash_id` varchar(255) not null COMMENT 'rule_id + point_pk',
  `rule_id` bigint unsigned not null,
  `rule_name` varchar(255) not null,
  `rule_note` varchar(512) not null default 'alert rule note',
  `res_classpaths` varchar(1024) not null default '' COMMENT 'belong classpaths',
  `priority` tinyint(1) not null,
  `status` tinyint(1) not null,
  `is_prome_pull` tinyint(1) not null,
  `is_recovery` tinyint(1) not null,
  `history_points` text COMMENT 'metric, history points',
  `trigger_time` bigint not null,
  `notify_channels` varchar(255) not null default '',
  `notify_groups` varchar(255) not null default '',
  `notify_users` varchar(255) not null default '',
  `runbook_url` varchar(255) default NULL,
  `readable_expression` varchar(1024) not null COMMENT 'e.g. mem.bytes.used.percent(all,60s) > 0',
  `tags` varchar(1024) not null default '' COMMENT 'merge data_tags rule_tags and res_tags',
  PRIMARY KEY (`id`),
  KEY `hash_id` (`hash_id`),
  KEY `rule_id` (`rule_id`),
  KEY `trigger_time` (`trigger_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

CREATE TABLE `metric_description` (
    `id` bigint unsigned not null auto_increment,
    `metric` varchar(255) not null default '',
    `description` varchar(255) not null default '',
    PRIMARY KEY (`id`),
    UNIQUE KEY (`metric`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

insert into metric_description(metric, description) values('system_ntp_offset', '系统时间偏移量');
insert into metric_description(metric, description) values('system_proc_count', '系统进程个数');
insert into metric_description(metric, description) values('system_uptime', '系统运行的时间');
insert into metric_description(metric, description) values('system_cpu_util', '总体CPU使用率(单位：%)');
insert into metric_description(metric, description) values('system_cpu_switches', 'cpu上下文交换次数');
insert into metric_description(metric, description) values('system_cpu_guest', '虚拟处理器CPU时间占比(单位：%)');
insert into metric_description(metric, description) values('system_cpu_idle', '总体CPU空闲率(单位：%)');
insert into metric_description(metric, description) values('system_cpu_iowait', '等待I/O的CPU时间占比(单位：%)');
insert into metric_description(metric, description) values('system_cpu_num_cores', 'CPU核心数');
insert into metric_description(metric, description) values('system_cpu_steal', '等待处理其他虚拟核的时间占比(单位：%)');
insert into metric_description(metric, description) values('system_cpu_system', '内核态CPU时间占比(单位：%)');
insert into metric_description(metric, description) values('system_cpu_user', '用户态CPU时间占比(单位：%)');
insert into metric_description(metric, description) values('system_disk_bytes_free', '磁盘某分区余量大小（单位：byte）');
insert into metric_description(metric, description) values('system_disk_used_percent', '磁盘某分区用量占比（单位：%）');
insert into metric_description(metric, description) values('system_disk_read_time', '设备读操作耗时(单位：ms)');
insert into metric_description(metric, description) values('system_disk_read_time_percent', '读取磁盘时间百分比（单位：%）');
insert into metric_description(metric, description) values('system_disk_bytes_total', '磁盘某分区总量（单位：byte）');
insert into metric_description(metric, description) values('system_disk_bytes_used', '磁盘某分区用量大小（单位：byte）');
insert into metric_description(metric, description) values('system_disk_write_time', '设备写操作耗时(单位：ms)');
insert into metric_description(metric, description) values('system_disk_write_time_percent', '写入磁盘时间百分比（单位：%）');
insert into metric_description(metric, description) values('system_files_allocated', '系统已分配文件句柄数');
insert into metric_description(metric, description) values('system_files_left', '系统未分配文件句柄数');
insert into metric_description(metric, description) values('system_files_used_percent', '系统使用文件句柄占已分配百分比（单位：%）');
insert into metric_description(metric, description) values('system_files_max', '系统可以打开的最大文件句柄数');
insert into metric_description(metric, description) values('system_files_used', '系统使用的已分配文件句柄数');
insert into metric_description(metric, description) values('system_disk_inodes_free', '某分区空闲inode数量');
insert into metric_description(metric, description) values('system_disk_inodes_used_percent', '某分区已用inode占比（单位：%）');
insert into metric_description(metric, description) values('system_disk_inodes_total', '某分区inode总数量');
insert into metric_description(metric, description) values('system_disk_inodes_used', '某分区已用inode数量');
insert into metric_description(metric, description) values('system_io_avgqu_sz', '设备平均队列长度');
insert into metric_description(metric, description) values('system_io_avgrq_sz', '设备平均请求大小');
insert into metric_description(metric, description) values('system_io_await', '每次IO平均处理时间（单位：ms）');
insert into metric_description(metric, description) values('system_io_r_await', '读请求平均耗时(单位：ms)');
insert into metric_description(metric, description) values('system_io_read_request', '每秒读请求数量');
insert into metric_description(metric, description) values('system_io_read_bytes', '每秒读取字节数');
insert into metric_description(metric, description) values('system_io_rrqm_s', '每秒合并到设备队列的读请求数');
insert into metric_description(metric, description) values('system_io_svctm', '每次IO平均服务时间（单位：ms）');
insert into metric_description(metric, description) values('system_io_util', 'I/O请求的CPU时间百分比');
insert into metric_description(metric, description) values('system_io_w_await', '写请求平均耗时(单位：ms)');
insert into metric_description(metric, description) values('system_io_write_request', '每秒写请求数量');
insert into metric_description(metric, description) values('system_io_write_bytes', '每秒写取字节数');
insert into metric_description(metric, description) values('system_io_wrqm_s', '每秒合并到设备队列的写请求数');
insert into metric_description(metric, description) values('system_load_1', '近1分钟平均负载');
insert into metric_description(metric, description) values('system_load_5', '近5分钟平均负载');
insert into metric_description(metric, description) values('system_load_15', '近15分钟平均负载');
insert into metric_description(metric, description) values('system_mem_buffered', '文件缓冲区的物理RAM量（单位：*byte*）');
insert into metric_description(metric, description) values('system_mem_cached', '缓存内存的物理RAM量（单位：*byte*）');
insert into metric_description(metric, description) values('system_mem_commit_limit', '系统当前可分配的内存总量（单位：*byte*）');
insert into metric_description(metric, description) values('system_mem_committed', '在磁盘分页文件上保留的物理内存量（单位：*byte*）');
insert into metric_description(metric, description) values('system_mem_committed_as', '系统已分配的包括进程未使用的内存量（单位：*byte*）');
insert into metric_description(metric, description) values('system_mem_nonpaged', '不能写入磁盘的物理内存量（单位：*byte*）');
insert into metric_description(metric, description) values('system_mem_paged', '没被使用是可以写入磁盘的物理内存量（单位：*byte*）');
insert into metric_description(metric, description) values('system_mem_free_percent', '内存空闲率');
insert into metric_description(metric, description) values('system_mem_used_percent', '内存使用率');
insert into metric_description(metric, description) values('system_mem_shared', '用作共享内存的物理RAM量（单位：*byte*）');
insert into metric_description(metric, description) values('system_mem_slab', '内核用来缓存数据结构供自己使用的内存量（单位：*byte*）');
insert into metric_description(metric, description) values('system_mem_total', '物理内存总量（单位：*byte*）');
insert into metric_description(metric, description) values('system_mem_free', '空闲内存大小（单位：*byte*）');
insert into metric_description(metric, description) values('system_mem_used', '已用内存大小（单位：*byte*）');
insert into metric_description(metric, description) values('system_swap_cached', '用作缓存的交换空间');
insert into metric_description(metric, description) values('system_swap_free', '空闲swap大小（单位：*byte*）');
insert into metric_description(metric, description) values('system_swap_free_percent', '空闲swap占比');
insert into metric_description(metric, description) values('system_swap_total', 'swap总大小（单位：*byte*）');
insert into metric_description(metric, description) values('system_swap_used', '已用swap大小（单位：*byte*）');
insert into metric_description(metric, description) values('system_swap_used_percent', '已用swap占比（单位：%）');
insert into metric_description(metric, description) values('system_net_bits_rcvd', '每秒设备上收到的bit数');
insert into metric_description(metric, description) values('system_net_bits_sent', '每秒设备上发送的bit数');
insert into metric_description(metric, description) values('system_net_conntrack_count', 'conntrack用量');
insert into metric_description(metric, description) values('system_net_conntrack_count_percent', 'conntrack用量占比（单位：%）');
insert into metric_description(metric, description) values('system_net_conntrack_max', 'conntrack最大值');
insert into metric_description(metric, description) values('system_net_packets_in_count', '接收数据包个数');
insert into metric_description(metric, description) values('system_net_packets_in_error', '接收数据包错误数');
insert into metric_description(metric, description) values('system_net_packets_out_count', '发送数据包个数');
insert into metric_description(metric, description) values('system_net_packets_out_error', '发送数据包错误数');
insert into metric_description(metric, description) values('system_net_tcp4_closing', 'TCPIPv4关闭中连接的数量');
insert into metric_description(metric, description) values('system_net_tcp4_established', 'TCPIPv4建立的连接数');
insert into metric_description(metric, description) values('system_net_tcp4_listening', 'TCPIPv4监听连接的数量');
insert into metric_description(metric, description) values('system_net_tcp4_opening', 'TCPIPv4打开中连接的数量');
insert into metric_description(metric, description) values('system_net_tcp6_closing', 'TCPIPv6关闭中连接的数量');
insert into metric_description(metric, description) values('system_net_tcp6_established', 'TCPIPv6建立的连接数');
insert into metric_description(metric, description) values('system_net_tcp6_listening', 'TCPIPv6监听连接的数量');
insert into metric_description(metric, description) values('system_net_tcp6_opening', 'TCPIPv6打开中连接的数量');
insert into metric_description(metric, description) values('system_net_tcp_backlog_drops', '数据包的丢弃数量(TCPbacklog没有空间)');
insert into metric_description(metric, description) values('system_net_tcp_backlog_drops_count', '数据包丢弃总数(TCPbacklog没有空间)');
insert into metric_description(metric, description) values('system_net_tcp_failed_retransmits_co', 'retransmit失败的数据包总数');
insert into metric_description(metric, description) values('system_net_tcp_in_segs', '收到的TCP段数');
insert into metric_description(metric, description) values('system_net_tcp_in_segs_count', '收到的TCP段的总数');
insert into metric_description(metric, description) values('system_net_tcp_listen_drops', '采集周期链接被drop的数量');
insert into metric_description(metric, description) values('system_net_tcp_listen_drops_count', '链接被drop的总数');
insert into metric_description(metric, description) values('system_net_tcp_out_segs', '发送的TCP段数');
insert into metric_description(metric, description) values('system_net_tcp_out_segs_count', '发送的TCP段的总数');
insert into metric_description(metric, description) values('system_net_tcp_recv_q_95percentile', 'TCP接收队列95百分位值（单位：*byte*）');
insert into metric_description(metric, description) values('system_net_tcp_recv_q_avg', 'TCP接收队列平均值（单位：*byte*）');
insert into metric_description(metric, description) values('system_net_tcp_recv_q_count', 'TCP接收连接速率');
insert into metric_description(metric, description) values('system_net_tcp_recv_q_max', 'TCP接收队列最大值（单位：*byte*）');
insert into metric_description(metric, description) values('system_net_tcp_recv_q_median', 'TCP接收队列中位值（单位：*byte*）');
insert into metric_description(metric, description) values('system_net_tcp_retrans_segs', 'TCP段的数量重传数');
insert into metric_description(metric, description) values('system_net_tcp_retrans_segs_count', 'TCP段的数量重传总数');
insert into metric_description(metric, description) values('system_net_tcp_send_q_95percentile', 'TCP发送队列95百分位值（单位：*byte*）');
insert into metric_description(metric, description) values('system_net_tcp_send_q_avg', 'TCP发送队列平均值（单位：*byte*）');
insert into metric_description(metric, description) values('system_net_tcp_send_q_count', 'TCP发送连接速率');
insert into metric_description(metric, description) values('system_net_tcp_send_q_max', 'TCP发送队列最大值（单位：*byte*）');
insert into metric_description(metric, description) values('system_net_tcp_send_q_median', 'TCP发送队列中位值（单位：*byte*）');
insert into metric_description(metric, description) values('system_net_udp_in_datagrams', '接收UDP数据报的速率');
insert into metric_description(metric, description) values('system_net_udp_in_datagrams_count', '接收UDP数据报总数');
insert into metric_description(metric, description) values('system_net_udp_in_errors', '接收的无法交付的UDP数据报的速率');
insert into metric_description(metric, description) values('system_net_udp_in_errors_count', '接收的无法交付的UDP数据报的总数');
insert into metric_description(metric, description) values('system_net_udp_no_ports', '收到的目的地端口没有应用程序的UDP数据报的速率');
insert into metric_description(metric, description) values('system_net_udp_no_ports_count', '收到的目的地端口没有应用程序的UDP数据报的总数');
insert into metric_description(metric, description) values('system_net_udp_out_datagrams', '发送UDP数据报的速率');
insert into metric_description(metric, description) values('system_net_udp_out_datagrams_count', '发送UDP数据报的总数');
insert into metric_description(metric, description) values('system_net_udp_rcv_buf_errors', '丢失的UDP数据报速率');
insert into metric_description(metric, description) values('system_net_udp_rcv_buf_errors_count', '丢失的UDP数据报总数（因接收缓冲区没有空间）');
insert into metric_description(metric, description) values('proc_cpu_sys', '进程系统态cpu使用率(单位：%)');
insert into metric_description(metric, description) values('proc_cpu_threads', '进程中线程数量');
insert into metric_description(metric, description) values('proc_cpu_util', '进程cpu使用率(单位：%)');
insert into metric_description(metric, description) values('proc_cpu_user', '进程用户态cpu使用率(单位：%)');
insert into metric_description(metric, description) values('proc_io_read_rate', '进程io读取频率(单位：hz)');
insert into metric_description(metric, description) values('proc_io_readbytes_rate', '进程io读取速率(单位：b/s)');
insert into metric_description(metric, description) values('proc_io_write_rate', '进程io写入频率(单位：hz)');
insert into metric_description(metric, description) values('proc_io_writebytes_rate', '进程io写入速率(单位：b/s)');
insert into metric_description(metric, description) values('proc_mem_data', '进程data内存大小');
insert into metric_description(metric, description) values('proc_mem_dirty', '进程dirty内存大小');
insert into metric_description(metric, description) values('proc_mem_lib', '进程lib内存大小');
insert into metric_description(metric, description) values('proc_mem_rss', '进程常驻内存大小');
insert into metric_description(metric, description) values('proc_mem_shared', '进程共享内存大小');
insert into metric_description(metric, description) values('proc_mem_swap', '进程交换空间大小');
insert into metric_description(metric, description) values('proc_mem_text', '进程Text内存大小');
insert into metric_description(metric, description) values('proc_mem_used', '进程内存使用量（单位：*byte*）');
insert into metric_description(metric, description) values('proc_mem_util', '进程内存使用率(单位：%)');
insert into metric_description(metric, description) values('proc_mem_vms', '进程虚拟内存大小');
insert into metric_description(metric, description) values('proc_net_bits_rate', '进程网络传输率(单位：b/s)');
insert into metric_description(metric, description) values('proc_net_conn_rate', '进程网络连接频率(单位：hz)');
insert into metric_description(metric, description) values('proc_num', '进程个数');
insert into metric_description(metric, description) values('proc_open_fd_count', '进程打开文件句柄数量');
insert into metric_description(metric, description) values('proc_port_listen', '进程监听端口');
insert into metric_description(metric, description) values('proc_uptime_avg', '进程组中最短的运行时间');
insert into metric_description(metric, description) values('proc_uptime_max', '进程组中最久的运行时间');
insert into metric_description(metric, description) values('proc_uptime_min', '进程组平均运行时间');
