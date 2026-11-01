-- Demo data for the Fleetdock screenshots (MySQL 8).
CREATE DATABASE support;
CREATE DATABASE payments;
CREATE USER 'fleetdock_admin'@'%' IDENTIFIED BY 'demo-mysql-password';
GRANT ALL PRIVILEGES ON *.* TO 'fleetdock_admin'@'%' WITH GRANT OPTION;
SET SESSION cte_max_recursion_depth = 100000;

USE support;
CREATE TABLE agents (id INT AUTO_INCREMENT PRIMARY KEY, name VARCHAR(120) NOT NULL, team VARCHAR(40) NOT NULL);
CREATE TABLE tickets (
  id INT AUTO_INCREMENT PRIMARY KEY,
  subject VARCHAR(255) NOT NULL,
  priority ENUM('low','normal','high','urgent') NOT NULL DEFAULT 'normal',
  status ENUM('open','pending','solved','closed') NOT NULL DEFAULT 'open',
  agent_id INT NULL,
  opened_at DATETIME NOT NULL,
  solved_at DATETIME NULL,
  CONSTRAINT fk_tickets_agent FOREIGN KEY (agent_id) REFERENCES agents(id)
);
INSERT INTO agents (name, team) VALUES ('Priya','Tier 1'),('Mateo','Tier 1'),('Hana','Tier 2'),('Oskar','Tier 2'),('Lena','Billing');
INSERT INTO tickets (subject, priority, status, agent_id, opened_at, solved_at)
WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < 5000)
SELECT CONCAT(ELT(1 + i % 6, 'Cannot log in', 'Backup failed overnight', 'Invoice question', 'Slow query on reports', 'Add a teammate', 'Export to CSV'), ' #', i),
       ELT(1 + i % 7, 'low','normal','normal','normal','high','high','urgent'),
       ELT(1 + i % 5, 'open','pending','solved','solved','closed'),
       IF(i % 9 = 0, NULL, 1 + i % 5),
       NOW() - INTERVAL i * 11 MINUTE,
       IF(i % 5 >= 2, NOW() - INTERVAL i * 7 MINUTE, NULL)
FROM n;

USE payments;
CREATE TABLE merchants (id INT AUTO_INCREMENT PRIMARY KEY, name VARCHAR(160) NOT NULL, country CHAR(2) NOT NULL);
CREATE TABLE charges (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  merchant_id INT NOT NULL,
  amount_cents INT NOT NULL,
  currency CHAR(3) NOT NULL,
  status ENUM('succeeded','failed','refunded') NOT NULL,
  created_at DATETIME NOT NULL,
  CONSTRAINT fk_charges_merchant FOREIGN KEY (merchant_id) REFERENCES merchants(id),
  KEY charges_created_idx (created_at)
);
INSERT INTO merchants (name, country) VALUES ('Blue Bottle','US'),('Kaffeehaus','DE'),('Le Comptoir','FR'),('Fika & Co','SE'),('Kissa Tokyo','JP');
INSERT INTO charges (merchant_id, amount_cents, currency, status, created_at)
WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < 20000)
SELECT 1 + i % 5, 300 + (i * 173) % 9000, ELT(1 + i % 5, 'USD','EUR','EUR','SEK','JPY'), ELT(1 + i % 12, 'succeeded','succeeded','succeeded','succeeded','succeeded','succeeded','succeeded','succeeded','succeeded','succeeded','failed','refunded'), NOW() - INTERVAL i * 3 MINUTE
FROM n;
