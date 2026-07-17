ALTER TABLE bookings
    DROP FOREIGN KEY fk_bookings_group,
    DROP KEY idx_bookings_group,
    DROP COLUMN booking_group_id;

DROP TABLE booking_groups;
