-- Note: These are actually views in the production database, we put just enough data in here to test the business logic
DROP TABLE IF EXISTS `recentchanges`;
CREATE TABLE `recentchanges`
(
    rc_id         int(10) unsigned    NOT NULL DEFAULT 0,
    rc_timestamp  binary(14)          NOT NULL,
    rc_actor      decimal(20, 0) NOT NULL DEFAULT 0,
    rc_namespace  int(11)             NOT NULL DEFAULT 0,
    rc_title      varbinary(255)      NOT NULL,
    rc_comment_id decimal(20, 0) NOT NULL DEFAULT 0,
    rc_minor      tinyint(3) unsigned NOT NULL DEFAULT 0,
    rc_bot        tinyint(3) unsigned NOT NULL DEFAULT 0,
    rc_new        tinyint(3) unsigned NOT NULL DEFAULT 0,
    rc_cur_id     int(10) unsigned    NOT NULL DEFAULT 0,
    rc_this_oldid int(10) unsigned    NOT NULL DEFAULT 0,
    rc_last_oldid int(10) unsigned    NOT NULL DEFAULT 0,
    rc_type       tinyint(3) unsigned NOT NULL DEFAULT 0,
    rc_source     varbinary(16)       NOT NULL,
    rc_patrolled  tinyint(3) unsigned NOT NULL DEFAULT 0,
    rc_ip         binary(0)           NULL     DEFAULT NULL,
    rc_old_len    int(10)             NULL     DEFAULT NULL,
    rc_new_len    int(10)             NULL     DEFAULT NULL,
    rc_deleted    tinyint(1) unsigned NOT NULL DEFAULT 0,
    rc_logid      int(10) unsigned    NOT NULL DEFAULT 0,
    rc_log_type   varbinary(255)      NULL     DEFAULT NULL,
    rc_log_action varbinary(255)      NULL     DEFAULT NULL,
    rc_params     blob NULL     DEFAULT NULL
) ENGINE = InnoDB
  DEFAULT CHARSET = latin1;

DROP TABLE IF EXISTS `page`;
CREATE TABLE `page`
(
    page_id            int     NOT NULL,
    page_namespace     int     NOT NULL,
    page_title         varbinary(255) NOT NULL,
    page_is_redirect   tinyint NOT NULL,
    page_is_new        tinyint NOT NULL,
    page_random double NOT NULL,
    page_touched       binary  NOT NULL,
    page_links_updated varbinary(255) NOT NULL,
    page_latest        int     NOT NULL,
    page_len           int     NOT NULL,
    page_content_model varbinary(255) NOT NULL,
    page_lang          varbinary(255) NOT NULL
) ENGINE = InnoDB
  DEFAULT CHARSET = latin1;

DROP TABLE IF EXISTS `revision`;
CREATE TABLE `revision`
(
    rev_id         int     NOT NULL,
    rev_page       int     NOT NULL,
    rev_comment_id decimal NOT NULL,
    rev_actor      bigint  NOT NULL,
    rev_timestamp  binary  NOT NULL,
    rev_minor_edit tinyint NOT NULL,
    rev_deleted    tinyint NOT NULL,
    rev_len        int     NOT NULL,
    rev_parent_id  int     NOT NULL,
    rev_sha1       varbinary(255) NOT NULL
) ENGINE = InnoDB
  DEFAULT CHARSET = latin1;

DROP TABLE IF EXISTS `actor`;
CREATE TABLE `actor`
(
    actor_id   bigint NOT NULL,
    actor_user int    NOT NULL,
    actor_name varbinary(255) NOT NULL
) ENGINE = InnoDB
  DEFAULT CHARSET = latin1;

DROP TABLE IF EXISTS `comment`;
CREATE TABLE `comment`
(
    comment_id   bigint NOT NULL,
    comment_hash int    NOT NULL,
    comment_text blob   NOT NULL,
    comment_data blob   NOT NULL
) ENGINE = InnoDB
  DEFAULT CHARSET = latin1;

DROP TABLE IF EXISTS `revision_userindex`;
CREATE TABLE `revision_userindex`
(
    rev_id         int     NOT NULL,
    rev_page       int     NOT NULL,
    rev_comment_id decimal NOT NULL,
    rev_actor      bigint  NOT NULL,
    rev_timestamp  binary  NOT NULL,
    rev_minor_edit tinyint NOT NULL,
    rev_deleted    tinyint NOT NULL,
    rev_len        int     NOT NULL,
    rev_parent_id  int     NOT NULL,
    rev_sha1       varbinary(255) NOT NULL
) ENGINE = InnoDB
  DEFAULT CHARSET = latin1;

DROP TABLE IF EXISTS `user`;
CREATE TABLE `user`
(
    user_id                  int    NOT NULL,
    user_name                varbinary(255) NOT NULL,
    user_real_name           varbinary(255) NOT NULL,
    user_password            binary NOT NULL,
    user_newpassword         binary NOT NULL,
    user_email               binary NOT NULL,
    user_touched             binary NOT NULL,
    user_token               binary NOT NULL,
    user_email_authenticated binary NOT NULL,
    user_email_token         binary NOT NULL,
    user_email_token_expires binary NOT NULL,
    user_registration        binary NOT NULL,
    user_newpass_time        binary NOT NULL,
    user_editcount           int    NOT NULL,
    user_password_expires    binary NOT NULL
) ENGINE = InnoDB
  DEFAULT CHARSET = latin1;