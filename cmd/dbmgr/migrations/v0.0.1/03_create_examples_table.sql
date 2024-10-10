CREATE TABLE IF NOT EXISTS examples (
  id INTEGER PRIMARY KEY,
  tool_id INTEGER,
  prompt TEXT,
  response TEXT,
  em_prompt F32_BLOB(768), 
  em_prompt_response F32_BLOB(768),
  FOREIGN KEY (tool_id) REFERENCES tools(id)
);