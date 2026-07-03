-- 000005_bookings.up.sql
-- Car booking = a car reserved over a time range, with a driver assigned.
-- Price is snapshotted so later rate changes never alter agreed bookings.
-- The no-overlap guarantee is enforced by triggers in migration 000007.

CREATE TABLE bookings (
    id                  BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    user_id             BIGINT UNSIGNED NOT NULL,
    car_id              BIGINT UNSIGNED NOT NULL,
    driver_id           BIGINT UNSIGNED DEFAULT NULL,   -- assigned by admin after confirmation
    booking_number      VARCHAR(32) NOT NULL,           -- app-generated, e.g. BKG-20260702-a1b2c3d4
    -- `status` is the rental lifecycle; payment is tracked separately (manual/offline,
    -- admin-confirmed) so it does not depend on where in the lifecycle the booking is.
    status              ENUM('requested','confirmed','driver_assigned','active','completed','cancelled')
                        NOT NULL DEFAULT 'requested',
    payment_status      ENUM('unpaid','paid','refunded') NOT NULL DEFAULT 'unpaid',
    paid_at             TIMESTAMP NULL DEFAULT NULL,
    start_at            DATETIME NOT NULL,
    end_at              DATETIME NOT NULL,
    days                INT NOT NULL,                   -- billed days
    daily_rate_snapshot DECIMAL(12,2) NOT NULL,         -- frozen from cars.daily_rate
    fees                DECIMAL(12,2) NOT NULL DEFAULT 0,
    total_price         DECIMAL(12,2) NOT NULL,         -- daily_rate_snapshot * days + fees
    -- snapshots so history reads cleanly even if the car/category changes
    car_name            VARCHAR(255) NOT NULL,
    car_category        VARCHAR(128) DEFAULT NULL,
    pickup_location     VARCHAR(512) NOT NULL,
    dropoff_location    VARCHAR(512) DEFAULT NULL,
    contact_phone       VARCHAR(32)  NOT NULL,
    note                TEXT,
    created_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_bookings_number (booking_number),
    KEY idx_bookings_user (user_id),
    KEY idx_bookings_car_time (car_id, start_at, end_at),  -- availability lookups
    KEY idx_bookings_driver (driver_id),
    KEY idx_bookings_status (status),
    KEY idx_bookings_payment (payment_status),
    CONSTRAINT fk_bookings_user   FOREIGN KEY (user_id)   REFERENCES users (id)   ON DELETE RESTRICT,
    CONSTRAINT fk_bookings_car    FOREIGN KEY (car_id)    REFERENCES cars (id)    ON DELETE RESTRICT,
    CONSTRAINT fk_bookings_driver FOREIGN KEY (driver_id) REFERENCES drivers (id) ON DELETE SET NULL,
    CONSTRAINT chk_bookings_dates CHECK (end_at > start_at),
    CONSTRAINT chk_bookings_amounts CHECK (days >= 1 AND daily_rate_snapshot >= 0 AND fees >= 0 AND total_price >= 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
