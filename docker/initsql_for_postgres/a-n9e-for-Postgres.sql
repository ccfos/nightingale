-- n9e version 5.8 for postgres 
CREATE TABLE users (
    id bigserial,
    username varchar(64) not null ,
    nickname varchar(64) not null ,
    password varchar(128) not null default '',
    phone varchar(16) not null default '',
    email varchar(64) not null default '',
    portrait varchar(255) not null default '' ,
    roles varchar(255) not null ,
    contacts varchar(1024) ,
    maintainer smallint not null default 0,
    create_at bigint not null default 0,
    create_by varchar(64) not null default '',
    update_at bigint not null default 0,
    update_by varchar(64) not null default ''
) ;
ALTER TABLE users ADD CONSTRAINT users_pk PRIMARY KEY (id);
ALTER TABLE users ADD CONSTRAINT users_un UNIQUE (username);

COMMENT ON COLUMN users.username IS 'login name, cannot rename';
COMMENT ON COLUMN users.nickname IS 'display name, chinese name';
COMMENT ON COLUMN users.portrait IS 'portrait image url';
COMMENT ON COLUMN users.roles IS 'Admin | Standard | Guest, split by space';
COMMENT ON COLUMN users.contacts IS 'json e.g. {wecom:xx, dingtalk_robot_token:yy}';

insert into users(id, username, nickname, password, roles, create_at, create_by, update_at, update_by) values(1, 'root', '超管', 'root.2020', 'Admin', date_part('epoch',current_timestamp)::int, 'system', date_part('epoch',current_timestamp)::int, 'system');


CREATE TABLE user_group (
    id bigserial,
    name varchar(128) not null default '',
    note varchar(255) not null default '',
    create_at bigint not null default 0,
    create_by varchar(64) not null default '',
    update_at bigint not null default 0,
    update_by varchar(64) not null default ''
) ;
ALTER TABLE user_group ADD CONSTRAINT user_group_pk PRIMARY KEY (id);
CREATE INDEX user_group_create_by_idx ON user_group (create_by);
CREATE INDEX user_group_update_at_idx ON user_group (update_at);

insert into user_group(id, name, create_at, create_by, update_at, update_by) values(1, 'demo-root-group', date_part('epoch',current_timestamp)::int, 'root', date_part('epoch',current_timestamp)::int, 'root');

CREATE TABLE user_group_member (
    id bigserial,
    group_id bigint  not null,
    user_id bigint  not null
) ;
ALTER TABLE user_group_member ADD CONSTRAINT user_group_member_pk PRIMARY KEY (id);
CREATE INDEX user_group_member_group_id_idx ON user_group_member (group_id);
CREATE INDEX user_group_member_user_id_idx ON user_group_member (user_id);

insert into user_group_member(group_id, user_id) values(1, 1);

CREATE TABLE configs (
    id bigserial,
    ckey varchar(191) not null,
    cval varchar(4096) not null default ''
) ;
ALTER TABLE configs ADD CONSTRAINT configs_pk PRIMARY KEY (id);
ALTER TABLE configs ADD CONSTRAINT configs_un UNIQUE (ckey);

CREATE TABLE role (
    id bigserial,
    name varchar(191) not null default '',
    note varchar(255) not null default ''
) ;
ALTER TABLE role ADD CONSTRAINT role_pk PRIMARY KEY (id);
ALTER TABLE role ADD CONSTRAINT role_un UNIQUE ("name");

insert into role(name, note) values('Admin', 'Administrator role');
insert into role(name, note) values('Standard', 'Ordinary user role');
insert into role(name, note) values('Guest', 'Readonly user role');

CREATE TABLE role_operation(
    id bigserial,
    role_name varchar(128) not null,
    operation varchar(191) not null
) ;
ALTER TABLE role_operation ADD CONSTRAINT role_operation_pk PRIMARY KEY (id);
CREATE INDEX role_operation_role_name_idx ON role_operation (role_name);
CREATE INDEX role_operation_operation_idx ON role_operation (operation);


-- Admin is special, who has no concrete operation but can do anything.
insert into role_operation(role_name, operation) values('Guest', '/metric/explorer');
insert into role_operation(role_name, operation) values('Guest', '/object/explorer');
insert into role_operation(role_name, operation) values('Guest', '/help/version');
insert into role_operation(role_name, operation) values('Guest', '/help/contact');
insert into role_operation(role_name, operation) values('Standard', '/metric/explorer');
insert into role_operation(role_name, operation) values('Standard', '/object/explorer');
insert into role_operation(role_name, operation) values('Standard', '/help/version');
insert into role_operation(role_name, operation) values('Standard', '/help/contact');
insert into role_operation(role_name, operation) values('Standard', '/users');
insert into role_operation(role_name, operation) values('Standard', '/user-groups');
insert into role_operation(role_name, operation) values('Standard', '/user-groups/add');
insert into role_operation(role_name, operation) values('Standard', '/user-groups/put');
insert into role_operation(role_name, operation) values('Standard', '/user-groups/del');
insert into role_operation(role_name, operation) values('Standard', '/busi-groups');
insert into role_operation(role_name, operation) values('Standard', '/busi-groups/add');
insert into role_operation(role_name, operation) values('Standard', '/busi-groups/put');
insert into role_operation(role_name, operation) values('Standard', '/busi-groups/del');
insert into role_operation(role_name, operation) values('Standard', '/targets');
insert into role_operation(role_name, operation) values('Standard', '/targets/add');
insert into role_operation(role_name, operation) values('Standard', '/targets/put');
insert into role_operation(role_name, operation) values('Standard', '/targets/del');
insert into role_operation(role_name, operation) values('Standard', '/dashboards');
insert into role_operation(role_name, operation) values('Standard', '/dashboards/add');
insert into role_operation(role_name, operation) values('Standard', '/dashboards/put');
insert into role_operation(role_name, operation) values('Standard', '/dashboards/del');
insert into role_operation(role_name, operation) values('Standard', '/alert-rules');
insert into role_operation(role_name, operation) values('Standard', '/alert-rules/add');
insert into role_operation(role_name, operation) values('Standard', '/alert-rules/put');
insert into role_operation(role_name, operation) values('Standard', '/alert-rules/del');
insert into role_operation(role_name, operation) values('Standard', '/alert-mutes');
insert into role_operation(role_name, operation) values('Standard', '/alert-mutes/add');
insert into role_operation(role_name, operation) values('Standard', '/alert-mutes/del');
insert into role_operation(role_name, operation) values('Standard', '/alert-subscribes');
insert into role_operation(role_name, operation) values('Standard', '/alert-subscribes/add');
insert into role_operation(role_name, operation) values('Standard', '/alert-subscribes/put');
insert into role_operation(role_name, operation) values('Standard', '/alert-subscribes/del');
insert into role_operation(role_name, operation) values('Standard', '/alert-cur-events');
insert into role_operation(role_name, operation) values('Standard', '/alert-cur-events/del');
insert into role_operation(role_name, operation) values('Standard', '/alert-his-events');
insert into role_operation(role_name, operation) values('Standard', '/job-tpls');
insert into role_operation(role_name, operation) values('Standard', '/job-tpls/add');
insert into role_operation(role_name, operation) values('Standard', '/job-tpls/put');
insert into role_operation(role_name, operation) values('Standard', '/job-tpls/del');
insert into role_operation(role_name, operation) values('Standard', '/job-tasks');
insert into role_operation(role_name, operation) values('Standard', '/job-tasks/add');
insert into role_operation(role_name, operation) values('Standard', '/job-tasks/put');
insert into role_operation(role_name, operation) values('Standard', '/recording-rules');
insert into role_operation(role_name, operation) values('Standard', '/recording-rules/add');
insert into role_operation(role_name, operation) values('Standard', '/recording-rules/put');
insert into role_operation(role_name, operation) values('Standard', '/recording-rules/del');

CREATE TABLE recording_rule (
    id bigserial NOT NULL,
    group_id bigint not null default 0,
    cluster varchar(128) not null,
    name varchar(255) not null,
    note varchar(255) not null,
    disabled smallint not null,
    prom_ql varchar(8192) not null,
    prom_eval_interval int not null,
    append_tags varchar(255) default '',
    create_at bigint default 0,
    create_by varchar(64) default '',
    update_at bigint default 0,
    update_by varchar(64) default '',
    CONSTRAINT recording_rule_pk PRIMARY KEY (id)
) ;
CREATE INDEX recording_rule_group_id_idx ON recording_rule (group_id);
CREATE INDEX recording_rule_update_at_idx ON recording_rule (update_at);
COMMENT ON COLUMN recording_rule.group_id IS 'group_id';
COMMENT ON COLUMN recording_rule.name IS 'new metric name';
COMMENT ON COLUMN recording_rule.note IS 'rule note';
COMMENT ON COLUMN recording_rule.disabled IS '0:enabled 1:disabled';
COMMENT ON COLUMN recording_rule.prom_ql IS 'promql';
COMMENT ON COLUMN recording_rule.prom_eval_interval IS 'evaluate interval';
COMMENT ON COLUMN recording_rule.append_tags IS 'split by space: service=n9e mod=api';

-- for alert_rule | collect_rule | mute | dashboard grouping
CREATE TABLE busi_group (
    id bigserial,
    name varchar(191) not null,
    label_enable smallint not null default 0,
    label_value varchar(191) not null default '' ,
    create_at bigint not null default 0,
    create_by varchar(64) not null default '',
    update_at bigint not null default 0,
    update_by varchar(64) not null default ''
) ;
ALTER TABLE busi_group ADD CONSTRAINT busi_group_pk PRIMARY KEY (id);
ALTER TABLE busi_group ADD CONSTRAINT busi_group_un UNIQUE ("name");
COMMENT ON COLUMN busi_group.label_value IS 'if label_enable: label_value can not be blank';

insert into busi_group(id, name, create_at, create_by, update_at, update_by) values(1, 'Default Busi Group', date_part('epoch',current_timestamp)::int, 'root', date_part('epoch',current_timestamp)::int, 'root');

CREATE TABLE busi_group_member (
    id bigserial,
    busi_group_id bigint not null ,
    user_group_id bigint not null ,
    perm_flag char(2) not null 
) ;
ALTER TABLE busi_group_member ADD CONSTRAINT busi_group_member_pk PRIMARY KEY (id);
CREATE INDEX busi_group_member_busi_group_id_idx ON busi_group_member (busi_group_id);
CREATE INDEX busi_group_member_user_group_id_idx ON busi_group_member (user_group_id);
COMMENT ON COLUMN busi_group_member.busi_group_id IS 'busi group id';
COMMENT ON COLUMN busi_group_member.user_group_id IS 'user group id';
COMMENT ON COLUMN busi_group_member.perm_flag IS 'ro | rw';

insert into busi_group_member(busi_group_id, user_group_id, perm_flag) values(1, 1, 'rw');

-- for dashboard new version
CREATE TABLE board (
    id bigserial  not null ,
    group_id bigint not null default 0 ,
    name varchar(191) not null,
    ident varchar(200) not null default '',
    tags varchar(255) not null ,
    public smallint not null default 0,
    create_at bigint not null default 0,
    create_by varchar(64) not null default '',
    update_at bigint not null default 0,
    update_by varchar(64) not null default ''
) ;
ALTER TABLE board ADD CONSTRAINT board_pk PRIMARY KEY (id);
ALTER TABLE board ADD CONSTRAINT board_un UNIQUE (group_id,"name");
COMMENT ON COLUMN board.group_id IS 'busi group id';
COMMENT ON COLUMN board.tags IS 'split by space';
COMMENT ON COLUMN board.public IS '0:false 1:true';
CREATE INDEX board_ident_idx ON board (ident);

-- for dashboard new version
CREATE TABLE board_payload (
    id bigint  not null ,
    payload text not null
);
ALTER TABLE board_payload ADD CONSTRAINT board_payload_un UNIQUE (id);
COMMENT ON COLUMN board_payload.id IS 'dashboard id';

-- deprecated
CREATE TABLE dashboard (
    id bigserial,
    group_id bigint not null default 0 ,
    name varchar(191) not null,
    tags varchar(255) not null ,
    configs varchar(8192) ,
    create_at bigint not null default 0,
    create_by varchar(64) not null default '',
    update_at bigint not null default 0,
    update_by varchar(64) not null default ''
) ;
ALTER TABLE dashboard ADD CONSTRAINT dashboard_pk PRIMARY KEY (id);

-- deprecated
-- auto create the first subclass 'Default chart group' of dashboard
CREATE TABLE chart_group (
    id bigserial,
    dashboard_id bigint  not null,
    name varchar(255) not null,
    weight int not null default 0
) ;
ALTER TABLE chart_group ADD CONSTRAINT chart_group_pk PRIMARY KEY (id);

-- deprecated
CREATE TABLE chart (
    id bigserial,
    group_id bigint  not null ,
    configs text,
    weight int not null default 0
) ;
ALTER TABLE chart ADD CONSTRAINT chart_pk PRIMARY KEY (id);

CREATE TABLE chart_share (
    id bigserial,
    cluster varchar(128) not null,
    configs text,
    create_at bigint not null default 0,
    create_by varchar(64) not null default ''
) ;
ALTER TABLE chart_share ADD CONSTRAINT chart_share_pk PRIMARY KEY (id);
CREATE INDEX chart_share_create_at_idx ON chart_share (create_at);

CREATE TABLE alert_rule (
    id bigserial NOT NULL,
    group_id int8 NOT NULL DEFAULT 0,
    cate varchar(128) not null default '' ,
    "cluster" varchar(128) NOT NULL,
    "name" varchar(255) NOT NULL,
    note varchar(1024) NOT NULL,
    severity int2 NOT NULL,
    disabled int2 NOT NULL,
    prom_for_duration int4 NOT NULL,
    prom_ql text NOT NULL,
    prom_eval_interval int4 NOT NULL,
    enable_stime bpchar(5) NOT NULL DEFAULT '00:00'::bpchar,
    enable_etime bpchar(5) NOT NULL DEFAULT '23:59'::bpchar,
    enable_days_of_week varchar(32) NOT NULL DEFAULT ''::character varying,
    enable_in_bg int2 NOT NULL DEFAULT 0,
    notify_recovered int2 NOT NULL,
    notify_channels varchar(255) NOT NULL DEFAULT ''::character varying,
    notify_groups varchar(255) NOT NULL DEFAULT ''::character varying,
    notify_repeat_step int4 NOT NULL DEFAULT 0,
    notify_max_number int4 not null default 0,
    recover_duration int4 NOT NULL DEFAULT 0,
    callbacks varchar(255) NOT NULL DEFAULT ''::character varying,
    runbook_url varchar(255) NULL,
    append_tags varchar(255) NOT NULL DEFAULT ''::character varying,
    create_at int8 NOT NULL DEFAULT 0,
    create_by varchar(64) NOT NULL DEFAULT ''::character varying,
    update_at int8 NOT NULL DEFAULT 0,
    update_by varchar(64) NOT NULL DEFAULT ''::character varying,
    prod varchar(255) NOT NULL DEFAULT ''::character varying,
    algorithm varchar(255) NOT NULL DEFAULT ''::character varying,
    algo_params varchar(255) NULL,
    delay int4 NOT NULL DEFAULT 0,
    CONSTRAINT alert_rule_pk PRIMARY KEY (id)
);
CREATE INDEX alert_rule_group_id_idx ON alert_rule USING btree (group_id);
CREATE INDEX alert_rule_update_at_idx ON alert_rule USING btree (update_at);

COMMENT ON COLUMN alert_rule.group_id IS 'busi group id';
COMMENT ON COLUMN alert_rule.severity IS '0:Emergency 1:Warning 2:Notice';
COMMENT ON COLUMN alert_rule.disabled IS '0:enabled 1:disabled';
COMMENT ON COLUMN alert_rule.prom_for_duration IS 'prometheus for, unit:s';
COMMENT ON COLUMN alert_rule.prom_ql IS 'promql';
COMMENT ON COLUMN alert_rule.prom_eval_interval IS 'evaluate interval';
COMMENT ON COLUMN alert_rule.enable_stime IS '00:00';
COMMENT ON COLUMN alert_rule.enable_etime IS '23:59';
COMMENT ON COLUMN alert_rule.enable_days_of_week IS 'split by space: 0 1 2 3 4 5 6';
COMMENT ON COLUMN alert_rule.enable_in_bg IS '1: only this bg 0: global';
COMMENT ON COLUMN alert_rule.notify_recovered IS 'whether notify when recovery';
COMMENT ON COLUMN alert_rule.notify_channels IS 'split by space: sms voice email dingtalk wecom';
COMMENT ON COLUMN alert_rule.notify_groups IS 'split by space: 233 43';
COMMENT ON COLUMN alert_rule.notify_repeat_step IS 'unit: min';
COMMENT ON COLUMN alert_rule.recover_duration IS 'unit: s';
COMMENT ON COLUMN alert_rule.callbacks IS 'split by space: http://a.com/api/x http://a.com/api/y';
COMMENT ON COLUMN alert_rule.append_tags IS 'split by space: service=n9e mod=api';

CREATE TABLE alert_mute (
    id bigserial,
    group_id bigint not null default 0 ,
    cate varchar(128) not null default '' ,
    prod varchar(255) NOT NULL DEFAULT '' ,
    note varchar(1024) not null default '',
    cluster varchar(128) not null,
    tags varchar(4096) not null default '' ,
    cause varchar(255) not null default '',
    btime bigint not null default 0 ,
    etime bigint not null default 0 ,
    disabled smallint not null default 0 ,
    create_at bigint not null default 0,
    create_by varchar(64) not null default '',
    update_at bigint not null default 0,
    update_by varchar(64) not null default ''
) ;
ALTER TABLE alert_mute ADD CONSTRAINT alert_mute_pk PRIMARY KEY (id);
CREATE INDEX alert_mute_create_at_idx ON alert_mute (create_at);
CREATE INDEX alert_mute_group_id_idx ON alert_mute (group_id);
COMMENT ON COLUMN alert_mute.group_id IS 'busi group id';
COMMENT ON COLUMN alert_mute.tags IS 'json,map,tagkey->regexp|value';
COMMENT ON COLUMN alert_mute.btime IS 'begin time';
COMMENT ON COLUMN alert_mute.etime IS 'end time';
COMMENT ON COLUMN alert_mute.disabled IS '0:enabled 1:disabled';

CREATE TABLE alert_subscribe (
    id bigserial,
    "name" varchar(255) NOT NULL default '',
    disabled int2 NOT NULL default 0 ,
    group_id bigint not null default 0 ,
    cate varchar(128) not null default '' ,
    cluster varchar(128) not null,
    rule_id bigint not null default 0,
    tags jsonb not null ,
    redefine_severity smallint default 0 ,
    new_severity smallint not null ,
    redefine_channels smallint default 0 ,
    new_channels varchar(255) not null default '' ,
    user_group_ids varchar(250) not null ,
    create_at bigint not null default 0,
    create_by varchar(64) not null default '',
    update_at bigint not null default 0,
    update_by varchar(64) not null default ''
) ;
ALTER TABLE alert_subscribe ADD CONSTRAINT alert_subscribe_pk PRIMARY KEY (id);
CREATE INDEX alert_subscribe_group_id_idx ON alert_subscribe (group_id);
CREATE INDEX alert_subscribe_update_at_idx ON alert_subscribe (update_at);
COMMENT ON COLUMN alert_subscribe.disabled IS '0:enabled 1:disabled';
COMMENT ON COLUMN alert_subscribe.group_id IS 'busi group id';
COMMENT ON COLUMN alert_subscribe.tags IS 'json,map,tagkey->regexp|value';
COMMENT ON COLUMN alert_subscribe.redefine_severity IS 'is redefine severity?';
COMMENT ON COLUMN alert_subscribe.new_severity IS '0:Emergency 1:Warning 2:Notice';
COMMENT ON COLUMN alert_subscribe.redefine_channels IS 'is redefine channels?';
COMMENT ON COLUMN alert_subscribe.new_channels IS 'split by space: sms voice email dingtalk wecom';
COMMENT ON COLUMN alert_subscribe.user_group_ids IS 'split by space 1 34 5, notify cc to user_group_ids';


CREATE TABLE target (
    id bigserial,
    group_id bigint not null default 0 ,
    cluster varchar(128) not null ,
    ident varchar(191) not null ,
    note varchar(255) not null default '' ,
    tags varchar(512) not null default '' ,
    update_at bigint not null default 0
) ;
ALTER TABLE target ADD CONSTRAINT target_pk PRIMARY KEY (id);
ALTER TABLE target ADD CONSTRAINT target_un UNIQUE (ident);
CREATE INDEX target_group_id_idx ON target (group_id);

COMMENT ON COLUMN target.group_id IS 'busi group id';
COMMENT ON COLUMN target."cluster" IS 'append to alert event as field';
COMMENT ON COLUMN target.ident IS 'target id';
COMMENT ON COLUMN target.note IS 'append to alert event as field';
COMMENT ON COLUMN target.tags IS 'append to series data as tags, split by space, append external space at suffix';


CREATE TABLE metric_view (
    id bigserial,
    name varchar(191) not null default '',
    cate smallint not null ,
    configs varchar(8192) not null default '',
    create_at bigint not null default 0,
    create_by bigint not null default 0 ,
    update_at bigint not null default 0
) ;
ALTER TABLE metric_view ADD CONSTRAINT metric_view_pk PRIMARY KEY (id);
CREATE INDEX metric_view_create_by_idx ON metric_view (create_by);

COMMENT ON COLUMN metric_view.cate IS '0: preset 1: custom';
COMMENT ON COLUMN metric_view.create_by IS 'user id';

insert into metric_view(name, cate, configs) values('Host View', 0, '{"filters":[{"oper":"=","label":"__name__","value":"cpu_usage_idle"}],"dynamicLabels":[],"dimensionLabels":[{"label":"ident","value":""}]}');

CREATE TABLE alert_aggr_view (
    id bigserial,
    name varchar(191) not null default '',
    rule varchar(2048) not null default '',
    cate smallint not null ,
    create_at bigint not null default 0,
    create_by bigint not null default 0 ,
    update_at bigint not null default 0
) ;
ALTER TABLE alert_aggr_view ADD CONSTRAINT alert_aggr_view_pk PRIMARY KEY (id);
CREATE INDEX alert_aggr_view_create_by_idx ON alert_aggr_view (create_by);

COMMENT ON COLUMN alert_aggr_view.cate IS '0: preset 1: custom';
COMMENT ON COLUMN alert_aggr_view.create_by IS 'user id';

insert into alert_aggr_view(name, rule, cate) values('By BusiGroup, Severity', 'field:group_name::field:severity', 0);
insert into alert_aggr_view(name, rule, cate) values('By RuleName', 'field:rule_name', 0);

CREATE TABLE alert_cur_event (
    id bigserial NOT NULL,
    cate varchar(128) not null default '' ,
    "cluster" varchar(128) NOT NULL,
    group_id int8 NOT NULL,
    group_name varchar(255) NOT NULL DEFAULT ''::character varying,
    hash varchar(64) NOT NULL,
    rule_id int8 NOT NULL,
    rule_name varchar(255) NOT NULL,
    rule_note varchar(2048) NOT NULL DEFAULT 'alert rule note'::character varying,
    severity int2 NOT NULL,
    prom_for_duration int4 NOT NULL,
    prom_ql varchar(8192) NOT NULL,
    prom_eval_interval int4 NOT NULL,
    callbacks varchar(255) NOT NULL DEFAULT ''::character varying,
    runbook_url varchar(255) NULL,
    notify_recovered int2 NOT NULL,
    notify_channels varchar(255) NOT NULL DEFAULT ''::character varying,
    notify_groups varchar(255) NOT NULL DEFAULT ''::character varying,
    notify_repeat_next int8 NOT NULL DEFAULT 0,
    notify_cur_number int4 not null default 0,
    target_ident varchar(191) NOT NULL DEFAULT ''::character varying,
    target_note varchar(191) NOT NULL DEFAULT ''::character varying,
    first_trigger_time int8,
    trigger_time int8 NOT NULL,
    trigger_value varchar(255) NOT NULL,
    tags varchar(1024) NOT NULL DEFAULT ''::character varying,
    rule_prod varchar(255) NOT NULL DEFAULT ''::character varying,
    rule_algo varchar(255) NOT NULL DEFAULT ''::character varying,
    CONSTRAINT alert_cur_event_pk PRIMARY KEY (id)
);
CREATE INDEX alert_cur_event_hash_idx ON alert_cur_event USING btree (hash);
CREATE INDEX alert_cur_event_notify_repeat_next_idx ON alert_cur_event USING btree (notify_repeat_next);
CREATE INDEX alert_cur_event_rule_id_idx ON alert_cur_event USING btree (rule_id);
CREATE INDEX alert_cur_event_trigger_time_idx ON alert_cur_event USING btree (trigger_time, group_id);
COMMENT ON COLUMN alert_cur_event.id IS 'use alert_his_event.id';
COMMENT ON COLUMN alert_cur_event.group_id IS 'busi group id of rule';
COMMENT ON COLUMN alert_cur_event.group_name IS 'busi group name';
COMMENT ON COLUMN alert_cur_event.hash IS 'rule_id + vector_pk';
COMMENT ON COLUMN alert_cur_event.rule_note IS 'alert rule note';
COMMENT ON COLUMN alert_cur_event.severity IS '1:Emergency 2:Warning 3:Notice';
COMMENT ON COLUMN alert_cur_event.prom_for_duration IS 'prometheus for, unit:s';
COMMENT ON COLUMN alert_cur_event.prom_ql IS 'promql';
COMMENT ON COLUMN alert_cur_event.prom_eval_interval IS 'evaluate interval';
COMMENT ON COLUMN alert_cur_event.callbacks IS 'split by space: http://a.com/api/x http://a.com/api/y';
COMMENT ON COLUMN alert_cur_event.notify_recovered IS 'whether notify when recovery';
COMMENT ON COLUMN alert_cur_event.notify_channels IS 'split by space: sms voice email dingtalk wecom';
COMMENT ON COLUMN alert_cur_event.notify_groups IS 'split by space: 233 43';
COMMENT ON COLUMN alert_cur_event.notify_repeat_next IS 'next timestamp to notify, get repeat settings from rule';
COMMENT ON COLUMN alert_cur_event.target_ident IS 'target ident, also in tags';
COMMENT ON COLUMN alert_cur_event.target_note IS 'target note';
COMMENT ON COLUMN alert_cur_event.tags IS 'merge data_tags rule_tags, split by ,,';

CREATE TABLE alert_his_event (
    id bigserial NOT NULL,
    is_recovered int2 NOT NULL,
    cate varchar(128) not null default '' ,
    "cluster" varchar(128) NOT NULL,
    group_id int8 NOT NULL,
    group_name varchar(255) NOT NULL DEFAULT ''::character varying,
    hash varchar(64) NOT NULL,
    rule_id int8 NOT NULL,
    rule_name varchar(255) NOT NULL,
    rule_note varchar(2048) NOT NULL DEFAULT 'alert rule note'::character varying,
    severity int2 NOT NULL,
    prom_for_duration int4 NOT NULL,
    prom_ql varchar(8192) NOT NULL,
    prom_eval_interval int4 NOT NULL,
    callbacks varchar(255) NOT NULL DEFAULT ''::character varying,
    runbook_url varchar(255) NULL,
    notify_recovered int2 NOT NULL,
    notify_channels varchar(255) NOT NULL DEFAULT ''::character varying,
    notify_groups varchar(255) NOT NULL DEFAULT ''::character varying,
    notify_cur_number int4 not null default 0,
    target_ident varchar(191) NOT NULL DEFAULT ''::character varying,
    target_note varchar(191) NOT NULL DEFAULT ''::character varying,
    first_trigger_time int8,
    trigger_time int8 NOT NULL,
    trigger_value varchar(255) NOT NULL,
    recover_time int8 NOT NULL DEFAULT 0,
    last_eval_time int8 NOT NULL DEFAULT 0,
    tags varchar(1024) NOT NULL DEFAULT ''::character varying,
    rule_prod varchar(255) NOT NULL DEFAULT ''::character varying,
    rule_algo varchar(255) NOT NULL DEFAULT ''::character varying,
    CONSTRAINT alert_his_event_pk PRIMARY KEY (id)
);
CREATE INDEX alert_his_event_hash_idx ON alert_his_event USING btree (hash);
CREATE INDEX alert_his_event_rule_id_idx ON alert_his_event USING btree (rule_id);
CREATE INDEX alert_his_event_trigger_time_idx ON alert_his_event USING btree (trigger_time, group_id);

COMMENT ON COLUMN alert_his_event.group_id IS 'busi group id of rule';
COMMENT ON COLUMN alert_his_event.group_name IS 'busi group name';
COMMENT ON COLUMN alert_his_event.hash IS 'rule_id + vector_pk';
COMMENT ON COLUMN alert_his_event.rule_note IS 'alert rule note';
COMMENT ON COLUMN alert_his_event.severity IS '0:Emergency 1:Warning 2:Notice';
COMMENT ON COLUMN alert_his_event.prom_for_duration IS 'prometheus for, unit:s';
COMMENT ON COLUMN alert_his_event.prom_ql IS 'promql';
COMMENT ON COLUMN alert_his_event.prom_eval_interval IS 'evaluate interval';
COMMENT ON COLUMN alert_his_event.callbacks IS 'split by space: http://a.com/api/x http://a.com/api/y';
COMMENT ON COLUMN alert_his_event.notify_recovered IS 'whether notify when recovery';
COMMENT ON COLUMN alert_his_event.notify_channels IS 'split by space: sms voice email dingtalk wecom';
COMMENT ON COLUMN alert_his_event.notify_groups IS 'split by space: 233 43';
COMMENT ON COLUMN alert_his_event.target_ident IS 'target ident, also in tags';
COMMENT ON COLUMN alert_his_event.target_note IS 'target note';
COMMENT ON COLUMN alert_his_event.last_eval_time IS 'for time filter';
COMMENT ON COLUMN alert_his_event.tags IS 'merge data_tags rule_tags, split by ,,';

CREATE TABLE task_tpl
(
    id        serial,
    group_id  int  not null ,
    title     varchar(255) not null default '',
    account   varchar(64)  not null,
    batch     int  not null default 0,
    tolerance int  not null default 0,
    timeout   int  not null default 0,
    pause     varchar(255) not null default '',
    script    text         not null,
    args      varchar(512) not null default '',
    tags      varchar(255) not null default '' ,
    create_at bigint not null default 0,
    create_by varchar(64) not null default '',
    update_at bigint not null default 0,
    update_by varchar(64) not null default ''
) ;
ALTER TABLE task_tpl ADD CONSTRAINT task_tpl_pk PRIMARY KEY (id);
CREATE INDEX task_tpl_group_id_idx ON task_tpl (group_id);

COMMENT ON COLUMN task_tpl.group_id IS 'busi group id';
COMMENT ON COLUMN task_tpl.tags IS 'split by space';


CREATE TABLE task_tpl_host
(
    ii   serial,
    id   int  not null ,
    host varchar(128)  not null 
) ;
ALTER TABLE task_tpl_host ADD CONSTRAINT task_tpl_host_pk PRIMARY KEY (id);
CREATE INDEX task_tpl_host_id_idx ON task_tpl_host (id,host);

COMMENT ON COLUMN task_tpl_host.id IS 'task tpl id';
COMMENT ON COLUMN task_tpl_host.host IS 'ip or hostname';

CREATE TABLE task_record
(
    id bigserial,
    group_id bigint not null ,
    ibex_address   varchar(128) not null,
    ibex_auth_user varchar(128) not null default '',
    ibex_auth_pass varchar(128) not null default '',
    title     varchar(255)    not null default '',
    account   varchar(64)     not null,
    batch     int     not null default 0,
    tolerance int     not null default 0,
    timeout   int     not null default 0,
    pause     varchar(255)    not null default '',
    script    text            not null,
    args      varchar(512)    not null default '',
    create_at bigint not null default 0,
    create_by varchar(64) not null default ''
) ;
ALTER TABLE task_record ADD CONSTRAINT task_record_pk PRIMARY KEY (id);
CREATE INDEX task_record_create_at_idx ON task_record (create_at,group_id);
CREATE INDEX task_record_create_by_idx ON task_record (create_by);

COMMENT ON COLUMN task_record.id IS 'ibex task id';
COMMENT ON COLUMN task_record.group_id IS 'busi group id';

CREATE TABLE alerting_engines
(
    id bigserial NOT NULL,
    instance varchar(128) not null default '',
    cluster varchar(128) not null default '',
    clock bigint not null
) ;
ALTER TABLE alerting_engines ADD CONSTRAINT alerting_engines_pk PRIMARY KEY (id);
COMMENT ON COLUMN alerting_engines.instance IS 'instance identification, e.g. 10.9.0.9:9090';
COMMENT ON COLUMN alerting_engines.cluster IS 'target reader cluster';
