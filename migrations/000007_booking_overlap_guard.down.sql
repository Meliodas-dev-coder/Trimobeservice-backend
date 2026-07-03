-- 000007_booking_overlap_guard.down.sql
DROP TRIGGER IF EXISTS trg_bookings_no_overlap_upd;
DROP TRIGGER IF EXISTS trg_bookings_no_overlap_ins;
