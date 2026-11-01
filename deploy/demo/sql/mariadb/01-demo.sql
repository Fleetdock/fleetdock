-- Demo data for the Fleetdock screenshots (MariaDB).
CREATE DATABASE cms;
CREATE DATABASE inventory;
CREATE DATABASE crm;
CREATE USER 'fleetdock_admin'@'%' IDENTIFIED BY 'demo-mariadb-password';
GRANT ALL PRIVILEGES ON *.* TO 'fleetdock_admin'@'%' WITH GRANT OPTION;

USE cms;
CREATE TABLE authors (
  id INT AUTO_INCREMENT PRIMARY KEY,
  name VARCHAR(120) NOT NULL,
  email VARCHAR(190) NOT NULL UNIQUE,
  bio TEXT
);
CREATE TABLE posts (
  id INT AUTO_INCREMENT PRIMARY KEY,
  author_id INT NOT NULL,
  slug VARCHAR(190) NOT NULL UNIQUE,
  title VARCHAR(255) NOT NULL,
  status ENUM('draft','published','archived') NOT NULL DEFAULT 'draft',
  views INT NOT NULL DEFAULT 0,
  published_at DATETIME NULL,
  CONSTRAINT fk_posts_author FOREIGN KEY (author_id) REFERENCES authors(id)
);
CREATE TABLE comments (
  id INT AUTO_INCREMENT PRIMARY KEY,
  post_id INT NOT NULL,
  author_name VARCHAR(120) NOT NULL,
  body TEXT NOT NULL,
  approved TINYINT(1) NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT fk_comments_post FOREIGN KEY (post_id) REFERENCES posts(id) ON DELETE CASCADE
);
INSERT INTO authors (name, email, bio) VALUES
 ('Ada Lovelace','ada@example.org','Writes about analytical engines.'),
 ('Grace Hopper','grace@example.org','Compilers, COBOL and nanoseconds.'),
 ('Linus Torvalds','linus@example.org','Kernels and version control.'),
 ('Margaret Hamilton','margaret@example.org','Software engineering, on the moon.'),
 ('Radia Perlman','radia@example.org','Spanning trees and networks.');
INSERT INTO posts (author_id, slug, title, status, views, published_at)
SELECT 1 + seq % 5, CONCAT('post-', seq),
       CONCAT(ELT(1 + seq % 6, 'How we cut backup time in half', 'Postgres vs MariaDB for small teams', 'Zero-downtime schema changes', 'Indexing JSON columns', 'Running 400 databases on one VPS', 'What we learned from a restore drill'), ' (part ', 1 + seq DIV 6, ')'),
       ELT(1 + seq % 4, 'published','published','draft','archived'), (seq * 137) % 25000,
       NOW() - INTERVAL seq DAY
FROM seq_1_to_240;
INSERT INTO comments (post_id, author_name, body, approved, created_at)
SELECT 1 + seq % 240, ELT(1 + seq % 5, 'sam', 'riley', 'jordan', 'alex', 'taylor'), 'Great write-up, thanks for sharing.', seq % 4 <> 0, NOW() - INTERVAL seq HOUR
FROM seq_1_to_3000;

USE inventory;
CREATE TABLE warehouses (id INT AUTO_INCREMENT PRIMARY KEY, code CHAR(3) NOT NULL UNIQUE, city VARCHAR(80) NOT NULL);
CREATE TABLE items (
  id INT AUTO_INCREMENT PRIMARY KEY,
  sku VARCHAR(32) NOT NULL UNIQUE,
  description VARCHAR(255) NOT NULL,
  unit_cost DECIMAL(10,2) NOT NULL
);
CREATE TABLE stock_levels (
  warehouse_id INT NOT NULL,
  item_id INT NOT NULL,
  quantity INT NOT NULL,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (warehouse_id, item_id),
  CONSTRAINT fk_stock_wh FOREIGN KEY (warehouse_id) REFERENCES warehouses(id),
  CONSTRAINT fk_stock_item FOREIGN KEY (item_id) REFERENCES items(id)
);
INSERT INTO warehouses (code, city) VALUES ('AMS','Amsterdam'),('BER','Berlin'),('NYC','New York'),('SFO','San Francisco'),('TYO','Tokyo');
INSERT INTO items (sku, description, unit_cost)
SELECT CONCAT('INV-', LPAD(seq, 5, '0')), CONCAT(ELT(1 + seq % 6, 'Steel bracket','Rack rail','Patch cable','SFP+ module','Power supply','Fan tray'), ' #', seq), ROUND(2 + (seq * 7.31) % 400, 2)
FROM seq_1_to_800;
INSERT INTO stock_levels (warehouse_id, item_id, quantity)
SELECT 1 + seq % 5, 1 + seq DIV 5, (seq * 31) % 900 FROM seq_0_to_3999;

USE crm;
CREATE TABLE companies (id INT AUTO_INCREMENT PRIMARY KEY, name VARCHAR(160) NOT NULL, industry VARCHAR(60), employees INT);
CREATE TABLE contacts (
  id INT AUTO_INCREMENT PRIMARY KEY,
  company_id INT NOT NULL,
  name VARCHAR(120) NOT NULL,
  email VARCHAR(190) NOT NULL,
  phone VARCHAR(40),
  CONSTRAINT fk_contacts_company FOREIGN KEY (company_id) REFERENCES companies(id)
);
CREATE TABLE deals (
  id INT AUTO_INCREMENT PRIMARY KEY,
  company_id INT NOT NULL,
  stage ENUM('lead','qualified','proposal','won','lost') NOT NULL,
  value_eur DECIMAL(12,2) NOT NULL,
  close_date DATE,
  CONSTRAINT fk_deals_company FOREIGN KEY (company_id) REFERENCES companies(id)
);
INSERT INTO companies (name, industry, employees)
SELECT CONCAT(ELT(1 + seq % 8, 'Northwind','Contoso','Globex','Initech','Umbrella','Hooli','Vandelay','Stark'), ' ', ELT(1 + seq DIV 8 % 5, 'Labs','Systems','Group','Retail','Logistics')), ELT(1 + seq % 5, 'SaaS','Retail','Logistics','Finance','Media'), 10 + (seq * 97) % 5000
FROM seq_1_to_400;
INSERT INTO contacts (company_id, name, email, phone)
SELECT 1 + seq % 400, CONCAT(ELT(1 + seq % 6, 'Sam','Riley','Jordan','Alex','Taylor','Morgan'), ' ', ELT(1 + seq DIV 6 % 6, 'Kim','Novak','Silva','Okafor','Larsen','Tanaka')), CONCAT('contact', seq, '@example.com'), CONCAT('+1 555 ', LPAD(seq, 4, '0'))
FROM seq_1_to_1600;
INSERT INTO deals (company_id, stage, value_eur, close_date)
SELECT 1 + seq % 400, ELT(1 + seq % 5, 'lead','qualified','proposal','won','lost'), ROUND(1000 + (seq * 811) % 250000, 2), CURDATE() + INTERVAL (CAST(seq AS SIGNED) % 120 - 60) DAY
FROM seq_1_to_1200;
