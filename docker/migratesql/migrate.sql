/* v7.0.0-beta.3 */
CREATE TABLE `builtin_metrics` (
    `id` bigint unsigned NOT NULL AUTO_INCREMENT COMMENT 'unique identifier',
    `collector` varchar(191) NOT NULL COMMENT 'type of collector',
    `typ` varchar(191) NOT NULL COMMENT 'type of metric',
    `name` varchar(191) NOT NULL COMMENT 'name of metric',
    `unit` varchar(191) NOT NULL COMMENT 'unit of metric',
    `lang` varchar(191) NOT NULL DEFAULT '' COMMENT 'language of metric',
    `note` varchar(4096) NOT NULL COMMENT 'description of metric in Chinese',
    `expression` varchar(4096) NOT NULL COMMENT 'expression of metric',
    `created_at` bigint NOT NULL DEFAULT 0 COMMENT 'create time',
    `created_by` varchar(191) NOT NULL DEFAULT '' COMMENT 'creator',
    `updated_at` bigint NOT NULL DEFAULT 0 COMMENT 'update time',
    `updated_by` varchar(191) NOT NULL DEFAULT '' COMMENT 'updater',
    PRIMARY KEY (`id`),
    UNIQUE KEY `idx_collector_typ_name` (`lang`,`collector`, `typ`, `name`),
    INDEX `idx_collector` (`collector`),
    INDEX `idx_typ` (`typ`),
    INDEX `idx_name` (`name`),
    INDEX `idx_lang` (`lang`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE `metric_filter` (
  `id` bigint NOT NULL AUTO_INCREMENT COMMENT 'unique identifier',
  `name` varchar(191) NOT NULL COMMENT 'name of metric filter',
  `configs` varchar(4096) NOT NULL COMMENT 'configuration of metric filter',
  `groups_perm` text,
  `create_at` bigint NOT NULL DEFAULT '0' COMMENT 'create time',
  `create_by` varchar(191) NOT NULL DEFAULT '' COMMENT 'creator',
  `update_at` bigint NOT NULL DEFAULT '0' COMMENT 'update time',
  `update_by` varchar(191) NOT NULL DEFAULT '' COMMENT 'updater',
  PRIMARY KEY (`id`),
  KEY `idx_name` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;


CREATE TABLE `board_busigroup` (
  `busi_group_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'busi group id',
  `board_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'board id',
  PRIMARY KEY (`busi_group_id`, `board_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

/* v7.0.0-beta.6 */
CREATE TABLE `builtin_components` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT COMMENT '''unique identifier''',
  `ident` varchar(191) NOT NULL COMMENT '''identifier of component''',
  `logo` varchar(191) NOT NULL COMMENT '''logo of component''',
  `readme` text NOT NULL COMMENT '''readme of component''',
  `created_at` bigint(20) NOT NULL DEFAULT 0 COMMENT '''create time''',
  `created_by` varchar(191) NOT NULL DEFAULT '' COMMENT '''creator''',
  `updated_at` bigint(20) NOT NULL DEFAULT 0 COMMENT '''update time''',
  `updated_by` varchar(191) NOT NULL DEFAULT '' COMMENT '''updater''',
  PRIMARY KEY (`id`),
  KEY `idx_ident` (`ident`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE `builtin_payloads` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT COMMENT '''unique identifier''',
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

/* v7.0.0-beta.7 */
ALTER TABLE users ADD COLUMN last_active_time BIGINT NOT NULL DEFAULT 0;

/* v7.0.0-beta.13 */
ALTER TABLE recording_rule ADD COLUMN cron_pattern VARCHAR(255) DEFAULT '' COMMENT 'cron pattern';

/* v7.0.0-beta.14 */
ALTER TABLE alert_cur_event ADD COLUMN original_tags TEXT COMMENT 'labels key=val,,k2=v2';
ALTER TABLE alert_his_event ADD COLUMN original_tags TEXT COMMENT 'labels key=val,,k2=v2';

/* v7.1.0 */
ALTER TABLE target ADD COLUMN os VARCHAR(31) DEFAULT '' COMMENT 'os type';

/* v7.2.0 */
CREATE TABLE notification_record (
    `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
    `event_id` BIGINT NOT NULL,
    `sub_id` BIGINT NOT NULL,
    `channel` VARCHAR(255) NOT NULL,
    `status` TINYINT NOT NULL DEFAULT 0,
    `target` VARCHAR(1024) NOT NULL,
    `details` VARCHAR(2048),
    `created_at` BIGINT NOT NULL,
    INDEX idx_evt (event_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;


/* v7.3.0 2024-08-26 */
ALTER TABLE `target` ADD COLUMN `host_tags` TEXT COMMENT 'global labels set in conf file';

/* v7.3.4 2024-08-28 */
ALTER TABLE `builtin_payloads` ADD COLUMN `component_id` bigint(20) NOT NULL DEFAULT 0 COMMENT 'component_id';

/* v7.4.0 2024-09-20 */
CREATE TABLE `target_busi_group` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `target_ident` varchar(191) NOT NULL,
  `group_id` bigint NOT NULL,
  `update_at` bigint NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_target_group` (`target_ident`,`group_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

/* v7.7.0 2024-11-13 */
ALTER TABLE `recording_rule` ADD COLUMN `datasource_queries` TEXT;
ALTER TABLE `alert_rule` ADD COLUMN `datasource_queries` TEXT;

/* v7.7.2 2024-12-02 */
ALTER TABLE alert_subscribe MODIFY COLUMN rule_ids varchar(1024);
ALTER TABLE alert_subscribe MODIFY COLUMN busi_groups varchar(4096);

/* v8.0.0-beta.1 2024-12-13 */
ALTER TABLE `alert_rule` ADD COLUMN `cron_pattern` VARCHAR(64);
ALTER TABLE `builtin_components` MODIFY COLUMN `logo` mediumtext COMMENT '''logo of component''';

/* v8.0.0-beta.2 2024-12-26 */
ALTER TABLE `es_index_pattern` ADD COLUMN `cross_cluster_enabled` int not null default 0;

/* v8.0.0-beta.3 2024-01-03 */
ALTER TABLE `builtin_components` ADD COLUMN `disabled` INT NOT NULL DEFAULT 0 COMMENT 'is disabled or not';
        
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

/* v8.0.0-beta.5 2025-02-05 */
CREATE TABLE `user_token` (
    `id` bigint NOT NULL AUTO_INCREMENT,
    `username` varchar(255) NOT NULL DEFAULT '',
    `token_name` varchar(255) NOT NULL DEFAULT '',
    `token` varchar(255) NOT NULL DEFAULT '',
    `create_at` bigint NOT NULL DEFAULT 0,
    `last_used` bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;