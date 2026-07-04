-- 000012_admin_manual_orders.up.sql
-- Admins can create phone/walk-in orders that are not tied to an app account,
-- mirroring manual bookings (000009).

ALTER TABLE orders
    MODIFY COLUMN user_id BIGINT UNSIGNED NULL,
    ADD COLUMN customer_name VARCHAR(255) DEFAULT NULL AFTER user_id;
