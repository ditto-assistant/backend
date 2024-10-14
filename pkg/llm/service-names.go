package llm

type ServiceName string

// Text Embeddings
const (
	// ModelTextEmbedding004 is Google's embedding model.
	ModelTextEmbedding004 ServiceName = "text-embedding-004"
	// ModelTextEmbedding3Small is OpenAI's embedding model.
	ModelTextEmbedding3Small ServiceName = "text-embedding-3-small"
)

// Text Models
const (
	// ModelGemini15Flash is Google's Gemini 1.5 Flash model.
	ModelGemini15Flash ServiceName = "gemini-1.5-flash"
	// ModelGemini15Pro is Google's Gemini 1.5 Pro model.
	ModelGemini15Pro ServiceName = "gemini-1.5-pro"
	// ModelClaude35Sonnet is Anthropic's Claude 3.5 Sonnet model.
	ModelClaude35Sonnet ServiceName = "claude-3-5-sonnet@20240620"
	// ModelDalle3 is OpenAI's DALL-E 3 model.
	ModelDalle3 ServiceName = "dall-e-3"
)

// Search Engines
const (
	// SearchEngineBrave is Brave's search engine.
	SearchEngineBrave ServiceName = "brave-search"
	// SearchEngineGoogle is Google's search engine.
	SearchEngineGoogle ServiceName = "google-search"
)

func (m ServiceName) String() string {
	return string(m)
}

type ServiceType string

const (
	ServiceTypePrompt    ServiceType = "prompt"
	ServiceTypeEmbedding ServiceType = "embedding"
	ServiceTypeImageGen  ServiceType = "image_generation"
	ServiceTypeSearch    ServiceType = "search"
)
