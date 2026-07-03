-- 000001_users_and_auth.up.sql
-- Core identity. Customers and the single super-admin share one table,
-- distinguished by `role`. Guests are NOT stored: they browse anonymously and
-- only get a row here once they register in order to transact.

CREATE TABLE users (
    id                BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    role              ENUM('customer','admin') NOT NULL DEFAULT 'customer',
    email             VARCHAR(255) NOT NULL,
    password_hash     VARCHAR(255) NOT NULL,
    full_name         VARCHAR(255) NOT NULL,
    phone             VARCHAR(32)  DEFAULT NULL,
    email_verified_at TIMESTAMP    NULL DEFAULT NULL,
    is_active         BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at        TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_users_email (email)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Saved delivery addresses for customers (reusable at checkout).
-- The order itself snapshots the chosen address, so editing/deleting an
-- address here never mutates a past order.
CREATE TABLE addresses (
    id             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    user_id        BIGINT UNSIGNED NOT NULL,
    label          VARCHAR(64)  DEFAULT NULL,   -- "Home", "Office"
    recipient_name VARCHAR(255) NOT NULL,
    phone          VARCHAR(32)  NOT NULL,
    line1          VARCHAR(255) NOT NULL,
    line2          VARCHAR(255) DEFAULT NULL,
    city           VARCHAR(128) NOT NULL,
    region         VARCHAR(128) DEFAULT NULL,
    country        VARCHAR(128) NOT NULL,
    postal_code    VARCHAR(32)  DEFAULT NULL,
    is_default     BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at     TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_addresses_user (user_id),
    CONSTRAINT fk_addresses_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Opaque refresh tokens for JWT rotation. Store only a SHA-256 hash of the
-- token, never the token itself.
CREATE TABLE refresh_tokens (
    id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    user_id    BIGINT UNSIGNED NOT NULL,
    token_hash CHAR(64)     NOT NULL,          -- SHA-256 hex of the refresh token
    user_agent VARCHAR(255) DEFAULT NULL,
    ip_address VARCHAR(45)  DEFAULT NULL,      -- fits IPv6
    expires_at TIMESTAMP    NOT NULL,
    revoked_at TIMESTAMP    NULL DEFAULT NULL,
    created_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_refresh_token_hash (token_hash),
    KEY idx_refresh_user (user_id),
    CONSTRAINT fk_refresh_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
