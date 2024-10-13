package llm

type ModelName string

const (
	ModelTextEmbedding004 ModelName = "text-embedding-004"
	ModelGemini15Flash    ModelName = "gemini-1.5-flash"
	ModelGemini15Pro      ModelName = "gemini-1.5-pro"
	ModelDalle3           ModelName = "dall-e-3"
)

func (m ModelName) String() string {
	return string(m)
}
