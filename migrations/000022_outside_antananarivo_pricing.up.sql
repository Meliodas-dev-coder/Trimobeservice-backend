-- 000022_outside_antananarivo_pricing.up.sql
-- Standard cars have a local daily rate and a separate daily rate for trips
-- outside the Antananarivo region. Cargo transport keeps distance pricing.

ALTER TABLE cars
    ADD COLUMN outside_antananarivo_daily_rate DECIMAL(12,2) NOT NULL DEFAULT 0 AFTER daily_rate;

UPDATE cars c
JOIN car_categories cc ON cc.id = c.category_id
SET c.outside_antananarivo_daily_rate = c.daily_rate
WHERE cc.is_cargo_transport = FALSE;

ALTER TABLE bookings
    ADD COLUMN outside_antananarivo BOOLEAN NOT NULL DEFAULT FALSE AFTER daily_rate_snapshot;
