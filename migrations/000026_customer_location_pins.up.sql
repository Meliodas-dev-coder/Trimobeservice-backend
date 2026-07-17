ALTER TABLE orders
    ADD COLUMN ship_latitude DECIMAL(10,7) NULL AFTER ship_postal_code,
    ADD COLUMN ship_longitude DECIMAL(10,7) NULL AFTER ship_latitude,
    ADD COLUMN ship_location_reference VARCHAR(512) NULL AFTER ship_longitude;

ALTER TABLE bookings
    ADD COLUMN pickup_latitude DECIMAL(10,7) NULL AFTER pickup_location,
    ADD COLUMN pickup_longitude DECIMAL(10,7) NULL AFTER pickup_latitude,
    ADD COLUMN pickup_reference VARCHAR(512) NULL AFTER pickup_longitude,
    ADD COLUMN dropoff_latitude DECIMAL(10,7) NULL AFTER dropoff_location,
    ADD COLUMN dropoff_longitude DECIMAL(10,7) NULL AFTER dropoff_latitude,
    ADD COLUMN dropoff_reference VARCHAR(512) NULL AFTER dropoff_longitude;

ALTER TABLE event_requests
    ADD COLUMN location_latitude DECIMAL(10,7) NULL AFTER location,
    ADD COLUMN location_longitude DECIMAL(10,7) NULL AFTER location_latitude,
    ADD COLUMN location_reference VARCHAR(512) NULL AFTER location_longitude;

ALTER TABLE healthcare_requests
    ADD COLUMN location_latitude DECIMAL(10,7) NULL AFTER address,
    ADD COLUMN location_longitude DECIMAL(10,7) NULL AFTER location_latitude,
    ADD COLUMN location_reference VARCHAR(512) NULL AFTER location_longitude;
