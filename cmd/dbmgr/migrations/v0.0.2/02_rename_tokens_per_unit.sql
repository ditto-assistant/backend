DROP INDEX IF EXISTS idx_tokens_per_dollar_name;

ALTER TABLE tokens_per_dollar RENAME TO tokens_per_unit;

DELETE FROM tokens_per_unit WHERE name = 'ditto';

INSERT INTO tokens_per_unit (name, count) VALUES ('dollar', 1000000000);

CREATE INDEX IF NOT EXISTS idx_tokens_per_unit_name ON tokens_per_unit(name);

DROP TRIGGER IF EXISTS update_tokens_per_dollar_timestamp;

CREATE TRIGGER IF NOT EXISTS update_tokens_per_unit_timestamp
AFTER UPDATE ON tokens_per_unit
FOR EACH ROW
BEGIN
  UPDATE tokens_per_unit SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id;
END;

DROP TRIGGER IF EXISTS after_insert_receipts;

CREATE TRIGGER after_insert_receipts
AFTER INSERT ON receipts
FOR EACH ROW
BEGIN
    -- Calculate the ditto_token_cost and update the newly inserted row
    UPDATE receipts
    SET ditto_token_cost = (
        SELECT MAX(1, ROUND(
            (COALESCE(base_cost_per_call, 0) * tpu.count +
             COALESCE(base_cost_per_million_tokens * (NEW.total_tokens / 1000000.0), 0) * tpu.count +
             COALESCE(base_cost_per_million_input_tokens * (NEW.input_tokens / 1000000.0), 0) * tpu.count +
             COALESCE(base_cost_per_million_output_tokens * (NEW.output_tokens / 1000000.0), 0) * tpu.count +
             COALESCE(base_cost_per_image * NEW.num_images, 0) * tpu.count +
             COALESCE(base_cost_per_search * NEW.num_searches, 0) * tpu.count +
             COALESCE(base_cost_per_second * NEW.call_duration_seconds, 0) * tpu.count +
             COALESCE(base_cost_per_gb_processed * (NEW.data_processed_bytes / 1073741824.0), 0) * tpu.count +
             COALESCE(base_cost_per_gb_stored * (NEW.data_stored_bytes / 1073741824.0), 0) * tpu.count
            ) * (1 + profit_margin_percentage / 100.0)
        ))
        FROM services, tokens_per_unit AS tpu
        WHERE services.id = NEW.service_id AND tpu.name = 'dollar'
    )
    WHERE id = NEW.id;
    -- Update the user's balance
    UPDATE users
    SET balance = balance - (
        SELECT ditto_token_cost
        FROM receipts
        WHERE id = NEW.id
    )
    WHERE id = NEW.user_id;
END;

INSERT INTO tokens_per_unit (name, count, updated_at) 
VALUES ('image',(
        SELECT ROUND(tpu.count * (svc.base_cost_per_image + svc.base_cost_per_call) * (1 + svc.profit_margin_percentage / 100.0))
        FROM tokens_per_unit tpu, services svc
        WHERE tpu.name = 'dollar' AND svc.name = 'dall-e-3'
    ), CURRENT_TIMESTAMP);

INSERT INTO tokens_per_unit (name, count, updated_at) 
VALUES ('search',(
        SELECT ROUND(tpu.count * (svc.base_cost_per_search + svc.base_cost_per_call) * (1 + svc.profit_margin_percentage / 100.0))
        FROM tokens_per_unit tpu, services svc
        WHERE tpu.name = 'dollar' AND svc.name = 'brave-search'
    ), CURRENT_TIMESTAMP);

CREATE TRIGGER IF NOT EXISTS after_services_update_tokens_per_unit
AFTER UPDATE ON services
FOR EACH ROW
BEGIN
    -- Update or insert 'images' row (DALL-E 3 pricing)
    INSERT INTO tokens_per_unit (name, count, updated_at)
    VALUES ('image', (
        SELECT ROUND(tpu.count * (NEW.base_cost_per_image + NEW.base_cost_per_call) * (1 + NEW.profit_margin_percentage / 100.0))
        FROM tokens_per_unit tpu
        WHERE tpu.name = 'dollar'
    ), CURRENT_TIMESTAMP)
    ON CONFLICT(name) DO UPDATE SET
        count = ROUND((
            SELECT tpu.count * (NEW.base_cost_per_image + NEW.base_cost_per_call) * (1 + NEW.profit_margin_percentage / 100.0)
            FROM tokens_per_unit tpu
            WHERE tpu.name = 'dollar'
        )),
        updated_at = CURRENT_TIMESTAMP
    WHERE name = 'image';
    -- Update or insert 'searches' row (Brave Search pricing)
    INSERT INTO tokens_per_unit (name, count, updated_at)
    VALUES ('search', (
        SELECT ROUND(tpu.count * (NEW.base_cost_per_search + NEW.base_cost_per_call) * (1 + NEW.profit_margin_percentage / 100.0))
        FROM tokens_per_unit tpu
        WHERE tpu.name = 'dollar'
    ), CURRENT_TIMESTAMP)
    ON CONFLICT(name) DO UPDATE SET
        count = ROUND((
            SELECT tpu.count * (NEW.base_cost_per_search + NEW.base_cost_per_call) * (1 + NEW.profit_margin_percentage / 100.0)
            FROM tokens_per_unit tpu
            WHERE tpu.name = 'dollar'
        )),
        updated_at = CURRENT_TIMESTAMP
    WHERE name = 'search';
END;
