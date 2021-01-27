set names utf8;
use n9e_mon;

drop table if exists `collect_rule`;

CREATE TABLE `collect_rule` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT 'id',
  `nid` bigint(20) unsigned NOT NULL DEFAULT '0' COMMENT 'nid',
  `step` int(11) NOT NULL DEFAULT '0' COMMENT 'step',
  `timeout` int(11) NOT NULL DEFAULT '0' COMMENT 'total timeout',
  `collect_type` varchar(64) NOT NULL DEFAULT '' COMMENT 'collect plugin name',
  `name` varchar(255) NOT NULL DEFAULT '' COMMENT 'collect rule name',
  `region` varchar(32) NOT NULL DEFAULT 'default' COMMENT 'region',
  `comment` varchar(512) NOT NULL DEFAULT '' COMMENT 'comment',
  `data` blob NULL COMMENT 'data',
  `tags` varchar(512) NOT NULL DEFAULT '' COMMENT 'tags',
  `creator` varchar(64) NOT NULL DEFAULT '' COMMENT 'creator',
  `updater` varchar(64) NOT NULL DEFAULT '' COMMENT 'updater',
  `created_at` bigint not null default 0,
  `updated_at` bigint not null default 0,
  PRIMARY KEY (`id`),
  KEY `idx_nid` (`nid`),
  KEY `idx_collect_type` (`collect_type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8 COMMENT 'collect rule';
