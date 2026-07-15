-- 000021_restore_cargo_distance_pricing.up.sql
-- Remove fuel pricing and restore the fixed cargo distance rule.

ALTER TABLE bookings
    DROP COLUMN fuel_price_per_liter_snapshot,
    DROP COLUMN fuel_consumption_per_100km_snapshot;

ALTER TABLE cars
    DROP CHECK chk_cars_fuel_price,
    DROP CHECK chk_cars_fuel_consumption,
    DROP COLUMN fuel_price_per_liter,
    DROP COLUMN fuel_consumption_per_100km;

UPDATE car_categories
SET default_daily_rate = 0,
    cargo_per_km_rate = 10000.00,
    cargo_minimum_rate = 120000.00
WHERE is_cargo_transport = TRUE;
