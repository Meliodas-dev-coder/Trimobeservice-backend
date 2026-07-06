-- 000013_event_planning.down.sql
-- Reverse of 000013: drop event tables (children first) and shrink the payments
-- enum back. Any 'event' payment rows must be gone before rolling back.

ALTER TABLE payments
    MODIFY COLUMN payable_type ENUM('order','booking') NOT NULL;

DROP TABLE IF EXISTS event_request_services;
DROP TABLE IF EXISTS event_requests;
DROP TABLE IF EXISTS event_services;
DROP TABLE IF EXISTS event_service_categories;
