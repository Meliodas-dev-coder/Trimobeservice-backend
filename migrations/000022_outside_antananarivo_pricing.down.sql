-- 000022_outside_antananarivo_pricing.down.sql

ALTER TABLE bookings
    DROP COLUMN outside_antananarivo;

ALTER TABLE cars
    DROP COLUMN outside_antananarivo_daily_rate;
