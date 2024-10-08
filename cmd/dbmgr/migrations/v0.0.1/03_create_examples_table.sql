CREATE TABLE IF NOT EXISTS examples (
  tool_id INTEGER,
  prompt TEXT,
  response TEXT,
  -- 768-dimensional f32 vector embeddings
  em_prompt F32_BLOB(768), 
  em_prompt_response F32_BLOB(768),
  FOREIGN KEY (tool_id) REFERENCES tools(rowid)
);