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
    update_by varchar(64) not null default '',
    PRIMARY KEY (id),
    UNIQUE (username)
) ;
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
    update_by varchar(64) not null default '',
    PRIMARY KEY (id)
) ;
CREATE INDEX user_group_create_by_idx ON user_group (create_by);
CREATE INDEX user_group_update_at_idx ON user_group (update_at);

insert into user_group(id, name, create_at, create_by, update_at, update_by) values(1, 'demo-root-group', date_part('epoch',current_timestamp)::int, 'root', date_part('epoch',current_timestamp)::int, 'root');

CREATE TABLE user_group_member (
    id bigserial,
    group_id bigint  not null,
    user_id bigint  not null,
    PRIMARY KEY(id)
) ;
CREATE INDEX user_group_member_group_id_idx ON user_group_member (group_id);
CREATE INDEX user_group_member_user_id_idx ON user_group_member (user_id);

insert into user_group_member(group_id, user_id) values(1, 1);

CREATE TABLE configs (
    id bigserial,
    ckey varchar(191) not null,
    cval text not null default '',
    PRIMARY KEY (id),
    UNIQUE (ckey)
) ;

CREATE TABLE role (
    id bigserial,
    name varchar(191) not null default '',
    note varchar(255) not null default '',
    PRIMARY KEY (id),
    UNIQUE (name)
) ;

insert into role(name, note) values('Admin', 'Administrator role');
insert into role(name, note) values('Standard', 'Ordinary user role');
insert into role(name, note) values('Guest', 'Readonly user role');

CREATE TABLE role_operation(
    id bigserial,
    role_name varchar(128) not null,
    operation varchar(191) not null,
    PRIMARY KEY(id)
) ;
CREATE INDEX role_operation_role_name_idx ON role_operation (role_name);
CREATE INDEX role_operation_operation_idx ON role_operation (operation);


-- Admin is special, who has no concrete operation but can do anything.
insert into role_operation(role_name, operation) values('Guest', '/metric/explorer');
insert into role_operation(role_name, operation) values('Guest', '/object/explorer');
insert into role_operation(role_name, operation) values('Guest', '/log/explorer');
insert into role_operation(role_name, operation) values('Guest', '/trace/explorer');
insert into role_operation(role_name, operation) values('Guest', '/help/version');
insert into role_operation(role_name, operation) values('Guest', '/help/contact');

insert into role_operation(role_name, operation) values('Standard', '/metric/explorer');
insert into role_operation(role_name, operation) values('Standard', '/object/explorer');
insert into role_operation(role_name, operation) values('Standard', '/log/explorer');
insert into role_operation(role_name, operation) values('Standard', '/trace/explorer');
insert into role_operation(role_name, operation) values('Standard', '/help/version');
insert into role_operation(role_name, operation) values('Standard', '/help/contact');
insert into role_operation(role_name, operation) values('Standard', '/help/servers');
insert into role_operation(role_name, operation) values('Standard', '/help/migrate');

insert into role_operation(role_name, operation) values('Standard', '/alert-rules-built-in');
insert into role_operation(role_name, operation) values('Standard', '/dashboards-built-in');
insert into role_operation(role_name, operation) values('Standard', '/trace/dependencies');

insert into role_operation(role_name, operation) values('Admin', '/help/source');
insert into role_operation(role_name, operation) values('Admin', '/help/sso');
insert into role_operation(role_name, operation) values('Admin', '/help/notification-tpls');
insert into role_operation(role_name, operation) values('Admin', '/help/notification-settings');

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

-- for alert_rule | collect_rule | mute | dashboard grouping
CREATE TABLE busi_group (
    id bigserial,
    name varchar(191) not null,
    label_enable smallint not null default 0,
    label_value varchar(191) not null default '' ,
    create_at bigint not null default 0,
    create_by varchar(64) not null default '',
    update_at bigint not null default 0,
    update_by varchar(64) not null default '',
    PRIMARY KEY (id),
    UNIQUE (name)
) ;
COMMENT ON COLUMN busi_group.label_value IS 'if label_enable: label_value can not be blank';

insert into busi_group(id, name, create_at, create_by, update_at, update_by) values(1, 'Default Busi Group', date_part('epoch',current_timestamp)::int, 'root', date_part('epoch',current_timestamp)::int, 'root');

CREATE TABLE busi_group_member (
    id bigserial,
    busi_group_id bigint not null ,
    user_group_id bigint not null ,
    perm_flag char(2) not null ,
    PRIMARY KEY (id)
) ;
CREATE INDEX busi_group_member_busi_group_id_idx ON busi_group_member (busi_group_id);
CREATE INDEX busi_group_member_user_group_id_idx ON busi_group_member (user_group_id);
COMMENT ON COLUMN busi_group_member.busi_group_id IS 'busi group id';
COMMENT ON COLUMN busi_group_member.user_group_id IS 'user group id';
COMMENT ON COLUMN busi_group_member.perm_flag IS 'ro | rw';


insert into busi_group_member(busi_group_id, user_group_id, perm_flag) values(1, 1, 'rw');

-- for dashboard new version
CREATE TABLE board (
    id bigserial,
    group_id bigint not null default 0 ,
    name varchar(191) not null,
    ident varchar(200) not null default '',
    tags varchar(255) not null ,
    public smallint not null default 0 ,
    built_in smallint not null default 0 ,
    hide smallint not null default 0 ,
    create_at bigint not null default 0,
    create_by varchar(64) not null default '',
    update_at bigint not null default 0,
    update_by varchar(64) not null default '',
    PRIMARY KEY (id),
    UNIQUE (group_id, name)
) ;
CREATE INDEX board_ident_idx ON board (ident);
COMMENT ON COLUMN board.group_id IS 'busi group id';
COMMENT ON COLUMN board.tags IS 'split by space';
COMMENT ON COLUMN board.public IS '0:false 1:true';
COMMENT ON COLUMN board.built_in IS '0:false 1:true';
COMMENT ON COLUMN board.hide IS '0:false 1:true';


-- for dashboard new version
CREATE TABLE board_payload (
    id bigint  not null ,
    payload text not null,
    UNIQUE (id)
) ;
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
    update_by varchar(64) not null default '',
    PRIMARY KEY (id),
    UNIQUE (group_id, name)
) ;
COMMENT ON COLUMN dashboard.group_id IS 'busi group id';
COMMENT ON COLUMN dashboard.tags IS 'split by space';
COMMENT ON COLUMN dashboard.configs IS 'dashboard variables';

-- deprecated
-- auto create the first subclass 'Default chart group' of dashboard
CREATE TABLE chart_group (
    id bigserial,
    dashboard_id bigint  not null,
    name varchar(255) not null,
    weight int not null default 0,
    PRIMARY KEY (id)
) ;
CREATE INDEX chart_group_dashboard_id_idx ON chart_group (dashboard_id);

-- deprecated
CREATE TABLE chart (
    id bigserial,
    group_id bigint  not null ,
    configs text,
    weight int not null default 0,
    PRIMARY KEY (id)
) ;
CREATE INDEX chart_group_id_idx ON chart (group_id);
COMMENT ON COLUMN chart.group_id IS 'chart group id';


CREATE TABLE chart_share (
    id bigserial,
    cluster varchar(128) not null,
    datasource_id bigint  not null default 0,
    configs text,
    create_at bigint not null default 0,
    create_by varchar(64) not null default '',
    primary key (id)
) ;
CREATE INDEX chart_share_create_at_idx ON chart_share (create_at);


CREATE TABLE alert_rule (
    id bigserial,
    group_id bigint not null default 0 ,
    cate varchar(128) not null,
    datasource_ids varchar(255) not null default '' ,
    cluster varchar(128) not null,
    name varchar(255) not null,
    note varchar(1024) not null default '',
    prod varchar(255) not null default '',
    algorithm varchar(255) not null default '',
    algo_params varchar(255),
    delay int not null default 0,
    severity smallint not null ,
    disabled smallint not null ,
    prom_for_duration int not null ,
    rule_config text not null ,
    prom_ql text not null ,
    prom_eval_interval int not null ,
    enable_stime varchar(255) not null default '00:00',
    enable_etime varchar(255) not null default '23:59',
    enable_days_of_week varchar(255) not null default '' ,
    enable_in_bg smallint not null default 0 ,
    notify_recovered smallint not null ,
    notify_channels varchar(255) not null default '' ,
    notify_groups varchar(255) not null default '' ,
    notify_repeat_step int not null default 0 ,
    notify_max_number int not null default 0 ,
    recover_duration int not null default 0 ,
    callbacks varchar(255) not null default '' ,
    runbook_url varchar(255),
    append_tags varchar(255) not null default '' ,
    annotations text not null ,
    extra_config text not null ,
    create_at bigint not null default 0,
    create_by varchar(64) not null default '',
    update_at bigint not null default 0,
    update_by varchar(64) not null default '',
    PRIMARY KEY (id)
) ;
CREATE INDEX alert_rule_group_id_idx ON alert_rule (group_id);
CREATE INDEX alert_rule_update_at_idx ON alert_rule (update_at);
COMMENT ON COLUMN alert_rule.group_id IS 'busi group id';
COMMENT ON COLUMN alert_rule.datasource_ids IS 'datasource ids';
COMMENT ON COLUMN alert_rule.severity IS '1:Emergency 2:Warning 3:Notice';
COMMENT ON COLUMN alert_rule.disabled IS '0:enabled 1:disabled';
COMMENT ON COLUMN alert_rule.prom_for_duration IS 'prometheus for, unit:s';
COMMENT ON COLUMN alert_rule.rule_config IS 'rule_config';
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
COMMENT ON COLUMN alert_rule.annotations IS 'annotations';
COMMENT ON COLUMN alert_rule.extra_config IS 'extra_config';

CREATE TABLE alert_mute (
    id bigserial,
    group_id bigint not null default 0 ,
    prod varchar(255) not null default '',
    note varchar(1024) not null default '',
    cate varchar(128) not null,
    cluster varchar(128) not null,
    datasource_ids varchar(255) not null default '' ,
    tags jsonb NOT NULL ,
    cause varchar(255) not null default '',
    btime bigint not null default 0 ,
    etime bigint not null default 0 ,
    disabled smallint not null default 0 ,
    mute_time_type smallint not null default 0,
    periodic_mutes varchar(4096) not null default '',
    severities varchar(32) not null default '',
    create_at bigint not null default 0,
    create_by varchar(64) not null default '',
    update_at bigint not null default 0,
    update_by varchar(64) not null default '',
    PRIMARY KEY (id)
) ;
CREATE INDEX alert_mute_group_id_idx ON alert_mute (group_id);
CREATE INDEX alert_mute_update_at_idx ON alert_mute (update_at);
COMMENT ON COLUMN alert_mute.group_id IS 'busi group id';
COMMENT ON COLUMN alert_mute.datasource_ids IS 'datasource ids';
COMMENT ON COLUMN alert_mute.tags IS 'json,map,tagkey->regexp|value';
COMMENT ON COLUMN alert_mute.btime IS 'begin time';
COMMENT ON COLUMN alert_mute.etime IS 'end time';
COMMENT ON COLUMN alert_mute.disabled IS '0:enabled 1:disabled';


CREATE TABLE alert_subscribe (
    id bigserial,
    name varchar(255) not null default '',
    disabled smallint not null default 0 ,
    group_id bigint not null default 0 ,
    prod varchar(255) not null default '',
    cate varchar(128) not null,
    datasource_ids varchar(255) not null default '' ,
    cluster varchar(128) not null,
    rule_id bigint not null default 0,
    severities varchar(32) not null default '',
    tags varchar(4096) not null default '[]',
    redefine_severity smallint default 0 ,
    new_severity smallint not null ,
    redefine_channels smallint default 0 ,
    new_channels varchar(255) not null default '' ,
    user_group_ids varchar(250) not null ,
    webhooks text not null,
    extra_config text not null,
    redefine_webhooks smallint default 0,
    for_duration bigint not null default 0,
    create_at bigint not null default 0,
    create_by varchar(64) not null default '',
    update_at bigint not null default 0,
    update_by varchar(64) not null default '',
    PRIMARY KEY (id)
) ;
CREATE INDEX alert_subscribe_group_id_idx ON alert_subscribe (group_id);
CREATE INDEX alert_subscribe_update_at_idx ON alert_subscribe (update_at);
COMMENT ON COLUMN alert_subscribe.disabled IS '0:enabled 1:disabled';
COMMENT ON COLUMN alert_subscribe.group_id IS 'busi group id';
COMMENT ON COLUMN alert_subscribe.datasource_ids IS 'datasource ids';
COMMENT ON COLUMN alert_subscribe.tags IS 'json,map,tagkey->regexp|value';
COMMENT ON COLUMN alert_subscribe.redefine_severity IS 'is redefine severity?';
COMMENT ON COLUMN alert_subscribe.new_severity IS '0:Emergency 1:Warning 2:Notice';
COMMENT ON COLUMN alert_subscribe.redefine_channels IS 'is redefine channels?';
COMMENT ON COLUMN alert_subscribe.new_channels IS 'split by space: sms voice email dingtalk wecom';
COMMENT ON COLUMN alert_subscribe.user_group_ids IS 'split by space 1 34 5, notify cc to user_group_ids';
COMMENT ON COLUMN alert_subscribe.extra_config IS 'extra_config';


CREATE TABLE target (
    id bigserial,
    group_id bigint not null default 0 ,
    ident varchar(191) not null ,
    note varchar(255) not null default '' ,
    tags varchar(512) not null default '' ,
    update_at bigint not null default 0,
    PRIMARY KEY (id),
    UNIQUE (ident)
) ;
CREATE INDEX target_group_id_idx ON target (group_id);
COMMENT ON COLUMN target.group_id IS 'busi group id';
COMMENT ON COLUMN target.ident IS 'target id';
COMMENT ON COLUMN target.note IS 'append to alert event as field';
COMMENT ON COLUMN target.tags IS 'append to series data as tags, split by space, append external space at suffix';

-- case1: target_idents; case2: target_tags
-- CREATE TABLE collect_rule (
--     id bigserial,
--     group_id bigint not null default 0 comment 'busi group id',
--     cluster varchar(128) not null,
--     target_idents varchar(512) not null default '' comment 'ident list, split by space',
--     target_tags varchar(512) not null default '' comment 'filter targets by tags, split by space',
--     name varchar(191) not null default '',
--     note varchar(255) not null default '',
--     step int not null,
--     type varchar(64) not null comment 'e.g. port proc log plugin',
--     data text not null,
--     append_tags varchar(255) not null default '' comment 'split by space: e.g. mod=n9e dept=cloud',
--     create_at bigint not null default 0,
--     create_by varchar(64) not null default '',
--     update_at bigint not null default 0,
--     update_by varchar(64) not null default '',
--     PRIMARY KEY (id),
--     KEY (group_id, type, name)
-- ) ;

CREATE TABLE metric_view (
    id bigserial,
    name varchar(191) not null default '',
    cate smallint not null ,
    configs varchar(8192) not null default '',
    create_at bigint not null default 0,
    create_by bigint not null default 0,
    update_at bigint not null default 0,
    PRIMARY KEY (id)
) ;
CREATE INDEX metric_view_create_by_idx ON metric_view (create_by);
COMMENT ON COLUMN metric_view.cate IS '0: preset 1: custom';
COMMENT ON COLUMN metric_view.create_by IS 'user id';


insert into metric_view(name, cate, configs) values('Host View', 0, '{"filters":[{"oper":"=","label":"__name__","value":"cpu_usage_idle"}],"dynamicLabels":[],"dimensionLabels":[{"label":"ident","value":""}]}');
 
CREATE TABLE recording_rule (
    id bigserial,
    group_id bigint not null default '0',
    datasource_ids varchar(255) not null default '',
    cluster varchar(128) not null,
    name varchar(255) not null ,
    note varchar(255) not null ,
    disabled smallint not null default 0 ,
    prom_ql varchar(8192) not null ,
    prom_eval_interval int not null ,
    append_tags varchar(255) default '' ,
    query_configs text not null ,
    create_at bigint default '0',
    create_by varchar(64) default '',
    update_at bigint default '0',
    update_by varchar(64) default '',
    PRIMARY KEY (id)
) ;
CREATE INDEX recording_rule_group_id_idx ON recording_rule (group_id);
CREATE INDEX recording_rule_update_at_idx ON recording_rule (update_at);
COMMENT ON COLUMN recording_rule.group_id IS 'group_id';
COMMENT ON COLUMN recording_rule.datasource_ids IS 'datasource ids';
COMMENT ON COLUMN recording_rule.name IS 'new metric name';
COMMENT ON COLUMN recording_rule.note IS 'rule note';
COMMENT ON COLUMN recording_rule.disabled IS '0:enabled 1:disabled';
COMMENT ON COLUMN recording_rule.prom_ql IS 'promql';
COMMENT ON COLUMN recording_rule.prom_eval_interval IS 'evaluate interval';
COMMENT ON COLUMN recording_rule.append_tags IS 'split by space: service=n9e mod=api';
COMMENT ON COLUMN recording_rule.query_configs IS 'query configs';


CREATE TABLE alert_aggr_view (
    id bigserial,
    name varchar(191) not null default '',
    rule varchar(2048) not null default '',
    cate smallint not null ,
    create_at bigint not null default 0,
    create_by bigint not null default 0,
    update_at bigint not null default 0,
    PRIMARY KEY (id)
) ;
CREATE INDEX alert_aggr_view_create_by_idx ON alert_aggr_view (create_by);
COMMENT ON COLUMN alert_aggr_view.cate IS '0: preset 1: custom';
COMMENT ON COLUMN alert_aggr_view.create_by IS 'user id';


insert into alert_aggr_view(name, rule, cate) values('By BusiGroup, Severity', 'field:group_name::field:severity', 0);
insert into alert_aggr_view(name, rule, cate) values('By RuleName', 'field:rule_name', 0);

CREATE TABLE alert_cur_event (
    id bigint  not null ,
    cate varchar(128) not null,
    datasource_id bigint not null default 0 ,
    cluster varchar(128) not null,
    group_id bigint  not null ,
    group_name varchar(255) not null default '' ,
    hash varchar(64) not null ,
    rule_id bigint  not null,
    rule_name varchar(255) not null,
    rule_note varchar(2048) not null ,
    rule_prod varchar(255) not null default '',
    rule_algo varchar(255) not null default '',
    severity smallint not null ,
    prom_for_duration int not null ,
    prom_ql varchar(8192) not null ,
    prom_eval_interval int not null ,
    callbacks varchar(255) not null default '' ,
    runbook_url varchar(255),
    notify_recovered smallint not null ,
    notify_channels varchar(255) not null default '' ,
    notify_groups varchar(255) not null default '' ,
    notify_repeat_next bigint not null default 0 ,
    notify_cur_number int not null default 0 ,
    target_ident varchar(191) not null default '' ,
    target_note varchar(191) not null default '' ,
    first_trigger_time bigint,
    trigger_time bigint not null,
    trigger_value varchar(255) not null,
    annotations text not null ,
    rule_config text not null ,
    tags varchar(1024) not null default '' ,
    PRIMARY KEY (id)
) ;
CREATE INDEX alert_cur_event_hash_idx ON alert_cur_event (hash);
CREATE INDEX alert_cur_event_rule_id_idx ON alert_cur_event (rule_id);
CREATE INDEX alert_cur_event_tg_idx ON alert_cur_event (trigger_time, group_id);
CREATE INDEX alert_cur_event_nrn_idx ON alert_cur_event (notify_repeat_next);
COMMENT ON COLUMN alert_cur_event.id IS 'use alert_his_event.id';
COMMENT ON COLUMN alert_cur_event.datasource_id IS 'datasource id';
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
COMMENT ON COLUMN alert_cur_event.annotations IS 'annotations';
COMMENT ON COLUMN alert_cur_event.rule_config IS 'rule_config';
COMMENT ON COLUMN alert_cur_event.tags IS 'merge data_tags rule_tags, split by ,,';


CREATE TABLE alert_his_event (
    id bigserial,
    is_recovered smallint not null,
    cate varchar(128) not null,
    datasource_id bigint not null default 0 ,
    cluster varchar(128) not null,
    group_id bigint  not null ,
    group_name varchar(255) not null default '' ,
    hash varchar(64) not null ,
    rule_id bigint  not null,
    rule_name varchar(255) not null,
    rule_note varchar(2048) not null default 'alert rule note',
    rule_prod varchar(255) not null default '',
    rule_algo varchar(255) not null default '',
    severity smallint not null ,
    prom_for_duration int not null ,
    prom_ql varchar(8192) not null ,
    prom_eval_interval int not null ,
    callbacks varchar(255) not null default '' ,
    runbook_url varchar(255),
    notify_recovered smallint not null ,
    notify_channels varchar(255) not null default '' ,
    notify_groups varchar(255) not null default '' ,
    notify_cur_number int not null default 0 ,
    target_ident varchar(191) not null default '' ,
    target_note varchar(191) not null default '' ,
    first_trigger_time bigint,
    trigger_time bigint not null,
    trigger_value varchar(255) not null,
    recover_time bigint not null default 0,
    last_eval_time bigint not null default 0 ,
    tags varchar(1024) not null default '' ,
    annotations text not null ,
    rule_config text not null ,
    PRIMARY KEY (id)
) ;
CREATE INDEX alert_his_event_hash_idx ON alert_his_event (hash);
CREATE INDEX alert_his_event_rule_id_idx ON alert_his_event (rule_id);
CREATE INDEX alert_his_event_tg_idx ON alert_his_event (trigger_time, group_id);
COMMENT ON COLUMN alert_his_event.group_id IS 'busi group id of rule';
COMMENT ON COLUMN alert_his_event.datasource_id IS 'datasource id';
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
COMMENT ON COLUMN alert_his_event.annotations IS 'annotations';
COMMENT ON COLUMN alert_his_event.rule_config IS 'rule_config';

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
    update_by varchar(64) not null default '',
    PRIMARY KEY (id)
) ;
CREATE INDEX task_tpl_group_id_idx ON task_tpl (group_id);
COMMENT ON COLUMN task_tpl.group_id IS 'busi group id';
COMMENT ON COLUMN task_tpl.tags IS 'split by space';


CREATE TABLE task_tpl_host
(
    ii   serial,
    id   int  not null ,
    host varchar(128)  not null ,
    PRIMARY KEY (ii)
) ;
CREATE INDEX task_tpl_host_id_host_idx ON task_tpl_host (id, host);
COMMENT ON COLUMN task_tpl_host.id IS 'task tpl id';
COMMENT ON COLUMN task_tpl_host.host IS 'ip or hostname';


CREATE TABLE task_record
(
    id bigint  not null ,
    event_id bigint not null default 0,
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
    create_by varchar(64) not null default '',
    PRIMARY KEY (id)
) ;
CREATE INDEX task_record_cg_idx ON task_record (create_at, group_id);
CREATE INDEX task_record_create_by_idx ON task_record (create_by);
CREATE INDEX task_record_event_id_idx ON task_record (event_id);
COMMENT ON COLUMN task_record.id IS 'ibex task id';
COMMENT ON COLUMN task_record.group_id IS 'busi group id';
COMMENT ON COLUMN task_record.event_id IS 'event id';

CREATE TABLE alerting_engines
(
    id serial,
    instance varchar(128) not null default '' ,
    datasource_id bigint not null default 0 ,
    engine_cluster varchar(128) not null default '' ,
    clock bigint not null,
    PRIMARY KEY (id)
) ;
COMMENT ON COLUMN alerting_engines.instance IS 'instance identification, e.g. 10.9.0.9:9090';
COMMENT ON COLUMN alerting_engines.datasource_id IS 'datasource id';
COMMENT ON COLUMN alerting_engines.engine_cluster IS 'target reader cluster';


CREATE TABLE datasource
(
    id serial,
    name varchar(191) not null default '',
    description varchar(255) not null default '',
    category varchar(255) not null default '',
    plugin_id int  not null default 0,
    plugin_type varchar(255) not null default '',
    plugin_type_name varchar(255) not null default '',
    cluster_name varchar(255) not null default '',
    settings text not null,
    status varchar(255) not null default '',
    http varchar(4096) not null default '',
    auth varchar(8192) not null default '',
    created_at bigint not null default 0,
    created_by varchar(64) not null default '',
    updated_at bigint not null default 0,
    updated_by varchar(64) not null default '',
    UNIQUE (name),
    PRIMARY KEY (id)
) ; 
  
CREATE TABLE builtin_cate (
    id bigserial,
    name varchar(191) not null,
    user_id bigint not null default 0,
    PRIMARY KEY (id)
) ;
 
CREATE TABLE notify_tpl (
    id bigserial,
    channel varchar(32) not null,
    name varchar(255) not null,
    content text not null,
    PRIMARY KEY (id),
    UNIQUE (channel)
) ;

CREATE TABLE sso_config (
    id bigserial,
    name varchar(191) not null,
    content text not null,
    PRIMARY KEY (id),
    UNIQUE (name)
) ;


CREATE TABLE es_index_pattern (
    id bigserial,
    datasource_id bigint not null default 0,
    name varchar(191) not null,
    time_field varchar(128) not null default '@timestamp',
    allow_hide_system_indices smallint not null default 0,
    fields_format varchar(4096) not null default '',
    create_at bigint default '0',
    create_by varchar(64) default '',
    update_at bigint default '0',
    update_by varchar(64) default '',
    PRIMARY KEY (id),
    UNIQUE (datasource_id, name)
) ;
COMMENT ON COLUMN es_index_pattern.datasource_id IS 'datasource id';
