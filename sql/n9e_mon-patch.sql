set names utf8;
use n9e_mon;

alter table collect_rule add `tags` varchar(512) NOT NULL DEFAULT '' COMMENT 'tags' after data;
