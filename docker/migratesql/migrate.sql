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