-- Demo data for the Fleetdock screenshots (PostgreSQL).
CREATE DATABASE shop;
CREATE DATABASE analytics;
CREATE DATABASE billing;

\connect shop
CREATE TABLE customers (
  id          bigserial PRIMARY KEY,
  email       text NOT NULL UNIQUE,
  full_name   text NOT NULL,
  country     char(2) NOT NULL,
  marketing   boolean NOT NULL DEFAULT false,
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE products (
  id          bigserial PRIMARY KEY,
  sku         text NOT NULL UNIQUE,
  name        text NOT NULL,
  category    text NOT NULL,
  price_cents integer NOT NULL CHECK (price_cents >= 0),
  stock       integer NOT NULL DEFAULT 0,
  active      boolean NOT NULL DEFAULT true
);
CREATE TABLE orders (
  id           bigserial PRIMARY KEY,
  customer_id  bigint NOT NULL REFERENCES customers(id),
  status       text NOT NULL DEFAULT 'pending',
  total_cents  integer NOT NULL,
  placed_at    timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE order_items (
  order_id    bigint NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
  product_id  bigint NOT NULL REFERENCES products(id),
  quantity    integer NOT NULL,
  price_cents integer NOT NULL,
  PRIMARY KEY (order_id, product_id)
);
CREATE INDEX orders_customer_idx ON orders (customer_id);
CREATE INDEX orders_placed_idx ON orders (placed_at);
CREATE VIEW revenue_by_day AS
  SELECT date_trunc('day', placed_at)::date AS day, count(*) AS orders, sum(total_cents) / 100.0 AS revenue
  FROM orders WHERE status <> 'cancelled' GROUP BY 1;

INSERT INTO customers (email, full_name, country, marketing, created_at)
SELECT lower(f || '.' || l || g) || '@' || (ARRAY['gmail.com','outlook.com','proton.me','fastmail.com','icloud.com'])[1 + g % 5],
       f || ' ' || l,
       (ARRAY['US','DE','GB','FR','NL','SE','CA','AU','JP','BR'])[1 + g % 10],
       g % 3 = 0,
       now() - (g || ' hours')::interval
FROM generate_series(1, 2400) g,
LATERAL (SELECT (ARRAY['Ada','Grace','Linus','Margaret','Ken','Barbara','Dennis','Frances','Alan','Radia','Guido','Sophie','Yukihiro','Anders','Hedy'])[1 + g % 15] AS f,
                (ARRAY['Lovelace','Hopper','Torvalds','Hamilton','Thompson','Liskov','Ritchie','Allen','Turing','Perlman','Rossum','Wilson','Matsumoto','Hejlsberg','Lamarr'])[1 + (g / 15) % 15] AS l) n;

INSERT INTO products (sku, name, category, price_cents, stock, active)
SELECT 'SKU-' || lpad(g::text, 5, '0'),
       (ARRAY['Mechanical keyboard','USB-C hub','27" monitor','Noise-cancelling headphones','Laptop stand','Webcam 4K','Desk lamp','Ergonomic mouse','Standing desk','Cable kit'])[1 + g % 10] || ' ' || (ARRAY['Pro','Lite','Max','Mini','Plus'])[1 + g % 5],
       (ARRAY['peripherals','displays','audio','furniture','accessories'])[1 + g % 5],
       1500 + (g * 937) % 48000,
       (g * 13) % 240,
       g % 17 <> 0
FROM generate_series(1, 180) g;

INSERT INTO orders (customer_id, status, total_cents, placed_at)
SELECT 1 + (g * 7) % 2400,
       (ARRAY['paid','paid','paid','shipped','shipped','delivered','delivered','pending','refunded','cancelled'])[1 + g % 10],
       2500 + (g * 331) % 90000,
       now() - ((g * 17) % 43200 || ' minutes')::interval
FROM generate_series(1, 9000) g;

INSERT INTO order_items (order_id, product_id, quantity, price_cents)
SELECT o.id, 1 + (o.id * k) % 180, 1 + (o.id + k) % 3, 1500 + (o.id * k * 97) % 48000
FROM orders o, generate_series(1, 2) k
ON CONFLICT DO NOTHING;

\connect analytics
CREATE TABLE events (
  id          bigserial PRIMARY KEY,
  occurred_at timestamptz NOT NULL,
  session_id  uuid NOT NULL,
  name        text NOT NULL,
  path        text NOT NULL,
  country     char(2),
  duration_ms integer
);
CREATE TABLE daily_active_users (
  day   date PRIMARY KEY,
  users integer NOT NULL
);
INSERT INTO events (occurred_at, session_id, name, path, country, duration_ms)
SELECT now() - ((g * 7) % 20160 || ' minutes')::interval,
       md5((g / 8)::text)::uuid,
       (ARRAY['page_view','page_view','page_view','click','signup','checkout_started','checkout_completed','search'])[1 + g % 8],
       (ARRAY['/','/pricing','/docs','/docs/getting-started','/blog','/features','/login','/signup'])[1 + (g / 3) % 8],
       (ARRAY['US','DE','GB','FR','NL','SE','CA','AU','JP','BR'])[1 + g % 10],
       100 + (g * 53) % 9000
FROM generate_series(1, 50000) g;
INSERT INTO daily_active_users
SELECT d::date, 1200 + (extract(doy FROM d)::int * 37) % 800 FROM generate_series(now() - interval '90 days', now(), interval '1 day') d;
CREATE INDEX events_occurred_idx ON events (occurred_at);

\connect billing
CREATE TABLE plans (
  id          text PRIMARY KEY,
  name        text NOT NULL,
  price_cents integer NOT NULL,
  interval    text NOT NULL DEFAULT 'month'
);
CREATE TABLE subscriptions (
  id          bigserial PRIMARY KEY,
  account     text NOT NULL,
  plan_id     text NOT NULL REFERENCES plans(id),
  status      text NOT NULL,
  started_at  timestamptz NOT NULL,
  renews_at   timestamptz
);
CREATE TABLE invoices (
  id              bigserial PRIMARY KEY,
  subscription_id bigint NOT NULL REFERENCES subscriptions(id),
  amount_cents    integer NOT NULL,
  status          text NOT NULL,
  issued_at       timestamptz NOT NULL
);
INSERT INTO plans VALUES ('starter','Starter',900,'month'),('team','Team',4900,'month'),('business','Business',19900,'month'),('enterprise','Enterprise',99000,'year');
INSERT INTO subscriptions (account, plan_id, status, started_at, renews_at)
SELECT 'acct_' || lpad(g::text, 5, '0'), (ARRAY['starter','team','team','business','enterprise'])[1 + g % 5],
       (ARRAY['active','active','active','past_due','canceled'])[1 + g % 5], now() - (g || ' days')::interval, now() + ((g % 30) || ' days')::interval
FROM generate_series(1, 600) g;
INSERT INTO invoices (subscription_id, amount_cents, status, issued_at)
SELECT 1 + g % 600, (ARRAY[900,4900,19900,99000])[1 + g % 4], (ARRAY['paid','paid','paid','open','void'])[1 + g % 5], now() - ((g % 365) || ' days')::interval
FROM generate_series(1, 4000) g;

\connect postgres
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;
