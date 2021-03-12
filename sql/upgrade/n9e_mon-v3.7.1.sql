set names utf8;
use n9e_mon;

alter table log_collect add  `whether_attache_one_log_line` tinyint(1)    not null default 0 after last_updated;


