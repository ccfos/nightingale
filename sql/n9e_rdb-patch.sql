set names utf8;
use n9e_rdb;

alter table session add `access_token` char(128) default '' after sid;
alter table session add key (`access_token`);
