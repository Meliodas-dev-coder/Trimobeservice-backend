# Database migrations

MySQL 8.0+ schema for the Trimo multiservice platform, written for
[golang-migrate](https://github.com/golang-migrate/migrate) (numbered
`.up.sql` / `.down.sql` pairs).

## Migration order

| # | Migration | Tables / objects |
|---|-----------|------------------|
| 000001 | `users_and_auth` | `users`, `addresses`, `refresh_tokens` |
| 000002 | `catalog_phones` | `brands`, `product_categories`, `products`, `product_variants`, `product_images` |
| 000003 | `orders` | `carts`, `cart_items`, `orders`, `order_items` |
| 000004 | `mobility_cars` | `car_categories`, `cars`, `car_images`, `drivers` |
| 000005 | `bookings` | `bookings` |
| 000006 | `payments` | `payments` (polymorphic: order or booking) |
| 000007 | `booking_overlap_guard` | triggers preventing overlapping car bookings |
| 000008 | `remove_car_category_image_url` | drops category-level car image column |
| 000009 | `admin_manual_bookings` | allows bookings without app-user accounts and stores customer name snapshots |
| 000010 | `cargo_transport_pricing` | cargo category rates and cargo booking distance snapshots |
| 000011 | `product_templates` | `product_categories.template_key`, `products.attributes` (JSON), `product_facets` |
| 000012 | `admin_manual_orders` | nullable `orders.user_id` + `orders.customer_name` (phone/walk-in orders) |
| 000013 | `event_planning` | `event_service_categories`, `event_services`, `event_requests`, `event_request_services` |
| 000014 | `event_artists` | `artists` and `event_request_artists` (bookable gospel artists) |
| 000015 | `brand_departments` | `brands.department` (tech/fashion split for the catalog) |
| 000016 | `healthcare` | `practitioners`, `healthcare_service_categories`, `healthcare_services`, `healthcare_package_staff`, `healthcare_requests`, `healthcare_request_assignments`, `healthcare_settings`; extends `payments.payable_type` with `healthcare` |
| 000017 | `content_translations` | localized content snapshots |
| 000018 | `coffee_catalog_seed` | coffee catalog seed data |
| 000019 | `audit_logs` | admin audit log |
| 000020 | `cargo_fuel_pricing` | cargo fuel-pricing fields |
| 000021 | `restore_cargo_distance_pricing` | restores distance-based cargo pricing |
| 000022 | `outside_antananarivo_pricing` | outside-region booking rate snapshots |
| 000023 | `category_outside_antananarivo_default` | category default outside-region rates |
| 000024 | `booking_groups` | one booking reference/payment total with multiple per-car booking items |
| 000025 | `invoicing` | `org_settings` (seller identity), `invoices`, `invoice_lines`, `invoice_sequences` (per-kind/year gapless numbering) |

They must apply in order — later migrations reference earlier tables via
foreign keys.

## Design notes

- **Four domains, shared users.** Phones (e-commerce), cars (time-based
  rental with driver), event planning (service requests), and healthcare (home
  consultations + care packages) are separate module trees that share `users`,
  auth, and `payments`. Each follows the same shape: **Category → Item →
  Transaction**.
- **Price snapshots.** `order_items` and `bookings` freeze the price agreed at
  purchase/booking time. Changing a product/car rate later never alters
  historical records.
- **Category default rates.** `car_categories.default_daily_rate` and
  `default_outside_antananarivo_daily_rate` are baselines that prefill new
  standard cars. The matching per-car rates are the source of truth, and
  `bookings.daily_rate_snapshot` freezes the selected rate per booking.
- **Cargo transport pricing.** Cargo-only car categories use
  `cargo_minimum_rate` for the first 10km, then add
  `cargo_per_km_rate * (distance_km - 10)` for longer routes.
- **Category-specific specs** live in the `JSON` `attributes` column on `cars`
  (e.g. `payload_kg` for cargo, `luggage_m3` for buses), on `products`
  (product-level specs), and on `product_variants` (per-SKU axes).
- **Product templates.** A `product_categories.template_key` picks a product
  "type" (phone, laptop, audio…, defined in Go, not the DB). That type drives
  which spec fields a product/variant carries in `attributes`. Filterable specs
  are denormalised into `product_facets` on every product/variant write for fast
  storefront faceting.
- **Departments (tech vs fashion).** Each product template belongs to a
  department (defined in Go). A category inherits its department from its
  template, and a product from its category — so categories/products are filtered
  by department without a stored column. Brands have no template link, so they
  carry their own `brands.department`. The admin and storefront use this to keep
  tech (phones/laptops/accessories…) and fashion (clothing/footwear) in separate
  sections.
- **No double-booking.** Enforced in two layers: the application must check
  availability inside the booking transaction with `SELECT ... FOR UPDATE`, and
  the triggers in 000007 are the DB-level safety net.
- **Multi-car bookings.** `booking_groups` gives 2–10 reserved cars one public
  booking reference and aggregate payable total. The underlying `bookings`
  rows remain per-car items for overlap protection and driver assignment.
- **Customer location pins.** Delivery, car pickup/dropoff, event, and home-care
  requests can store exact latitude/longitude coordinates plus a nearby meeting
  reference. Address text remains available for older records and as a fallback.
- **Manual payments.** `payments` records what an admin confirms (cash / bank
  transfer / mobile money) for orders, bookings, quoted event requests, and
  healthcare requests. Adding a real gateway later is just a new `method`.
- **Invoicing.** `invoices` sits above all four domains via
  `(invoiceable_type, invoiceable_id)` — the same polymorphism as `payments`.
  An invoice is a **frozen document**: seller identity (snapshotted from
  `org_settings`), buyer identity, `invoice_lines`, and totals are captured at
  issue time, so later edits never rewrite a sent document. Three `kind`s share
  the table — `proforma`, `final`, `credit_note` — each numbered from its own
  gapless per-year `invoice_sequences` counter. Tax is settings-driven
  (`org_settings.default_tax_rate`, 0 until TVA-registered). Amount paid /
  balance due are derived live from the `payments` ledger, never stored.
- **Healthcare (home care).** A doctor/nurse `practitioners` roster (like
  `drivers`) is assigned to `healthcare_requests`. A request is either a
  quote-priced **consultation** or a fixed-price **package** whose staff makeup
  lives in `healthcare_package_staff`; assignments are snapshotted in
  `healthcare_request_assignments`. Practitioner overlap is enforced softly in
  the app (a warning), not by a DB trigger. The emergency contact shown on the
  client page is a single editable row in `healthcare_settings`.

## Running

Create the database once (utf8mb4), then apply migrations:

```sql
CREATE DATABASE IF NOT EXISTS trimobase
  CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
```

```bash
# with the golang-migrate CLI (multiStatements is required for the triggers)
migrate -path ./migrations \
        -database "mysql://user:pass@tcp(127.0.0.1:3306)/trimobase?multiStatements=true" \
        up

# roll back the last migration
migrate -path ./migrations -database "mysql://.../trimobase?multiStatements=true" down 1
```

> The trigger file (000007) intentionally omits `DELIMITER` statements so it can
> be applied by the Go MySQL driver. If you run it through the `mysql` CLI
> instead, wrap each `CREATE TRIGGER` with `DELIMITER $$ ... $$`.
