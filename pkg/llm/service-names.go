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
	ModelClaude35Sonnet ServiceName = "claude-3-5-sonnet"
	// ModelDalle3 is OpenAI's DALL-E 3 model.
	ModelDalle3 ServiceName = "dall-e-3"
	// ModelMistralNemo is Mistral's nemo model.
	ModelMistralNemo ServiceName = "mistral-nemo"
	// ModelMistralLarge is Mistral's large model.
	ModelMistralLarge ServiceName = "mistral-large"
	// ModelLlama32 is Meta's Llama 3.2 model.
	ModelLlama32 ServiceName = "llama-3-2"
	// ModelFlux11ProUltra is BFL's FLUX 1.1 Pro Ultra model
	ModelFlux11ProUltra ServiceName = "flux-1-1-pro-ultra"
	// ModelFlux11Pro is BFL's FLUX 1.1 Pro model
	ModelFlux11Pro ServiceName = "flux-1-1-pro"
	// ModelFlux1Pro is BFL's FLUX.1 Pro model
	ModelFlux1Pro ServiceName = "flux-1-pro"
	// ModelFlux1Dev is BFL's FLUX.1 Dev model
	ModelFlux1Dev ServiceName = "flux-1-dev"
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
