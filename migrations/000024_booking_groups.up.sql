CREATE TABLE booking_groups (
    booking_id BIGINT UNSIGNED NOT NULL,
    booking_number VARCHAR(32) NOT NULL,
    total_price DECIMAL(12, 2) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (booking_id),
    UNIQUE KEY uq_booking_groups_number (booking_number),
    CONSTRAINT fk_booking_groups_primary_booking
        FOREIGN KEY (booking_id) REFERENCES bookings(id) ON DELETE RESTRICT,
    CONSTRAINT chk_booking_groups_total CHECK (total_price >= 0)
);

ALTER TABLE bookings
    ADD COLUMN booking_group_id BIGINT UNSIGNED NULL AFTER id,
    ADD KEY idx_bookings_group (booking_group_id),
    ADD CONSTRAINT fk_bookings_group
        FOREIGN KEY (booking_group_id) REFERENCES booking_groups(booking_id) ON DELETE RESTRICT;
