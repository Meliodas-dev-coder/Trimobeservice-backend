-- 000009_admin_manual_bookings.up.sql
-- Admins can create phone/walk-in bookings that are not tied to an app account.

ALTER TABLE bookings
    MODIFY COLUMN user_id BIGINT UNSIGNED NULL,
    ADD COLUMN customer_name VARCHAR(255) DEFAULT NULL AFTER user_id;
