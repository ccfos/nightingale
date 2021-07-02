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
insert into metric_description(metric, description) values('ntp_offset','本地时钟与NTP参考时钟之间的时间差(单位：秒)');
insert into metric_description(metric, description) values('proc_cpu_sys','进程系统态cpu使用率(单位：%)');
insert into metric_description(metric, description) values('proc_cpu_threads','进程中线程数量');
insert into metric_description(metric, description) values('proc_cpu_total','进程cpu使用率(单位：%)');
insert into metric_description(metric, description) values('proc_cpu_user','进程用户态cpu使用率(单位：%)');
insert into metric_description(metric, description) values('proc_createtime','进程启动时间');
insert into metric_description(metric, description) values('proc_io_read_rate','进程io读取频率(单位：hz)');
insert into metric_description(metric, description) values('proc_io_readbytes_rate','进程io读取速率(单位：b/s)');
insert into metric_description(metric, description) values('proc_io_write_rate','进程io写入频率(单位：hz)');
insert into metric_description(metric, description) values('proc_io_writebytes_rate','进程io写入速率(单位：b/s)');
insert into metric_description(metric, description) values('proc_mem_data','进程data内存大小');
insert into metric_description(metric, description) values('proc_mem_dirty','进程dirty内存大小');
insert into metric_description(metric, description) values('proc_mem_lib','进程lib内存大小');
insert into metric_description(metric, description) values('proc_mem_rss','进程常驻内存大小');
insert into metric_description(metric, description) values('proc_mem_shared','进程共享内存大小');
insert into metric_description(metric, description) values('proc_mem_swap','进程交换空间大小');
insert into metric_description(metric, description) values('proc_mem_text','进程Text内存大小');
insert into metric_description(metric, description) values('proc_mem_vms','进程虚拟内存大小');
insert into metric_description(metric, description) values('proc_net_bytes_rate','进程网络传输率(单位：b/s)');
insert into metric_description(metric, description) values('proc_net_conn_rate','进程网络连接频率(单位：hz)');
insert into metric_description(metric, description) values('proc_num','进程数量');
insert into metric_description(metric, description) values('proc_open_fd_count','进程文件句柄数量');
insert into metric_description(metric, description) values('proc_port_listen','进程监听端口');
insert into metric_description(metric, description) values('proc_uptime','进程运行时间');
insert into metric_description(metric, description) values('system_cpu_context_switches','cpu上下文交换次数');
insert into metric_description(metric, description) values('system_cpu_guest','CPU运行虚拟处理器的时间百分比,仅适用于虚拟机监控程序');
insert into metric_description(metric, description) values('system_cpu_interrupt','处理器在处理中断上花费的时间百分比');
insert into metric_description(metric, description) values('system_cpu_iowait','CPU等待IO操作完成所花费的时间百分比');
insert into metric_description(metric, description) values('system_cpu_num_cores','CPU核心数');
insert into metric_description(metric, description) values('system_cpu_stolen','虚拟CPU等待虚拟机监控程序为另一个虚拟CPU提供服务所用的时间百分比,仅适用于虚拟机');
insert into metric_description(metric, description) values('system_cpu_system','CPU运行内核的时间百分比');
insert into metric_description(metric, description) values('system_cpu_user','CPU用于运行用户空间进程的时间百分比');
insert into metric_description(metric, description) values('system_disk_free','可用的磁盘空间量');
insert into metric_description(metric, description) values('system_disk_in_use','磁盘空间使用率(单位：%)');
insert into metric_description(metric, description) values('system_disk_read_time','每台设备阅读所花费的时间 (单位：ms)');
insert into metric_description(metric, description) values('system_disk_read_time_pct','读取磁盘时间百分比');
insert into metric_description(metric, description) values('system_disk_total','磁盘空间总量');
insert into metric_description(metric, description) values('system_disk_used','磁盘空间已用量');
insert into metric_description(metric, description) values('system_disk_write_time','每台设备写入所花费的时间 (单位：ms)');
insert into metric_description(metric, description) values('system_disk_write_time_pct','写入磁盘的时间百分比');
insert into metric_description(metric, description) values('system_fs_file_handles_allocated','系统分配文件句柄数');
insert into metric_description(metric, description) values('system_fs_file_handles_allocated_unused','系统上未使用的已分配文件句柄数');
insert into metric_description(metric, description) values('system_fs_file_handles_in_use','已使用的已分配文件句柄数量超过系统最大值');
insert into metric_description(metric, description) values('system_fs_file_handles_max','系统上分配的最大文件句柄');
insert into metric_description(metric, description) values('system_fs_file_handles_used','系统使用的已分配文件句柄数');
insert into metric_description(metric, description) values('system_fs_inodes_free','空闲 inode 的数量');
insert into metric_description(metric, description) values('system_fs_inodes_in_use','正在使用的 inode 数量占总数的百分比');
insert into metric_description(metric, description) values('system_fs_inodes_total','inode 总数');
insert into metric_description(metric, description) values('system_fs_inodes_used','正在使用的 inode 数量');
insert into metric_description(metric, description) values('system_io_avg_q_sz','发送到设备的请求的平均队列大小');
insert into metric_description(metric, description) values('system_io_avg_rq_sz','向设备发出的请求的平均大小');
insert into metric_description(metric, description) values('system_io_await','每个I/O的平均耗时 (单位：ms)');
insert into metric_description(metric, description) values('system_io_r_await','读请求平均耗时 (单位：ms)');
insert into metric_description(metric, description) values('system_io_r_s','每秒向设备发出的读取请求数');
insert into metric_description(metric, description) values('system_io_rkb_s','每秒从设备读取的千字节数');
insert into metric_description(metric, description) values('system_io_rrqm_s','每秒合并到设备队列的读取请求数');
insert into metric_description(metric, description) values('system_io_svctm','向设备发出请求的平均服务时间');
insert into metric_description(metric, description) values('system_io_util','向设备发出 I/O 请求的 CPU 时间百分比');
insert into metric_description(metric, description) values('system_io_w_await','写请求平均耗时 (单位：ms)');
insert into metric_description(metric, description) values('system_io_w_s','每秒向设备发出的写请求数');
insert into metric_description(metric, description) values('system_io_wkb_s','每秒写入设备的千字节数');
insert into metric_description(metric, description) values('system_io_wrqm_s','每秒合并到设备中的写入请求数');
insert into metric_description(metric, description) values('system_load_1','1分钟的平均系统负载');
insert into metric_description(metric, description) values('system_load_15','15分钟的平均系统负载');
insert into metric_description(metric, description) values('system_load_5','5分钟的平均系统负载');
insert into metric_description(metric, description) values('system_load_norm_1','1 分钟内的平均系统负载由CPU数量标准化');
insert into metric_description(metric, description) values('system_load_norm_15','15 分钟内的平均系统负载由CPU数量标准化');
insert into metric_description(metric, description) values('system_load_norm_5','5 分钟内的平均系统负载由CPU数量标准化');
insert into metric_description(metric, description) values('system_mem_buffered','用于文件缓冲区的物理 RAM 量');
insert into metric_description(metric, description) values('system_mem_cached','用作缓存内存的物理 RAM 量');
insert into metric_description(metric, description) values('system_mem_commit_limit','系统当前可分配的内存总量，基于过量使用率');
insert into metric_description(metric, description) values('system_mem_committed','已在磁盘分页文件上保留空间的物理内存量，以防必须将其写回磁盘');
insert into metric_description(metric, description) values('system_mem_committed_as','系统上当前分配的内存量，即使它尚未被进程使用');
insert into metric_description(metric, description) values('system_mem_free','空闲内存的数量');
insert into metric_description(metric, description) values('system_mem_nonpaged','操作系统用于对象的物理内存量，这些对象不能写入磁盘，但只要分配了它们就必须保留在物理内存中');
insert into metric_description(metric, description) values('system_mem_page_free','空闲页面文件的数量');
insert into metric_description(metric, description) values('system_mem_page_tables','专用于最低页表级别的内存量');
insert into metric_description(metric, description) values('system_mem_page_total','页面文件的总大小');
insert into metric_description(metric, description) values('system_mem_page_used','正在使用的页面文件的数量');
insert into metric_description(metric, description) values('system_mem_paged','操作系统为对象使用的物理内存量，这些对象在不使用时可以写入磁盘');
insert into metric_description(metric, description) values('system_mem_pagefile_free','空闲页文件的数量');
insert into metric_description(metric, description) values('system_mem_pagefile_pct_free','免费的页文件数量占总数的百分比');
insert into metric_description(metric, description) values('system_mem_pagefile_total','页文件的总大小');
insert into metric_description(metric, description) values('system_mem_pagefile_used','正在使用的页文件的数量');
insert into metric_description(metric, description) values('system_mem_pct_usable','可用物理 RAM 的数量占总量的百分比');
insert into metric_description(metric, description) values('system_mem_pct_used','已使用物理 RAM 的数量占总量的百分比');
insert into metric_description(metric, description) values('system_mem_shared','用作共享内存的物理 RAM 量');
insert into metric_description(metric, description) values('system_mem_slab','内核用来缓存数据结构供自己使用的内存量');
insert into metric_description(metric, description) values('system_mem_total','物理内存总量(mb)');
insert into metric_description(metric, description) values('system_mem_usable','如果存在 /proc/meminfo 中 MemAvailable 的值，但如果不存在，则回退到添加空闲 + 缓冲 + 缓存内存(mb)');
insert into metric_description(metric, description) values('system_mem_used','正在使用的 RAM 量(mb)');
insert into metric_description(metric, description) values('system_net_bytes_rcvd',' 每秒设备上收到的字节数');
insert into metric_description(metric, description) values('system_net_bytes_sent',' 每秒设备上发送的字节数');
insert into metric_description(metric, description) values('system_net_conntrack_acct','boolean,启用连接跟踪流量计费每流程64位字节和数据包计数器');
insert into metric_description(metric, description) values('system_net_conntrack_buckets','哈希表的大小');
insert into metric_description(metric, description) values('system_net_conntrack_checksum','boolean验证传入数据包的校验和');
insert into metric_description(metric, description) values('system_net_conntrack_count','ConnTrack表中存在的连接数');
insert into metric_description(metric, description) values('system_net_conntrack_drop','ConnTrack表中的跌幅数');
insert into metric_description(metric, description) values('system_net_conntrack_early_drop','Conntrack表中的早期跌落的数量');
insert into metric_description(metric, description) values('system_net_conntrack_error','Conntrack表中的错误数');
insert into metric_description(metric, description) values('system_net_conntrack_events','boolean启用连接跟踪代码将通过ctnetlink提供具有连接跟踪事件的用户空间');
insert into metric_description(metric, description) values('system_net_conntrack_events_retry_timeout','events_retry_timeout');
insert into metric_description(metric, description) values('system_net_conntrack_expect_max','期望表的最大大小');
insert into metric_description(metric, description) values('system_net_conntrack_found','当前分配的流条目的数量');
insert into metric_description(metric, description) values('system_net_conntrack_generic_timeout','默认为通用超时这是指第4层未知/不支持的协议');
insert into metric_description(metric, description) values('system_net_conntrack_helper','boolean启用自动contrack辅助分配');
insert into metric_description(metric, description) values('system_net_conntrack_icmp_timeout','默认为ICMP超时');
insert into metric_description(metric, description) values('system_net_conntrack_ignore','ConnTrack表中忽略的数量');
insert into metric_description(metric, description) values('system_net_conntrack_insert','ConnTrack表中的插入数');
insert into metric_description(metric, description) values('system_net_conntrack_insert_failed','ConnTrack表中的插入失败的数量');
insert into metric_description(metric, description) values('system_net_conntrack_invalid','ConnTrack表中无效的数量');
insert into metric_description(metric, description) values('system_net_conntrack_log_invalid','日志日志无效的数据包由值指定的类型');
insert into metric_description(metric, description) values('system_net_conntrack_max','Conntrack表最大容量');
insert into metric_description(metric, description) values('system_net_conntrack_search_restart','搜索重启');
insert into metric_description(metric, description) values('system_net_conntrack_tcp_be_liberal','boolean只能从窗口rst段标记为无效');
insert into metric_description(metric, description) values('system_net_conntrack_tcp_loose','boolean以启用拾取已经建立的连接');
insert into metric_description(metric, description) values('system_net_conntrack_tcp_max_retrans','可以重新发送的最大数据包数,而无需从目的地接收（可接受的）ACK');
insert into metric_description(metric, description) values('system_net_conntrack_tcp_timeout','TCP超时');
insert into metric_description(metric, description) values('system_net_conntrack_timestamp','boolean启用连接跟踪流量时间戳');
insert into metric_description(metric, description) values('system_net_packets_in_count','接口接收的数据数据包数');
insert into metric_description(metric, description) values('system_net_packets_in_error','设备驱动程序检测到的数据包接收错误数');
insert into metric_description(metric, description) values('system_net_packets_out_count','接口传输的数据数据包数');
insert into metric_description(metric, description) values('system_net_packets_out_error','设备驱动程序检测到的数据包数量');
insert into metric_description(metric, description) values('system_net_tcp_backlog_drops','数据包的数量丢弃,因为TCP积压中没有空间自代理V5.14.0以来');
insert into metric_description(metric, description) values('system_net_tcp_backlog_drops_count','丢弃的数据包总数是因为TCP积压没有空间');
insert into metric_description(metric, description) values('system_net_tcp_failed_retransmits_count','无法重新发送的j数据包总数');
insert into metric_description(metric, description) values('system_net_tcp_in_segs','收到的TCP段数（仅限Linux或Solaris）');
insert into metric_description(metric, description) values('system_net_tcp_in_segs_count','收到的TCP段的总数（仅限Linux或Solaris）');
insert into metric_description(metric, description) values('system_net_tcp_listen_drops','连接的次数已退出侦听由于代理v5.14.0以来可用');
insert into metric_description(metric, description) values('system_net_tcp_listen_drops_count','Connections的总次数已退出侦听');
insert into metric_description(metric, description) values('system_net_tcp_listen_overflows','连接的次数已溢出接受缓冲区由于代理V5.14.0以来');
insert into metric_description(metric, description) values('system_net_tcp_listen_overflows_count','连接的总次数已溢出接受缓冲区');
insert into metric_description(metric, description) values('system_net_tcp_out_segs','仅传输的TCP段数（仅限Linux或Solaris）');
insert into metric_description(metric, description) values('system_net_tcp_out_segs_count','仅传输的TCP段（仅限Linux或Solaris）');
insert into metric_description(metric, description) values('system_net_tcp_rcv_packs','收到的TCP数据包数（仅限BSD）');
insert into metric_description(metric, description) values('system_net_tcp_recv_q_95percentile','TCP接收队列大小的第95百分位数');
insert into metric_description(metric, description) values('system_net_tcp_recv_q_avg','平均TCP接收队列大小');
insert into metric_description(metric, description) values('system_net_tcp_recv_q_count','连接速率');
insert into metric_description(metric, description) values('system_net_tcp_recv_q_max','最大TCP接收队列大小');
insert into metric_description(metric, description) values('system_net_tcp_recv_q_median','中位TCP接收队列大小');
insert into metric_description(metric, description) values('system_net_tcp_retrans_packs','TCP报文的数量重新发送（仅限BSD）');
insert into metric_description(metric, description) values('system_net_tcp_retrans_segs','TCP段的数量重传（仅限Linux或Solaris）');
insert into metric_description(metric, description) values('system_net_tcp_retrans_segs_count','重传TCP段的总数（仅限Linux或Solaris）');
insert into metric_description(metric, description) values('system_net_tcp_send_q_95percentile','TCP发送队列大小的第95百分位数');
insert into metric_description(metric, description) values('system_net_tcp_send_q_avg','平均TCP发送队列大小');
insert into metric_description(metric, description) values('system_net_tcp_send_q_count','连接速率');
insert into metric_description(metric, description) values('system_net_tcp_send_q_max','最大TCP发送队列大小');
insert into metric_description(metric, description) values('system_net_tcp_send_q_median','中位TCP发送队列大小');
insert into metric_description(metric, description) values('system_net_tcp_sent_packs','传输的TCP报文的数量（仅限BSD）');
insert into metric_description(metric, description) values('system_net_tcp4_closing','TCP IPv4关闭中连接的数量');
insert into metric_description(metric, description) values('system_net_tcp4_established','TCP IPv4建立的连接数');
insert into metric_description(metric, description) values('system_net_tcp4_listening','TCP IPv4监听连接的数量');
insert into metric_description(metric, description) values('system_net_tcp4_opening','TCP IPv4打开中连接的数量');
insert into metric_description(metric, description) values('system_net_tcp6_closing','TCP IPv6关闭中连接的数量');
insert into metric_description(metric, description) values('system_net_tcp6_established','TCP IPv6建立的连接数');
insert into metric_description(metric, description) values('system_net_tcp6_listening','TCP IPv6监听连接的数量');
insert into metric_description(metric, description) values('system_net_tcp6_opening','TCP IPv6打开中连接的数量');
insert into metric_description(metric, description) values('system_net_udp_in_datagrams','向UDP用户提供的UDP数据报的速率');
insert into metric_description(metric, description) values('system_net_udp_in_datagrams_count','向UDP用户提供的UDP数据报总数');
insert into metric_description(metric, description) values('system_net_udp_in_errors','由于缺少目的端口缺少应用程序以外的原因无法提供的接收UDP数据报的速率');
insert into metric_description(metric, description) values('system_net_udp_in_errors_count','由于目的地端口缺少应用程序,无法出于缺少应用程序的原因无法提供的接收UDP数据报的总数');
insert into metric_description(metric, description) values('system_net_udp_no_ports','目的地端口没有应用程序的收到UDP数据报的速率');
insert into metric_description(metric, description) values('system_net_udp_no_ports_count','收到的UDP数据报总数在目标端口没有应用程序');
insert into metric_description(metric, description) values('system_net_udp_out_datagrams','从此实体发送的UDP数据报');
insert into metric_description(metric, description) values('system_net_udp_out_datagrams_count','从此实体发送的UDP数据报总数');
insert into metric_description(metric, description) values('system_net_udp_rcv_buf_errors','丢失的UDP数据报速率因为接收缓冲区中没有空间');
insert into metric_description(metric, description) values('system_net_udp_rcv_buf_errors_count','丢失的UDP数据报总数因接收缓冲区没有空间');
insert into metric_description(metric, description) values('system_net_udp_snd_buf_errors','udp数据报丢失的速率因为发送缓冲区中没有空间');
insert into metric_description(metric, description) values('system_proc_count','进程数（仅限 Windows）');
insert into metric_description(metric, description) values('system_proc_queue_length','在处理器就绪队列中观察到延迟并等待执行的线程数（仅限 Windows）');
insert into metric_description(metric, description) values('system_swap_cached','用作缓存的交换空间');
insert into metric_description(metric, description) values('system_swap_free','可用交换空间的数量');
insert into metric_description(metric, description) values('system_swap_pct_free','未使用的交换空间量占总数的比例(0~1)');
insert into metric_description(metric, description) values('system_swap_total','交换空间总量');
insert into metric_description(metric, description) values('system_swap_used','正在使用的交换空间量');
insert into metric_description(metric, description) values('system_uptime','系统运行的时间');