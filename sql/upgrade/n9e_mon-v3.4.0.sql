set names utf8;
use n9e_mon;

CREATE TABLE `collect_rule` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT 'id',
  `nid` bigint(20) unsigned NOT NULL DEFAULT '0' COMMENT 'nid',
  `step` int(11) NOT NULL DEFAULT '0' COMMENT 'step',
  `timeout` int(11) NOT NULL DEFAULT '0' COMMENT 'total timeout',
  `collect_type` varchar(64) NOT NULL DEFAULT '' COMMENT 'collector name',
  `name` varchar(255) NOT NULL DEFAULT '' COMMENT 'name',
  `region` varchar(32) NOT NULL DEFAULT 'default' COMMENT 'region',
  `comment` varchar(512) NOT NULL DEFAULT '' COMMENT 'comment',
  `data` blob NULL COMMENT 'data',
  `tags` varchar(512) NOT NULL DEFAULT '' COMMENT 'tags',
  `creator` varchar(64) NOT NULL DEFAULT '' COMMENT 'creator',
  `last_updator` varchar(64) NOT NULL DEFAULT '' COMMENT 'last_updator',
  `created` datetime NOT NULL  COMMENT 'created',
  `last_updated` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_nid` (`nid`),
  KEY `idx_collect_type` (`collect_type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8 COMMENT 'api collect';



