-- 000010_cargo_transport_pricing.up.sql
-- Cargo-only car categories price bookings by route distance instead of rental days.

ALTER TABLE car_categories
    ADD COLUMN is_cargo_transport BOOLEAN NOT NULL DEFAULT FALSE AFTER default_daily_rate,
    ADD COLUMN cargo_per_km_rate DECIMAL(12,2) NOT NULL DEFAULT 0 AFTER is_cargo_transport,
    ADD COLUMN cargo_minimum_rate DECIMAL(12,2) NOT NULL DEFAULT 0 AFTER cargo_per_km_rate,
    ADD CONSTRAINT chk_car_category_cargo_rates CHECK (cargo_per_km_rate >= 0 AND cargo_minimum_rate >= 0);

ALTER TABLE bookings
    ADD COLUMN pricing_model ENUM('daily','cargo_distance') NOT NULL DEFAULT 'daily' AFTER total_price,
    ADD COLUMN distance_km DECIMAL(10,2) DEFAULT NULL AFTER pricing_model,
    ADD COLUMN cargo_per_km_rate_snapshot DECIMAL(12,2) DEFAULT NULL AFTER distance_km,
    ADD COLUMN cargo_minimum_rate_snapshot DECIMAL(12,2) DEFAULT NULL AFTER cargo_per_km_rate_snapshot,
    ADD CONSTRAINT chk_bookings_distance CHECK (distance_km IS NULL OR distance_km > 0);
