ALTER TABLE car_categories
    ADD COLUMN default_outside_antananarivo_daily_rate DECIMAL(12,2) NOT NULL DEFAULT 0 AFTER default_daily_rate;

UPDATE car_categories
SET default_outside_antananarivo_daily_rate = default_daily_rate
WHERE is_cargo_transport = FALSE;
