CREATE TABLE `privilege` (
  `id`      bigint unsigned NOT NULL AUTO_INCREMENT,
  `pid`     bigint unsigned NOT NULL,
  `typ`     varchar(64) NOT NULL,
  `cn`      varchar(128) NOT NULL,
  `en`      varchar(128) NOT NULL,
  `weight`  int NOT NULL,
  `path`    varchar(128) NOT NULL,
  `leaf`    tinyint(1) NOT NULL,
  `last_updater` varchar(64) NOT NULL,
  `last_updated` timestamp NOT NULL default CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
   PRIMARY KEY (`id`),
   KEY (`typ`),
   KEY (`path`)
) ENGINE = InnoDB DEFAULT CHARACTER SET = utf8;
