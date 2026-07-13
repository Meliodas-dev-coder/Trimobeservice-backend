-- 000019_audit_logs.up.sql
-- Audit trail for the admin back office. As staff accounts start sharing the
-- dashboard, every admin write (create/update/delete) is recorded with WHO did
-- it, WHAT they touched, and WHEN — so actions can be retraced. Rows are written
-- by an audit middleware that wraps RequireAdmin; reads are admin-only.
--
-- actor_user_id has NO foreign key on purpose: the trail must survive even if a
-- staff account is later removed (the id is retained; the display name is
-- resolved by a LEFT JOIN to users at read time). payload stores the (JSON)
-- request body, capped and with secrets redacted by the middleware.

CREATE TABLE audit_logs (
    id             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    actor_user_id  BIGINT UNSIGNED DEFAULT NULL,   -- who acted (nullable: user may be gone)
    method         VARCHAR(10)  NOT NULL,          -- HTTP verb: POST/PUT/PATCH/DELETE
    path           VARCHAR(512) NOT NULL,          -- request path, e.g. /api/v1/admin/orders/5/status
    target_type    VARCHAR(64)  DEFAULT NULL,      -- parsed resource, e.g. "orders"
    target_id      BIGINT UNSIGNED DEFAULT NULL,   -- parsed numeric id from the path
    status_code    SMALLINT UNSIGNED NOT NULL,     -- response status
    payload        JSON DEFAULT NULL,              -- request body (JSON only, capped, secrets redacted)
    created_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_audit_actor (actor_user_id),
    KEY idx_audit_target (target_type, target_id),
    KEY idx_audit_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
