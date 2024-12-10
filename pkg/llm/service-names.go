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

// Anthropic Models
const (
	ModelClaude3Haiku          ServiceName = "claude-3-haiku"
	ModelClaude3Haiku_20240307 ServiceName = "claude-3-haiku@20240307"

	ModelClaude35Haiku          ServiceName = "claude-3-5-haiku"
	ModelClaude35Haiku_20241022 ServiceName = "claude-3-5-haiku@20241022"

	ModelClaude35Sonnet          ServiceName = "claude-3-5-sonnet"
	ModelClaude35Sonnet_20240620 ServiceName = "claude-3-5-sonnet@20240620"

	ModelClaude35SonnetV2          ServiceName = "claude-3-5-sonnet-v2"
	ModelClaude35SonnetV2_20241022 ServiceName = "claude-3-5-sonnet-v2@20241022"
)

// DALL-E Models
const (
	ModelDalle3       ServiceName = "dall-e-3"
	ModelDalle3Wide   ServiceName = "dall-e-3-wide"
	ModelDalle3Tall   ServiceName = "dall-e-3-tall"
	ModelDalle3HD     ServiceName = "dall-e-3-hd"
	ModelDalle3HDWide ServiceName = "dall-e-3-hd-wide"
	ModelDalle3HDTall ServiceName = "dall-e-3-hd-tall"
	ModelDalle2       ServiceName = "dall-e-2"
	ModelDalle2Small  ServiceName = "dall-e-2-small"
	ModelDalle2Tiny   ServiceName = "dall-e-2-tiny"
)

// OpenAI GPT Models
const (
	ModelGPT4o      ServiceName = "gpt-4o"
	ModelGPT4o_1120 ServiceName = "gpt-4o-2024-11-20"

	ModelGPT4oMini          ServiceName = "gpt-4o-mini"
	ModelGPT4oMini_20240718 ServiceName = "gpt-4o-mini-2024-07-18"

	ModelO1Preview          ServiceName = "o1-preview"
	ModelO1Preview_20240912 ServiceName = "o1-preview-2024-09-12"

	ModelO1Mini          ServiceName = "o1-mini"
	ModelO1Mini_20240912 ServiceName = "o1-mini-2024-09-12"
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
