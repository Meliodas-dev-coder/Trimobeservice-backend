-- 000003_orders.up.sql
-- E-commerce transaction side: cart (authenticated users only; guests keep a
-- client-side cart that merges on login) -> order with frozen line-item prices.

CREATE TABLE carts (
    id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    user_id    BIGINT UNSIGNED NOT NULL,
    status     ENUM('active','converted','abandoned') NOT NULL DEFAULT 'active',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_carts_user_status (user_id, status),
    CONSTRAINT fk_carts_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE cart_items (
    id                 BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    cart_id            BIGINT UNSIGNED NOT NULL,
    product_variant_id BIGINT UNSIGNED NOT NULL,
    quantity           INT NOT NULL DEFAULT 1,
    created_at         TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at         TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_cart_item (cart_id, product_variant_id),  -- one row per variant per cart
    KEY idx_cart_items_variant (product_variant_id),
    CONSTRAINT fk_cart_items_cart FOREIGN KEY (cart_id)
        REFERENCES carts (id) ON DELETE CASCADE,
    CONSTRAINT fk_cart_items_variant FOREIGN KEY (product_variant_id)
        REFERENCES product_variants (id) ON DELETE CASCADE,
    CONSTRAINT chk_cart_item_qty CHECK (quantity > 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- An order is either delivered to the customer or picked up on site. Payment is
-- confirmed by an admin AFTER hand-over (deliver/pickup first, then pay), so
-- fulfillment (`status`) and payment (`payment_status`) are tracked separately.
-- A pickup order reserves stock only until `reserved_until` (placement + 24h).
CREATE TABLE orders (
    id             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    user_id        BIGINT UNSIGNED NOT NULL,
    order_number   VARCHAR(32) NOT NULL,       -- human ref, e.g. ORD-20260702-a1b2c3d4 (app-generated)
    fulfillment_type ENUM('delivery','pickup') NOT NULL DEFAULT 'delivery',
    status         ENUM('pending','confirmed','shipped','delivered','picked_up','cancelled','expired')
                   NOT NULL DEFAULT 'pending',
    payment_status ENUM('unpaid','paid','refunded') NOT NULL DEFAULT 'unpaid',
    subtotal       DECIMAL(12,2) NOT NULL DEFAULT 0,
    shipping_fee   DECIMAL(12,2) NOT NULL DEFAULT 0,
    total          DECIMAL(12,2) NOT NULL DEFAULT 0,
    -- pickup hold: stock is reserved until this time; NULL for delivery orders
    reserved_until DATETIME NULL DEFAULT NULL,
    -- shipping address snapshot (frozen at checkout); all NULL for pickup orders
    ship_recipient_name VARCHAR(255) DEFAULT NULL,
    ship_phone          VARCHAR(32)  DEFAULT NULL,
    ship_line1          VARCHAR(255) DEFAULT NULL,
    ship_line2          VARCHAR(255) DEFAULT NULL,
    ship_city           VARCHAR(128) DEFAULT NULL,
    ship_region         VARCHAR(128) DEFAULT NULL,
    ship_country        VARCHAR(128) DEFAULT NULL,
    ship_postal_code    VARCHAR(32)  DEFAULT NULL,
    note           TEXT,
    placed_at      TIMESTAMP NULL DEFAULT NULL,
    paid_at        TIMESTAMP NULL DEFAULT NULL,
    created_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_orders_number (order_number),
    KEY idx_orders_user (user_id),
    KEY idx_orders_status (status),
    KEY idx_orders_payment (payment_status),
    KEY idx_orders_reserved (reserved_until),   -- pickup-expiry sweeps
    CONSTRAINT fk_orders_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE RESTRICT,
    CONSTRAINT chk_order_totals CHECK (subtotal >= 0 AND shipping_fee >= 0 AND total >= 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Line items snapshot name/price so history survives product edits or deletes.
CREATE TABLE order_items (
    id                 BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    order_id           BIGINT UNSIGNED NOT NULL,
    product_variant_id BIGINT UNSIGNED DEFAULT NULL,   -- SET NULL if variant later deleted
    product_name       VARCHAR(255) NOT NULL,           -- snapshot
    variant_label      VARCHAR(255) DEFAULT NULL,       -- snapshot ("Black / 128GB")
    sku                VARCHAR(64)  DEFAULT NULL,        -- snapshot
    unit_price         DECIMAL(12,2) NOT NULL,          -- snapshot (price at purchase)
    quantity           INT NOT NULL,
    line_total         DECIMAL(12,2) NOT NULL,
    created_at         TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_order_items_order (order_id),
    KEY idx_order_items_variant (product_variant_id),
    CONSTRAINT fk_order_items_order FOREIGN KEY (order_id)
        REFERENCES orders (id) ON DELETE CASCADE,
    CONSTRAINT fk_order_items_variant FOREIGN KEY (product_variant_id)
        REFERENCES product_variants (id) ON DELETE SET NULL,
    CONSTRAINT chk_order_item CHECK (quantity > 0 AND unit_price >= 0 AND line_total >= 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
