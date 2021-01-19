set names utf8;
use n9e_mon;


alter table collect_rule change `last_updator` `updater` varchar(64) NOT NULL DEFAULT '' COMMENT 'updater';
alter table collect_rule add `created_at` bigint NOT NULL DEFAULT 0;
alter table collect_rule add `updated_at` bigint NOT NULL DEFAULT 0;
