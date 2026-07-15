-- 000021_restore_cargo_distance_pricing.down.sql

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

UPDATE car_categories
SET cargo_minimum_rate = 100000.00
WHERE is_cargo_transport = TRUE
  AND cargo_per_km_rate = 10000.00
  AND cargo_minimum_rate = 120000.00;
