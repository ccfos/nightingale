set names utf8;
use n9e_mon;

drop index idx_etime on event;
drop index idx_event_type on event;
drop index idx_status on event;

ALTER TABLE event ADD INDEX `idx_etime_hashid_type_status` (`etime`,`hashid`,`event_type`,`status`);
