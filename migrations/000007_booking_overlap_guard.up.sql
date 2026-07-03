-- 000007_booking_overlap_guard.up.sql
-- Database-level safety net against double-booking the same car. The app should
-- ALSO check with `SELECT ... FOR UPDATE` inside the booking transaction; these
-- triggers are the last line of defence.
--
-- Occupying statuses = confirmed, driver_assigned, active. A 'requested'
-- (not yet confirmed) or 'cancelled'/'completed' booking does not block a car.
-- (Payment is tracked in a separate payment_status column, so it is not here.)
-- Overlap test: NEW.start_at < existing.end_at AND NEW.end_at > existing.start_at
--
-- NOTE ON DELIMITERS: no DELIMITER statements are used here on purpose. Apply
-- these via golang-migrate / the Go MySQL driver (multiStatements=true), which
-- send each statement to the server directly. If you instead run this file
-- through the `mysql` CLI, wrap each trigger with DELIMITER $$ ... $$.

CREATE TRIGGER trg_bookings_no_overlap_ins
BEFORE INSERT ON bookings
FOR EACH ROW
BEGIN
    IF NEW.status IN ('confirmed','driver_assigned','active') THEN
        IF EXISTS (
            SELECT 1 FROM bookings b
            WHERE b.car_id = NEW.car_id
              AND b.status IN ('confirmed','driver_assigned','active')
              AND NEW.start_at < b.end_at
              AND NEW.end_at   > b.start_at
        ) THEN
            SIGNAL SQLSTATE '45000'
                SET MESSAGE_TEXT = 'Booking overlaps an existing reservation for this car';
        END IF;
    END IF;
END;

CREATE TRIGGER trg_bookings_no_overlap_upd
BEFORE UPDATE ON bookings
FOR EACH ROW
BEGIN
    IF NEW.status IN ('confirmed','driver_assigned','active') THEN
        IF EXISTS (
            SELECT 1 FROM bookings b
            WHERE b.car_id = NEW.car_id
              AND b.id    <> NEW.id
              AND b.status IN ('confirmed','driver_assigned','active')
              AND NEW.start_at < b.end_at
              AND NEW.end_at   > b.start_at
        ) THEN
            SIGNAL SQLSTATE '45000'
                SET MESSAGE_TEXT = 'Booking overlaps an existing reservation for this car';
        END IF;
    END IF;
END;
