# Trimobe backend

Go REST API for the Trimobe multiservice platform — a modular monolith serving
both the customer and admin frontends.

## Requirements

- Go 1.25+
- MySQL 8.0+ with a database named `trimobase`

## Layout

```
cmd/api/            entry point (main.go)
internal/
  config/           env-based configuration + MySQL DSN
  database/         connection pool + embedded-migration runner
  httpx/            JSON request/response helpers
  server/           chi router, global middleware, health endpoints
  auth/             registration, login, JWT + rotating refresh tokens
migrations/         golang-migrate .up/.down SQL (embedded via embed.go)
```

Each business module (auth, and later catalog, orders, mobility, bookings,
payments) is a self-contained package following the same layering:
`model → repository → service → handler → routes`.

## Configuration

Copy `.env.example` to `.env` and adjust. `JWT_SECRET` is required; everything
else has a development default. The `.env` file is loaded automatically at
startup (and ignored if absent).

## Running

```bash
# create the database once
mysql -u root -p -e "CREATE DATABASE IF NOT EXISTS trimobase CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"

make tidy     # resolve dependencies
make run      # applies migrations, then starts the server on :8080
```

Migrations are embedded in the binary and applied on startup (over a dedicated
`multiStatements` connection). No separate migrate step is required.

### Creating the first admin

The public API never grants the `admin` role (self-registration is always
`customer`), so bootstrap the first admin with the seed command. It applies
migrations first, so it works on a fresh database:

```bash
make seed EMAIL=admin@trimo.dev PASSWORD=secret123 NAME="Super Admin"
# or directly:
go run ./cmd/seed -email=admin@trimo.dev -password=secret123 -name="Super Admin"
```

If the email already exists the user is promoted to `admin` (its password is
reset only when `-password` is supplied).

## Endpoints so far

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET  | `/healthz` | – | liveness |
| GET  | `/readyz`  | – | readiness (pings DB) |
| POST | `/api/v1/auth/register` | – | create a customer account, returns tokens |
| POST | `/api/v1/auth/login`    | – | exchange credentials for tokens |
| POST | `/api/v1/auth/refresh`  | – | rotate a refresh token |
| POST | `/api/v1/auth/logout`   | – | revoke a refresh token |
| GET  | `/api/v1/auth/me`       | Bearer | current user profile |

Access tokens are short-lived JWTs (Bearer); refresh tokens are opaque, stored
only as SHA-256 hashes, and rotated on every refresh.

### Catalog (phones & accessories)

Public reads:

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/categories` | active categories |
| GET | `/api/v1/brands` | active brands |
| GET | `/api/v1/products` | active products; filters `category_id`, `brand_id`, `q`; paging `page`, `limit` |
| GET | `/api/v1/products/{slug}` | product detail with category, brand, active variants, images |

Admin (all require a Bearer token with `role=admin`):

| Method | Path | Description |
|--------|------|-------------|
| GET/POST | `/api/v1/admin/categories` | list (incl. inactive) / create |
| PUT/DELETE | `/api/v1/admin/categories/{id}` | update / delete |
| GET/POST | `/api/v1/admin/brands` | list / create |
| PUT/DELETE | `/api/v1/admin/brands/{id}` | update / delete |
| GET/POST | `/api/v1/admin/products` | list (incl. inactive) / create |
| GET/PUT/DELETE | `/api/v1/admin/products/{id}` | detail / update / delete |
| POST | `/api/v1/admin/products/{id}/variants` | add a variant (SKU) |
| PUT/DELETE | `/api/v1/admin/variants/{id}` | update / delete a variant |
| POST | `/api/v1/admin/products/{id}/images` | add an image |
| DELETE | `/api/v1/admin/images/{id}` | delete an image |

Slugs are auto-generated from names (de-duplicated with `-2`, `-3`, …). Prices
are DECIMAL(12,2) carried as strings to preserve precision. Category-specific
and variant-specific extras go in the JSON `attributes` field.

### Mobility (cars for hire with driver)

Public reads:

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/car-categories` | active car categories |
| GET | `/api/v1/cars` | cars with `status=available`; filters `category_id`, `q`; paging `page`, `limit` |
| GET | `/api/v1/cars/{slug}` | car detail with category and images |

Admin (require `role=admin`):

| Method | Path | Description |
|--------|------|-------------|
| GET/POST | `/api/v1/admin/car-categories` | list / create |
| PUT/DELETE | `/api/v1/admin/car-categories/{id}` | update / delete |
| GET/POST | `/api/v1/admin/cars` | list (optional `?status=`) / create |
| GET/PUT/DELETE | `/api/v1/admin/cars/{id}` | detail / update / delete |
| GET | `/api/v1/admin/cars/{id}/overview` | car detail with usage revenue, booking counts, and booking feed |
| POST | `/api/v1/admin/cars/{id}/images` | add a car image |
| DELETE | `/api/v1/admin/car-images/{id}` | delete a car image |
| GET/POST | `/api/v1/admin/drivers` | list (optional `?status=`) / create |
| GET/PUT/DELETE | `/api/v1/admin/drivers/{id}` | detail / update / delete |

Pricing cascade: a new car's `daily_rate` is seeded from its category's
`default_daily_rate` when left blank, then becomes the car's own source of
truth. `registration_plate` is unique (blank → NULL). Car-category-specific
specs (payload, luggage, …) go in the car's JSON `attributes`.

Car and driver `status` values are effective for the current day on read:
cars stored as `available` return `not_available` when they have a confirmed,
driver-assigned, active, or completed booking overlapping today; available
drivers return `assigned` under the same today-booking rule.

Cargo-only car categories use route-distance pricing: routes up to 10km use
`cargo_minimum_rate`; longer routes use `cargo_minimum_rate +
cargo_per_km_rate * (distance_km - 10)`.

### Orders (phones e-commerce)

An order is either **delivered** to the customer or **picked up on site** (a
pickup reserves stock for 24h, then auto-expires). Fulfillment and payment are
tracked separately: the customer receives the goods first, then an admin
confirms payment. Stock is reserved at checkout inside a `SELECT … FOR UPDATE`
transaction and released on cancel/expiry.

Client (require a Bearer token):

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/cart` | current cart with live prices + subtotal |
| POST | `/api/v1/cart/items` | add `{product_variant_id, quantity}` (increments if present) |
| PATCH | `/api/v1/cart/items/{id}` | set line quantity |
| DELETE | `/api/v1/cart/items/{id}` | remove a line |
| DELETE | `/api/v1/cart` | empty the cart |
| POST | `/api/v1/orders` | checkout: `{fulfillment_type: delivery\|pickup, shipping_address?, note?}` |
| GET | `/api/v1/orders` | my orders (paged) |
| GET | `/api/v1/orders/{id}` | my order detail |
| POST | `/api/v1/orders/{id}/cancel` | cancel while `pending`/`confirmed` (restores stock) |

Admin (`role=admin`):

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/admin/orders` | list; filters `status`, `payment_status`, `fulfillment_type` |
| GET | `/api/v1/admin/orders/{id}` | order detail |
| PATCH | `/api/v1/admin/orders/{id}/status` | advance fulfillment: delivery `pending→confirmed→shipped→delivered`, pickup `pending→picked_up`, or `cancelled` |

Payment is confirmed via the **payments** module (`POST /api/v1/admin/payments`),
which records an audited ledger entry and flips the order's `payment_status`.

A background sweeper releases stock from pickup orders whose 24h hold has
lapsed (marks them `expired`). Money is handled as integer cents internally and
stored as DECIMAL(12,2).

### Bookings (cars for hire with driver)

A booking reserves a car over a date range. Availability is checked inside a
`SELECT … FOR UPDATE` transaction (the DB trigger from migration 000007 is the
backstop), so a car is never double-booked. `status` is the rental lifecycle;
`payment_status` is separate (manual, admin-confirmed). The daily rate is
snapshotted at booking time, and rental days are billed as inclusive calendar
days (start day = 1, return day also counts).

Public:

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/availability?car_id=&start=&end=` | is a car free for a date range? (RFC3339 times) |

Client (require a Bearer token):

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/bookings` | book `{car_id, start_at, end_at, pickup_location, dropoff_location?, distance_km?, contact_phone, note?}` |
| GET | `/api/v1/bookings` | my bookings (paged) |
| GET | `/api/v1/bookings/{id}` | my booking detail (with driver) |
| POST | `/api/v1/bookings/{id}/cancel` | cancel while `confirmed`/`driver_assigned` |

Admin (`role=admin`):

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/admin/bookings` | list; filters `status`, `payment_status`, `car_id` |
| GET | `/api/v1/admin/bookings/{id}` | booking detail |
| POST | `/api/v1/admin/bookings/{id}/assign-driver` | assign `{driver_id}` (checks driver is active + free) |
| PATCH | `/api/v1/admin/bookings/{id}/status` | `confirmed/driver_assigned→active→completed`, or `cancelled` |

A customer booking is created as `confirmed` (it holds the car immediately).
Occupying statuses (hold the car) = `confirmed`, `driver_assigned`, `active` —
these mirror the DB trigger exactly. Assigning a driver also refuses a driver
already committed to an overlapping booking. Payment is confirmed via the
**payments** module.

### Payments (manual ledger)

The single, audited way to confirm a manual/offline payment against an order or
a booking. Recording a payment writes a ledger row **and** flips the target's
`payment_status` — atomically. Admin only.

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/admin/payments` | record `{payable_type: order\|booking, payable_id, method, amount?, reference?, note?}` |
| GET | `/api/v1/admin/payments` | list; filters `payable_type`, `payable_id`, `status`, `method` |
| GET | `/api/v1/admin/payments/{id}` | payment detail |
| POST | `/api/v1/admin/payments/{id}/refund` | refund a paid payment (flips target to `refunded`) |

Gates: an **order** payment is allowed only once `delivered`/`picked_up` (and
not cancelled/expired); a **booking** payment any time before cancellation;
neither may be paid twice. `method` is `cash`, `bank_transfer`, `mobile_money`,
or `other`. `amount` defaults to the target's total. `marked_paid_by` is taken
from the authenticated admin. Adding a real gateway later is just another
`method`.

## Module path

The Go module is `github.com/trimo/backend` (placeholder). Rename in `go.mod`
and update imports when the real repository is set up.
