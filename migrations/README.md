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

They must apply in order — later migrations reference earlier tables via
foreign keys.

## Design notes

- **Two domains, shared users.** Phones (e-commerce) and cars (time-based
  rental with driver) are separate module trees that share `users`, auth, and
  `payments`. Each follows the same shape: **Category → Item → Transaction**.
- **Price snapshots.** `order_items` and `bookings` freeze the price agreed at
  purchase/booking time. Changing a product/car rate later never alters
  historical records.
- **Category default rate.** `car_categories.default_daily_rate` is a baseline
  that prefills new cars; `cars.daily_rate` is the real, per-car source of
  truth; `bookings.daily_rate_snapshot` freezes it per booking.
- **Category-specific specs** live in the `JSON` `attributes` column on `cars`
  (e.g. `payload_kg` for cargo, `luggage_m3` for buses) and `product_variants`.
- **No double-booking.** Enforced in two layers: the application must check
  availability inside the booking transaction with `SELECT ... FOR UPDATE`, and
  the triggers in 000007 are the DB-level safety net.
- **Manual payments.** `payments` records what an admin confirms (cash / bank
  transfer / mobile money). Adding a real gateway later is just a new `method`.

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
