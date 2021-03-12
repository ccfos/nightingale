set names utf8;
use n9e_rdb;

CREATE TABLE `stats`
(
    `name`   varchar(64)  not null,
    `value`  bigint       not null default 0,
    PRIMARY KEY (`name`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8;
