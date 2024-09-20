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