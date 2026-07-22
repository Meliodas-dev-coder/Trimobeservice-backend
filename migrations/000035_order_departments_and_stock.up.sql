-- 000035_order_departments_and_stock.up.sql
-- Two related additions that let each catalog department (tech / fashion /
-- coffee) run its own back office without splitting the customer's experience:
--
-- 1. `order_items.department` — a snapshot, in the same spirit as the existing
--    name/price snapshots. The customer still checks out ONCE, pays ONCE and
--    gets ONE invoice; the admin side slices that single order per department,
--    so a coffee manager sees the coffee lines of an order and never the tech
--    ones. Snapshotting (rather than joining live) keeps the slice stable when a
--    variant is later deleted or its category re-templated.
--
-- 2. A stock ledger (`stock_movements`) plus a per-SKU `reorder_threshold`.
--    `product_variants.stock_quantity` stays the single source of truth for what
--    is on hand; the ledger explains *how* it got there (checkout reservation,
--    cancellation release, manual restock, correction) and powers the low-stock
--    queues on the department overview.

-- --- 1. department snapshot on order lines ---------------------------------

ALTER TABLE order_items
    ADD COLUMN department VARCHAR(32) DEFAULT NULL AFTER product_variant_id,
    ADD KEY idx_order_items_department (department),
    ADD KEY idx_order_items_order_department (order_id, department);

-- Backfill from the live catalog: product -> category -> template -> department.
-- The Go registry (internal/catalog/templates.go) owns this mapping; the CASE
-- below is a one-time copy for existing rows only. Lines whose variant was
-- already deleted keep a NULL department (unknown, and unknowable).
UPDATE order_items oi
    JOIN product_variants pv ON pv.id = oi.product_variant_id
    JOIN products p ON p.id = pv.product_id
    JOIN product_categories pc ON pc.id = p.category_id
SET oi.department = CASE pc.template_key
        WHEN 'clothing' THEN 'fashion'
        WHEN 'footwear' THEN 'fashion'
        WHEN 'coffee'   THEN 'coffee'
        ELSE 'tech'
    END
WHERE oi.department IS NULL;

-- --- 2. stock ledger --------------------------------------------------------

ALTER TABLE product_variants
    ADD COLUMN reorder_threshold INT NOT NULL DEFAULT 0 AFTER stock_quantity,
    ADD CONSTRAINT chk_variant_reorder_threshold CHECK (reorder_threshold >= 0);

-- Every change to stock_quantity gets a row here. `delta` is signed,
-- `quantity_after` is the resulting on-hand level so the ledger can be read
-- without replaying it from zero. Reservation reasons are written by the orders
-- module inside the same transaction that moves the stock.
--
-- Deleting a variant cascades its movements away: the ledger is an operational
-- stock record, not the financial audit trail (that lives in order_items,
-- payments, and invoices, all of which snapshot independently).
CREATE TABLE stock_movements (
    id                 BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    product_variant_id BIGINT UNSIGNED NOT NULL,
    department         VARCHAR(32) DEFAULT NULL,  -- snapshot, so scoped reads stay cheap
    reason             ENUM('initial','restock','manual_adjust','correction',
                            'order_reserve','order_release') NOT NULL,
    delta              INT NOT NULL,              -- signed; never 0
    quantity_after     INT NOT NULL,
    reference_type     ENUM('order') DEFAULT NULL,
    reference_id       BIGINT UNSIGNED DEFAULT NULL,
    note               VARCHAR(500) DEFAULT NULL,
    created_by         BIGINT UNSIGNED DEFAULT NULL,  -- admin who adjusted; NULL for system moves
    created_at         TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_stock_movements_variant (product_variant_id, id),
    KEY idx_stock_movements_department (department, created_at),
    KEY idx_stock_movements_reference (reference_type, reference_id),
    CONSTRAINT fk_stock_movements_variant FOREIGN KEY (product_variant_id)
        REFERENCES product_variants (id) ON DELETE CASCADE,
    CONSTRAINT fk_stock_movements_user FOREIGN KEY (created_by)
        REFERENCES users (id) ON DELETE SET NULL,
    CONSTRAINT chk_stock_movement_delta CHECK (delta <> 0),
    CONSTRAINT chk_stock_movement_after CHECK (quantity_after >= 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Open the ledger with the stock that already exists, so today's on-hand level
-- reconciles against the sum of movements from the very first read.
INSERT INTO stock_movements (product_variant_id, department, reason, delta, quantity_after, note)
SELECT pv.id,
       CASE pc.template_key
           WHEN 'clothing' THEN 'fashion'
           WHEN 'footwear' THEN 'fashion'
           WHEN 'coffee'   THEN 'coffee'
           ELSE 'tech'
       END,
       'initial',
       pv.stock_quantity,
       pv.stock_quantity,
       'Opening balance recorded when the stock ledger was introduced'
FROM product_variants pv
    JOIN products p ON p.id = pv.product_id
    JOIN product_categories pc ON pc.id = p.category_id
WHERE pv.stock_quantity > 0;
