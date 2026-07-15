-- 000020_cargo_fuel_pricing.up.sql
-- Kept in migration history for databases that already applied this version.

ALTER TABLE cars
    ADD COLUMN fuel_consumption_per_100km DECIMAL(8,2) DEFAULT NULL AFTER fuel_type,
    ADD COLUMN fuel_price_per_liter DECIMAL(12,2) DEFAULT NULL AFTER fuel_consumption_per_100km,
    ADD CONSTRAINT chk_cars_fuel_consumption CHECK (
        fuel_consumption_per_100km IS NULL OR fuel_consumption_per_100km > 0
    ),
    ADD CONSTRAINT chk_cars_fuel_price CHECK (
        fuel_price_per_liter IS NULL OR fuel_price_per_liter > 0
    );

ALTER TABLE bookings
    ADD COLUMN fuel_consumption_per_100km_snapshot DECIMAL(8,2) DEFAULT NULL AFTER cargo_minimum_rate_snapshot,
    ADD COLUMN fuel_price_per_liter_snapshot DECIMAL(12,2) DEFAULT NULL AFTER fuel_consumption_per_100km_snapshot;
