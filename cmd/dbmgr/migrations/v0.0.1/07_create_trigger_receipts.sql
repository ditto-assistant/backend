-- Create an AFTER INSERT trigger on the receipts table
CREATE TRIGGER after_insert_receipts
AFTER INSERT ON receipts
FOR EACH ROW
BEGIN
    -- Calculate the ditto_token_cost and update the newly inserted row
    UPDATE receipts
    SET ditto_token_cost = (
        SELECT MAX(1, ROUND(
            (COALESCE(base_cost_per_call, 0) * 1000000 +
             COALESCE(base_cost_per_million_tokens * (NEW.total_tokens / 1000000.0), 0) * 1000000 +
             COALESCE(base_cost_per_million_input_tokens * (NEW.input_tokens / 1000000.0), 0) * 1000000 +
             COALESCE(base_cost_per_million_output_tokens * (NEW.output_tokens / 1000000.0), 0) * 1000000 +
             COALESCE(base_cost_per_image * NEW.num_images, 0) * 1000000 +
             COALESCE(base_cost_per_search * NEW.num_searches, 0) * 1000000 +
             COALESCE(base_cost_per_second * NEW.call_duration_seconds, 0) * 1000000 +
             COALESCE(base_cost_per_gb_processed * (NEW.data_processed_bytes / 1073741824.0), 0) * 1000000 +
             COALESCE(base_cost_per_gb_stored * (NEW.data_stored_bytes / 1073741824.0), 0) * 1000000
            ) * (1 + profit_margin_percentage / 100.0)
        ))
        FROM services
        WHERE id = NEW.service_id
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
