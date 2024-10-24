DROP INDEX IF EXISTS idx_services_name;

DROP INDEX IF EXISTS idx_users_uid;

DROP TRIGGER IF EXISTS after_services_update_tokens_per_unit;

DROP TRIGGER IF EXISTS update_tokens_per_unit_timestamp;

DROP INDEX IF EXISTS idx_tokens_per_unit_name;

ALTER TABLE tokens_per_unit RENAME TO tokens_per_dollar;

DELETE FROM tokens_per_dollar;

INSERT INTO tokens_per_dollar (name, count) VALUES ('ditto', 1000000000);

CREATE INDEX IF NOT EXISTS idx_tokens_per_dollar_name ON tokens_per_dollar(name);

DROP TRIGGER IF EXISTS update_tokens_per_dollar_timestamp;

CREATE TRIGGER update_tokens_per_dollar_timestamp
AFTER UPDATE ON tokens_per_dollar
FOR EACH ROW
BEGIN
  UPDATE tokens_per_dollar SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id;
END;

DROP TRIGGER IF EXISTS after_insert_receipts;

CREATE TRIGGER after_insert_receipts
AFTER INSERT ON receipts
FOR EACH ROW
BEGIN
    UPDATE receipts
    SET ditto_token_cost = (
        SELECT MAX(1, ROUND(
            (COALESCE(base_cost_per_call, 0) * tpd.count +
             COALESCE(base_cost_per_million_tokens * (NEW.total_tokens / 1000000.0), 0) * tpd.count +
             COALESCE(base_cost_per_million_input_tokens * (NEW.input_tokens / 1000000.0), 0) * tpd.count +
             COALESCE(base_cost_per_million_output_tokens * (NEW.output_tokens / 1000000.0), 0) * tpd.count +
             COALESCE(base_cost_per_image * NEW.num_images, 0) * tpd.count +
             COALESCE(base_cost_per_search * NEW.num_searches, 0) * tpd.count +
             COALESCE(base_cost_per_second * NEW.call_duration_seconds, 0) * tpd.count +
             COALESCE(base_cost_per_gb_processed * (NEW.data_processed_bytes / 1073741824.0), 0) * tpd.count +
             COALESCE(base_cost_per_gb_stored * (NEW.data_stored_bytes / 1073741824.0), 0) * tpd.count
            ) * (1 + profit_margin_percentage / 100.0)
        ))
        FROM services, tokens_per_dollar AS tpd
        WHERE services.id = NEW.service_id AND tpd.name = 'ditto'
    )
    WHERE id = NEW.id;
    UPDATE users
    SET balance = balance - (
        SELECT ditto_token_cost
        FROM receipts
        WHERE id = NEW.id
    )
    WHERE id = NEW.user_id;
END;
