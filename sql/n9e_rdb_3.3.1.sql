alter table user add `organization` varchar(255) not null default '' after intro;
alter table user add `create_at`    timestamp    not null default CURRENT_TIMESTAMP;