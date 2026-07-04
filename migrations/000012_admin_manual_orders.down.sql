-- 000012_admin_manual_orders.down.sql

ALTER TABLE orders
    DROP COLUMN customer_name,
    MODIFY COLUMN user_id BIGINT UNSIGNED NOT NULL;
