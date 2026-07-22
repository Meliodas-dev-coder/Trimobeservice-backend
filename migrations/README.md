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
| 000026 | `customer_location_pins` | lat/lng coordinates + meeting reference on delivery/pickup/event/home-care records |
| 000027 | `event_line_pricing` | per-line pricing snapshots on event requests |
| 000028 | `admin_roles_and_permissions` | `admin_roles` (screen-permission bundles) + `users.is_super_admin` / `users.admin_role_id`; backfills existing admins as super-admins |
| 000029 | `hr_management` | native HR organization, employees, lifecycle, leave, attendance, performance, recruitment, expenses, compensation, private documents, notifications, workflows, and role presets |
| 000030 | `employee_access_and_hr_scopes` | employee back-office identity reconciliation, department hierarchy/module scope, position submenu/action permissions, scoped HR policies, temporary access assignments, and sensitive-read audit metadata |
| 000031 | `position_hierarchy` | `hr_positions.parent_position_id` (self-ref, same-department parent) + `hierarchy_rank`: the base per-department position ladder that seeds onboarding managers and drives the `position_hierarchy` leave-approval step |
| 000032 | `hr_shift_type` | `hr_shifts.shift_type` (day \| night \| special); only `special` shifts keep a user-supplied name |
| 000033 | `hr_contract_templates` | `hr_contract_templates` (employment \| memo_deal, French body with `{{namespace.key}}` placeholders) + seeded standard bodies |
| 000034 | `hr_contract_documents` | `hr_contract_documents` (issued documents with frozen `body_rendered`) + `hr_contract_sequences` for gapless per-(kind,year) references |
| 000035 | `order_departments_and_stock` | `order_items.department` (snapshot, backfilled) so each catalog department reads its own slice of the shared order book; `product_variants.reorder_threshold` + the `stock_movements` ledger (opened with each SKU's current balance) |

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
- **Admin roles & screen permissions.** Beyond the coarse `users.role`
  (`customer`/`admin`), `admin_roles` bundle a set of business-section keys (JSON,
  validated app-side against `internal/authz`) that an admin employee is assigned
  via `users.admin_role_id`. `users.is_super_admin` (the seeded owner) bypasses
  every check. Every admin API is gated by `RequirePermission` for its section, so
  a restricted employee is rejected server-side, not just hidden in the UI.
- **Healthcare (home care).** A doctor/nurse `practitioners` roster (like
  `drivers`) is assigned to `healthcare_requests`. A request is either a
  quote-priced **consultation** or a fixed-price **package** whose staff makeup
  lives in `healthcare_package_staff`; assignments are snapshotted in
  `healthcare_request_assignments`. Practitioner overlap is enforced softly in
  the app (a warning), not by a DB trigger. The emergency contact shown on the
  client page is a single editable row in `healthcare_settings`.

- **Department slicing of orders (000035).** The customer checks out once, pays
  once, and gets one invoice — that is unchanged. What 000035 adds is a
  `department` snapshot on each order line so the back office can show a
  department only its own lines and its own share of an order. The value is
  frozen at checkout (like the name and price beside it) so the split survives a
  variant being deleted or a category re-templated. Lines whose variant was
  already gone at migration time keep a NULL department: unattributable, and
  therefore visible only to the cross-department Orders screen.

- **Stock ledger (000035).** `product_variants.stock_quantity` stays the single
  source of truth for what is on hand — reservations are still subtracted from
  it at checkout and added back on cancellation. `stock_movements` records every
  one of those changes with a signed `delta` and the resulting
  `quantity_after`, so a level can always be explained without replaying it from
  zero. Reservation/release rows are written by the orders module inside the
  checkout transaction; restocks and corrections come from the stock screen.
  `reorder_threshold` (0 = no threshold) is what turns a low shelf into a
  warning rather than a surprise.

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
