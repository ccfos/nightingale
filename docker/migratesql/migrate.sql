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

/* v8.0.0-beta.3 2025-01-03 */
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

/* v8.0.0-beta.7 2025-03-01 */
CREATE TABLE `notify_rule` (
    `id` bigint unsigned not null auto_increment,
    `name` varchar(255) not null,
    `description` text,
    `enable` tinyint(1) not null default 0,
    `user_group_ids` varchar(255) not null default '',
    `notify_configs` text,
    `create_at` bigint not null default 0,
    `create_by` varchar(64) not null default '',
    `update_at` bigint not null default 0,
    `update_by` varchar(64) not null default '',
    PRIMARY KEY (`id`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

CREATE TABLE `notify_channel` (
    `id` bigint unsigned not null auto_increment,
    `name` varchar(255) not null,
    `ident` varchar(255) not null,
    `description` text, 
    `enable` tinyint(1) not null default 0,
    `param_config` text,
    `request_type` varchar(50) not null,
    `request_config` text,
    `create_at` bigint not null default 0,
    `create_by` varchar(64) not null default '',
    `update_at` bigint not null default 0,
    `update_by` varchar(64) not null default '',
    PRIMARY KEY (`id`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

CREATE TABLE `message_template` (
    `id` bigint unsigned not null auto_increment,
    `name` varchar(64) not null,
    `ident` varchar(64) not null,
    `content` text,
    `user_group_ids` varchar(64),
    `notify_channel_ident` varchar(64) not null default '',
    `private` int not null default 0,
    `create_at` bigint not null default 0,
    `create_by` varchar(64) not null default '',
    `update_at` bigint not null default 0,
    `update_by` varchar(64) not null default '',
    PRIMARY KEY (`id`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

ALTER TABLE `alert_rule` ADD COLUMN `notify_rule_ids` varchar(1024) DEFAULT '';
ALTER TABLE `alert_rule` ADD COLUMN `notify_version` int DEFAULT 0;

ALTER TABLE `alert_subscribe` ADD COLUMN `notify_rule_ids` varchar(1024) DEFAULT '';
ALTER TABLE `alert_subscribe` ADD COLUMN `notify_version` int DEFAULT 0;

ALTER TABLE `notification_record` ADD COLUMN `notify_rule_id` BIGINT NOT NULL DEFAULT 0;


/* v8.0.0-beta.9 2025-03-17 */
ALTER TABLE `message_template` ADD COLUMN `weight` int not null default 0;
ALTER TABLE `notify_channel` ADD COLUMN `weight` int not null default 0;

/* v8.0.0-beta.11 2025-04-10 */
ALTER TABLE `es_index_pattern` ADD COLUMN `note` varchar(1024) not null default '';
ALTER TABLE `datasource` ADD COLUMN `identifier` varchar(255) not null default '';

/* v8.0.0-beta.11 2025-05-15 */
ALTER TABLE `notify_rule` ADD COLUMN `pipeline_configs` text;

CREATE TABLE `event_pipeline` (
    `id` bigint unsigned not null auto_increment,
    `name` varchar(128) not null,
    `team_ids` text,
    `description` varchar(255) not null default '',
    `filter_enable` tinyint(1) not null default 0,
    `attr_filters` text,
    `processor_configs` text,
    `create_at` bigint not null default 0,
    `create_by` varchar(64) not null default '',
    `update_at` bigint not null default 0,
    `update_by` varchar(64) not null default '',
    PRIMARY KEY (`id`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

/* v8.0.0 2025-05-15 */
CREATE TABLE `embedded_product` (
    `id` bigint unsigned NOT NULL AUTO_INCREMENT,
    `name` varchar(255) DEFAULT NULL,
    `url` varchar(255) DEFAULT NULL,
    `is_private` boolean DEFAULT NULL,
    `team_ids` varchar(255),
    `create_at` bigint not null default 0,
    `create_by` varchar(64) not null default '',
    `update_at` bigint not null default 0,
    `update_by` varchar(64) not null default '',
    PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

/* v8.0.0 2025-05-29 */
CREATE TABLE `source_token` (
    `id` bigint unsigned NOT NULL AUTO_INCREMENT,
    `source_type` varchar(64) NOT NULL DEFAULT '' COMMENT 'source type',
    `source_id` varchar(255) NOT NULL DEFAULT '' COMMENT 'source identifier',
    `token` varchar(255) NOT NULL DEFAULT '' COMMENT 'access token',
    `expire_at` bigint NOT NULL DEFAULT 0 COMMENT 'expire timestamp',
    `create_at` bigint NOT NULL DEFAULT 0 COMMENT 'create timestamp',
    `create_by` varchar(64) NOT NULL DEFAULT '' COMMENT 'creator',
    PRIMARY KEY (`id`),
    KEY `idx_source_type_id_token` (`source_type`, `source_id`, `token`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;



/* v8.0.0-beta.12 2025-06-03 */
ALTER TABLE `alert_his_event` ADD COLUMN `notify_rule_ids` text COMMENT 'notify rule ids';
ALTER TABLE `alert_cur_event` ADD COLUMN `notify_rule_ids` text COMMENT 'notify rule ids';

/* v8.0.0-beta.13 */
-- 删除 builtin_metrics 表的 idx_collector_typ_name 唯一索引
DROP INDEX IF EXISTS `idx_collector_typ_name` ON `builtin_metrics`;

/* v8.0.0 2025-07-03 */
ALTER TABLE `builtin_metrics` ADD COLUMN `translation` TEXT COMMENT 'translation of metric' AFTER `lang`;

/* v8.4.0 2025-10-15 */
ALTER TABLE `notify_rule` ADD COLUMN `extra_config` text COMMENT 'extra config';

/* v8.4.1 2025-11-10 */
ALTER TABLE `alert_rule` ADD COLUMN `pipeline_configs` text COMMENT 'pipeline configs';

/* v8.4.2 2025-11-13 */
ALTER TABLE `board` ADD COLUMN `note` varchar(1024) not null default '' comment 'note';
ALTER TABLE `builtin_payloads` ADD COLUMN `note` varchar(1024) not null default '' comment 'note of payload';

/* v9 2026-01-09 */
ALTER TABLE `event_pipeline` ADD COLUMN `typ` varchar(128) NOT NULL DEFAULT '' COMMENT 'pipeline type: builtin, user-defined';
ALTER TABLE `event_pipeline` ADD COLUMN `use_case` varchar(128) NOT NULL DEFAULT '' COMMENT 'use case: metric_explorer, event_summary, event_pipeline';
ALTER TABLE `event_pipeline` ADD COLUMN `trigger_mode` varchar(128) NOT NULL DEFAULT 'event' COMMENT 'trigger mode: event, api, cron';
ALTER TABLE `event_pipeline` ADD COLUMN `disabled` tinyint(1) NOT NULL DEFAULT 0 COMMENT 'disabled flag';
ALTER TABLE `event_pipeline` ADD COLUMN `nodes` text COMMENT 'workflow nodes (JSON)';
ALTER TABLE `event_pipeline` ADD COLUMN `connections` text COMMENT 'node connections (JSON)';
ALTER TABLE `event_pipeline` ADD COLUMN `input_variables` text COMMENT 'input variables (JSON)';
ALTER TABLE `event_pipeline` ADD COLUMN `label_filters` text COMMENT 'label filters (JSON)';

CREATE TABLE `event_pipeline_execution` (
    `id` varchar(36) NOT NULL COMMENT 'execution id',
    `pipeline_id` bigint NOT NULL COMMENT 'pipeline id',
    `pipeline_name` varchar(128) DEFAULT '' COMMENT 'pipeline name snapshot',
    `event_id` bigint DEFAULT 0 COMMENT 'related alert event id',
    `mode` varchar(16) NOT NULL DEFAULT 'event' COMMENT 'trigger mode: event/api/cron',
    `status` varchar(16) NOT NULL DEFAULT 'running' COMMENT 'status: running/success/failed',
    `node_results` mediumtext COMMENT 'node execution results (JSON)',
    `error_message` varchar(1024) DEFAULT '' COMMENT 'error message',
    `error_node` varchar(36) DEFAULT '' COMMENT 'error node id',
    `created_at` bigint NOT NULL DEFAULT 0 COMMENT 'start timestamp',
    `finished_at` bigint DEFAULT 0 COMMENT 'finish timestamp',
    `duration_ms` bigint DEFAULT 0 COMMENT 'duration in milliseconds',
    `trigger_by` varchar(64) DEFAULT '' COMMENT 'trigger by',
    `inputs_snapshot` text COMMENT 'inputs snapshot',
    PRIMARY KEY (`id`),
    KEY `idx_pipeline_id` (`pipeline_id`),
    KEY `idx_event_id` (`event_id`),
    KEY `idx_mode` (`mode`),
    KEY `idx_status` (`status`),
    KEY `idx_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='event pipeline execution records';

/* v8.5.0 builtin_metrics new fields */
ALTER TABLE `builtin_metrics` ADD COLUMN `expression_type` varchar(32) NOT NULL DEFAULT 'promql' COMMENT 'expression type: metric_name or promql';
ALTER TABLE `builtin_metrics` ADD COLUMN `metric_type` varchar(191) NOT NULL DEFAULT '' COMMENT 'metric type like counter/gauge';
ALTER TABLE `builtin_metrics` ADD COLUMN `extra_fields` text COMMENT 'custom extra fields';

/* v9 2026-01-16 saved_view */
CREATE TABLE `saved_view` (
    `id` bigint NOT NULL AUTO_INCREMENT,
    `name` varchar(255) NOT NULL COMMENT 'view name',
    `page` varchar(64) NOT NULL COMMENT 'page identifier',
    `filter` text COMMENT 'filter config (JSON)',
    `public_cate` int NOT NULL DEFAULT 0 COMMENT 'public category: 0-self, 1-team, 2-all',
    `gids` text COMMENT 'team group ids (JSON)',
    `create_at` bigint NOT NULL DEFAULT 0 COMMENT 'create timestamp',
    `create_by` varchar(64) NOT NULL DEFAULT '' COMMENT 'creator',
    `update_at` bigint NOT NULL DEFAULT 0 COMMENT 'update timestamp',
    `update_by` varchar(64) NOT NULL DEFAULT '' COMMENT 'updater',
    PRIMARY KEY (`id`),
    KEY `idx_page` (`page`),
    KEY `idx_create_by` (`create_by`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='saved views for pages';

CREATE TABLE `user_view_favorite` (
    `id` bigint NOT NULL AUTO_INCREMENT,
    `view_id` bigint NOT NULL COMMENT 'saved view id',
    `user_id` bigint NOT NULL COMMENT 'user id',
    `create_at` bigint NOT NULL DEFAULT 0 COMMENT 'create timestamp',
    PRIMARY KEY (`id`),
    KEY `idx_view_id` (`view_id`),
    KEY `idx_user_id` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='user favorite views';

/* v9 2026-01-20 datasource weight */
ALTER TABLE `datasource` ADD COLUMN `weight` int not null default 0 COMMENT 'weight for sorting';
