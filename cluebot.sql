CREATE TABLE IF NOT EXISTS `beaten`
(
    `id`        int(11)      NOT NULL auto_increment,
    `timestamp` timestamp    NOT NULL default CURRENT_TIMESTAMP on update CURRENT_TIMESTAMP,
    `article`   varchar(256) NOT NULL,
    `diff`      varchar(512) NOT NULL,
    `user`      varchar(256) NOT NULL,
    PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=latin1;

CREATE TABLE IF NOT EXISTS `vandalism`
(
    `id`        int(11)       NOT NULL auto_increment,
    `timestamp` timestamp     NOT NULL default CURRENT_TIMESTAMP on update CURRENT_TIMESTAMP,
    `user`      varchar(256)  NOT NULL,
    `article`   varchar(256)  NOT NULL,
    `heuristic` varchar(64)   NOT NULL,
    `regex`     varchar(2048) default NULL,
    `reason`    varchar(512)  NOT NULL,
    `diff`      varchar(512)  NOT NULL,
    `old_id`    int(11)       NOT NULL,
    `new_id`    int(11)       NOT NULL,
    `reverted`  tinyint(1)    NOT NULL,
    PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=latin1;

CREATE TABLE IF NOT EXISTS `last_revert`
(
    `id`    int(11)      NOT NULL auto_increment,
    `title` varchar(256) NOT NULL,
    `user`  varchar(256) NOT NULL,
    `time`  int          NOT NULL,
    PRIMARY KEY (`id`),
    INDEX (`title`, `user`)
) ENGINE=InnoDB DEFAULT CHARSET=latin1;
