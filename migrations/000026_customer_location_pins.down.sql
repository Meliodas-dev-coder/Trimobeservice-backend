ALTER TABLE healthcare_requests
    DROP COLUMN location_reference,
    DROP COLUMN location_longitude,
    DROP COLUMN location_latitude;

ALTER TABLE event_requests
    DROP COLUMN location_reference,
    DROP COLUMN location_longitude,
    DROP COLUMN location_latitude;

ALTER TABLE bookings
    DROP COLUMN dropoff_reference,
    DROP COLUMN dropoff_longitude,
    DROP COLUMN dropoff_latitude,
    DROP COLUMN pickup_reference,
    DROP COLUMN pickup_longitude,
    DROP COLUMN pickup_latitude;

ALTER TABLE orders
    DROP COLUMN ship_location_reference,
    DROP COLUMN ship_longitude,
    DROP COLUMN ship_latitude;
