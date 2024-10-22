-- Rollback for 07_create_trigger_receipts.sql
DROP TRIGGER IF EXISTS after_insert_receipts;

-- Rollback for 05_create_receipts_table.sql
DROP INDEX IF EXISTS idx_receipts_user_timestamp;
DROP TABLE IF EXISTS receipts;

-- Rollback for 03_create_examples_table.sql
DROP TABLE IF EXISTS examples;

-- Rollback for 02_create_tools_table.sql
DROP TABLE IF EXISTS tools;

-- Rollback for 04_create_users_table.sql
DROP TABLE IF EXISTS users;

-- Rollback for 06.5_create_ditto_price_table.sql
DROP TRIGGER IF EXISTS update_tokens_per_dollar_timestamp;
DROP TABLE IF EXISTS tokens_per_dollar;

-- Rollback for 06_insert_services.sql
DELETE FROM services;

-- Rollback for 01_create_service_table.sql
DROP TRIGGER IF EXISTS update_services_timestamp;
DROP TABLE IF EXISTS services;
