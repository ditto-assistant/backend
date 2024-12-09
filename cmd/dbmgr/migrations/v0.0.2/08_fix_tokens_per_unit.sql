CREATE TABLE IF NOT EXISTS tokens_per_unit_new (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    count INTEGER NOT NULL,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO tokens_per_unit_new (name, count, updated_at)
SELECT name, count, updated_at 
FROM tokens_per_unit 
WHERE id IN (
    SELECT MIN(id) 
    FROM tokens_per_unit 
    GROUP BY name
);

DROP TRIGGER IF EXISTS after_insert_receipts;

DROP TRIGGER IF EXISTS update_tokens_per_unit_timestamp;

DROP TRIGGER IF EXISTS after_services_update_tokens_per_unit;

DROP TABLE tokens_per_unit;

ALTER TABLE tokens_per_unit_new RENAME TO tokens_per_unit;

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

DROP TRIGGER IF EXISTS update_tokens_per_unit_timestamp;

CREATE TRIGGER update_tokens_per_unit_timestamp
AFTER UPDATE ON tokens_per_unit
FOR EACH ROW
BEGIN
  UPDATE tokens_per_unit SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id;
END;

DROP TRIGGER IF EXISTS after_services_update_tokens_per_unit;

CREATE TRIGGER after_services_update_tokens_per_unit
AFTER UPDATE ON services
FOR EACH ROW
WHEN NEW.name IN ('dall-e-3', 'brave-search')
BEGIN
    -- Update image tokens only for DALL-E 3
    UPDATE tokens_per_unit 
    SET count = CASE
        WHEN NEW.name = 'dall-e-3' AND name = 'image' THEN
            ROUND((
                SELECT tpu.count * (COALESCE(NEW.base_cost_per_image, 0) + COALESCE(NEW.base_cost_per_call, 0)) * (1 + NEW.profit_margin_percentage / 100.0)
                FROM tokens_per_unit tpu
                WHERE tpu.name = 'dollar'
            ))
        ELSE count
    END,
    updated_at = CASE
        WHEN NEW.name = 'dall-e-3' AND name = 'image' THEN CURRENT_TIMESTAMP
        ELSE updated_at
    END
    WHERE name = 'image' AND NEW.name = 'dall-e-3';
    -- Update search tokens only for Brave Search
    UPDATE tokens_per_unit 
    SET count = CASE
        WHEN NEW.name = 'brave-search' AND name = 'search' THEN
            ROUND((
                SELECT tpu.count * (COALESCE(NEW.base_cost_per_search, 0) + COALESCE(NEW.base_cost_per_call, 0)) * (1 + NEW.profit_margin_percentage / 100.0)
                FROM tokens_per_unit tpu
                WHERE tpu.name = 'dollar'
            ))
        ELSE count
    END,
    updated_at = CASE
        WHEN NEW.name = 'brave-search' AND name = 'search' THEN CURRENT_TIMESTAMP
        ELSE updated_at
    END
    WHERE name = 'search' AND NEW.name = 'brave-search';
END; 