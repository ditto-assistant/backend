CREATE TABLE IF NOT EXISTS tools (
  name TEXT,
  description TEXT,
  version TEXT,
  cost_per_call REAL NOT NULL,
  -- for tools needing LLM, this multiplier multiplies the base token cost (which factors in the length of the prompt template)
  -- base cost = prompt template tokens * multiplier
  cost_multiplier REAL NOT NULL,
  -- the amount of tokens in the prompt template
  -- which will be multiplied by the cost per token.
  -- this cost varies depending on the LLM used.
  base_tokens INTEGER NOT NULL
);
