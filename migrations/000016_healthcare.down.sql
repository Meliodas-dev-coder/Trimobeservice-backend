-- 000016_healthcare.down.sql
-- Reverse of 000016: drop healthcare tables (children first) and shrink the
-- payments enum back. Any 'healthcare' payment rows must be gone before rolling
-- back, or the ENUM change will fail.

ALTER TABLE payments
    MODIFY COLUMN payable_type ENUM('order','booking','event') NOT NULL;

DROP TABLE IF EXISTS healthcare_request_assignments;
DROP TABLE IF EXISTS healthcare_requests;
DROP TABLE IF EXISTS healthcare_package_staff;
DROP TABLE IF EXISTS healthcare_services;
DROP TABLE IF EXISTS healthcare_service_categories;
DROP TABLE IF EXISTS healthcare_settings;
DROP TABLE IF EXISTS practitioners;
