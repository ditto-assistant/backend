CREATE TABLE IF NOT EXISTS tools (
  id TEXT PRIMARY KEY,
  name TEXT,
  description TEXT,
  version TEXT,
);

CREATE TABLE IF NOT EXISTS examples (
  id TEXT PRIMARY KEY,
  tool_id TEXT,
  prompt TEXT,
  response TEXT,
  -- 768-dimensional f32 vector embeddings
  em_prompt F32_BLOB(768), 
  em_response F32_BLOB(768), 
  em_prompt_response F32_BLOB(768),
);