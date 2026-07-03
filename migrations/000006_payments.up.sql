-- 000006_payments.up.sql
-- Manual/offline payments. A payment is a status an admin records against an
-- order or a booking (polymorphic via payable_type + payable_id). Kept as an
-- audit trail; adding a real gateway later is just another `method`.
-- NOTE: payable_id has no FK because it can point at two tables; integrity is
-- enforced in application code.

CREATE TABLE payments (
    id             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    payable_type   ENUM('order','booking') NOT NULL,
    payable_id     BIGINT UNSIGNED NOT NULL,
    amount         DECIMAL(12,2) NOT NULL,
    method         ENUM('cash','bank_transfer','mobile_money','other') NOT NULL DEFAULT 'cash',
    status         ENUM('pending','paid','refunded') NOT NULL DEFAULT 'pending',
    reference      VARCHAR(128) DEFAULT NULL,      -- transfer ref, receipt no, etc.
    marked_paid_by BIGINT UNSIGNED DEFAULT NULL,   -- admin user who confirmed payment
    marked_paid_at TIMESTAMP NULL DEFAULT NULL,
    note           TEXT,
    created_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_payments_payable (payable_type, payable_id),
    KEY idx_payments_status (status),
    KEY idx_payments_admin (marked_paid_by),
    CONSTRAINT fk_payments_admin FOREIGN KEY (marked_paid_by)
        REFERENCES users (id) ON DELETE SET NULL,
    CONSTRAINT chk_payments_amount CHECK (amount >= 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
