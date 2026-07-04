-- 000010_cargo_transport_pricing.down.sql

ALTER TABLE bookings
    DROP CHECK chk_bookings_distance,
    DROP COLUMN cargo_minimum_rate_snapshot,
    DROP COLUMN cargo_per_km_rate_snapshot,
    DROP COLUMN distance_km,
    DROP COLUMN pricing_model;

ALTER TABLE car_categories
    DROP CHECK chk_car_category_cargo_rates,
    DROP COLUMN cargo_minimum_rate,
    DROP COLUMN cargo_per_km_rate,
    DROP COLUMN is_cargo_transport;
