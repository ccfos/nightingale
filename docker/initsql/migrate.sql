use n9e_v6;

-- Alter table for AlertSubscribe
ALTER TABLE alert_subscribe ADD COLUMN busi_groups VARCHAR(4096) NOT NULL DEFAULT '[]';
ALTER TABLE alert_subscribe ADD COLUMN note VARCHAR(1024) DEFAULT '' COMMENT 'note';

-- Alter table for TaskRecord
ALTER TABLE task_records ADD COLUMN event_id BIGINT(20) NOT NULL DEFAULT 0 COMMENT 'event id';

-- Alter table for AlertHisEvent
CREATE INDEX idx_last_eval_time ON alert_his_event (last_eval_time);

-- Alter table for Target
ALTER TABLE target ADD COLUMN host_ip VARCHAR(15) DEFAULT '' COMMENT 'IPv4 string';
ALTER TABLE target ADD COLUMN agent_version VARCHAR(255) DEFAULT '' COMMENT 'agent version';

-- Alter table for Datasource
ALTER TABLE datasource ADD COLUMN is_default TINYINT NOT NULL DEFAULT 0 COMMENT 'is default datasource';

-- Alter table for Configs
ALTER TABLE configs ADD COLUMN note VARCHAR(1024) DEFAULT '' COMMENT 'note';
ALTER TABLE configs ADD COLUMN external INT DEFAULT 0 COMMENT '0: built-in 1: external';
ALTER TABLE configs ADD COLUMN encrypted INT DEFAULT 0 COMMENT '0: plaintext 1: ciphertext';
ALTER TABLE configs ADD COLUMN create_at INT DEFAULT 0 COMMENT 'create_at';
ALTER TABLE configs ADD COLUMN create_by VARCHAR(64) DEFAULT '' COMMENT 'create_by';
ALTER TABLE configs ADD COLUMN update_at INT DEFAULT 0 COMMENT 'update_at';
ALTER TABLE configs ADD COLUMN update_by VARCHAR(64) DEFAULT '' COMMENT 'update_by';
ALTER TABLE configs DROP INDEX ckey;

ALTER TABLE notify_tpl ADD COLUMN create_at INT DEFAULT 0 COMMENT 'create_at';
ALTER TABLE notify_tpl ADD COLUMN create_by VARCHAR(64) DEFAULT '' COMMENT 'create_by';
ALTER TABLE notify_tpl ADD COLUMN update_at INT DEFAULT 0 COMMENT 'update_at';
ALTER TABLE notify_tpl ADD COLUMN update_by VARCHAR(64) DEFAULT '' COMMENT 'update_by';