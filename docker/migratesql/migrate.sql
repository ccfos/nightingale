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

/* beta.5 */

Create Table: CREATE TABLE `builtin_components` (
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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4

Create Table: CREATE TABLE `builtin_payloads` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT COMMENT '''unique identifier''',
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
  KEY `idx_type` (`type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4