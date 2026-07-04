-- 000009_admin_manual_bookings.down.sql
-- Rollback requires all bookings to be linked to a user again.

ALTER TABLE bookings
    DROP COLUMN customer_name,
    MODIFY COLUMN user_id BIGINT UNSIGNED NOT NULL;
