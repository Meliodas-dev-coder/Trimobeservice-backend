-- 000008_remove_car_category_image_url.down.sql

ALTER TABLE car_categories
    ADD COLUMN image_url VARCHAR(512) DEFAULT NULL AFTER default_daily_rate;
