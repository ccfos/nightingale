CREATE TABLE task_meta
(
    id        bigserial,
    title     varchar(255)    not null default '',
    account   varchar(64)     not null,
    batch     int     not null default 0,
    tolerance int     not null default 0,
    timeout   int     not null default 0,
    pause     varchar(255)    not null default '',
    script    text            not null,
    args      varchar(512)    not null default '',
    creator   varchar(64)     not null default '',
    created   timestamp       not null default CURRENT_TIMESTAMP,
    PRIMARY KEY (id)
) ;
CREATE INDEX task_meta_creator_idx ON task_meta (creator);
CREATE INDEX task_meta_created_idx ON task_meta (created);

/* start|cancel|kill|pause */
CREATE TABLE task_action
(
    id     bigint  not null,
    action varchar(32)     not null,
    clock  bigint          not null default 0,
    PRIMARY KEY (id)
) ;

CREATE TABLE task_scheduler
(
    id        bigint  not null,
    scheduler varchar(128)    not null default ''
) ;
CREATE INDEX task_scheduler_id_scheduler_idx ON task_scheduler (id, scheduler);


CREATE TABLE task_scheduler_health
(
    scheduler varchar(128) not null,
    clock     bigint       not null,
    UNIQUE (scheduler)
) ;
CREATE INDEX task_scheduler_health_clock_idx ON task_scheduler_health (clock);


CREATE TABLE task_host_doing
(
    id     bigint  not null,
    host   varchar(128)    not null,
    clock  bigint          not null default 0,
    action varchar(16)     not null
) ;
CREATE INDEX task_host_doing_id_idx ON task_host_doing (id);
CREATE INDEX task_host_doing_host_idx ON task_host_doing (host);


CREATE TABLE task_host_0
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_1
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_2
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_3
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_4
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_5
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_6
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_7
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_8
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_9
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_10
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_11
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_12
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_13
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_14
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_15
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_16
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_17
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_18
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_19
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_20
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_21
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_22
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_23
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_24
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_25
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_26
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_27
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_28
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_29
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_30
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_31
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_32
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_33
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_34
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_35
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_36
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_37
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_38
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_39
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_40
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_41
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_42
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_43
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_44
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_45
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_46
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_47
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_48
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_49
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_50
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_51
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_52
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_53
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_54
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_55
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_56
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_57
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_58
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_59
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_60
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_61
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_62
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_63
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_64
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_65
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_66
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_67
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_68
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_69
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_70
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_71
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_72
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_73
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_74
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_75
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_76
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_77
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_78
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_79
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_80
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_81
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_82
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_83
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_84
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_85
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_86
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_87
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_88
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_89
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_90
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_91
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_92
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_93
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_94
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_95
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_96
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_97
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_98
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;

CREATE TABLE task_host_99
(
    ii     bigserial,
    id     bigint  not null,
    host   varchar(128)    not null,
    status varchar(32)     not null,
    stdout text,
    stderr text,
    UNIQUE (id, host),
    PRIMARY KEY (ii)
) ;
