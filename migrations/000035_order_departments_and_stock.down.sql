-- 000035_order_departments_and_stock.down.sql

DROP TABLE IF EXISTS stock_movements;

-- DROP CHECK (rather than DROP CONSTRAINT) matches the MySQL 8.0.16 baseline
-- the CHECK constraints in 000002 already assume.
ALTER TABLE product_variants
    DROP CHECK chk_variant_reorder_threshold,
    DROP COLUMN reorder_threshold;

ALTER TABLE order_items
    DROP KEY idx_order_items_order_department,
    DROP KEY idx_order_items_department,
    DROP COLUMN department;
