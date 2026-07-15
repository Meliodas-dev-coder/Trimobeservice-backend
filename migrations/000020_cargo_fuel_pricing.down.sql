-- 000020_cargo_fuel_pricing.down.sql

ALTER TABLE bookings
    DROP COLUMN fuel_price_per_liter_snapshot,
    DROP COLUMN fuel_consumption_per_100km_snapshot;

ALTER TABLE cars
    DROP CHECK chk_cars_fuel_price,
    DROP CHECK chk_cars_fuel_consumption,
    DROP COLUMN fuel_price_per_liter,
    DROP COLUMN fuel_consumption_per_100km;
