package bfl

// ReqFlux11Pro represents a request to generate an image using FLUX 1.1 [pro].
// It contains all necessary parameters for image generation.
type ReqFlux11Pro struct {
	// Prompt is the text prompt for image generation.
	// Required field that guides the image generation process.
	Prompt string `json:"prompt"`

	// Width of the generated image in pixels.
	//
	//	- Must be a multiple of 32
	//	- Minimum: 256
	//	- Maximum: 1440
	//	- Default: 1024
	Width int `json:"width"`

	// Height of the generated image in pixels.
	//
	//	- Must be a multiple of 32
	// 	- Minimum: 256
	// 	- Maximum: 1440
	// 	- Default: 768
	Height int `json:"height"`

	// PromptUpsampling determines whether to perform upsampling on the prompt.
	//
	//	- If active, automatically modifies the prompt for more creative generation
	//	- Default: false
	PromptUpsampling string `json:"prompt_upsampling"`

	// Seed is an optional value for reproducibility.
	// When provided with the same parameters, generates identical images.
	Seed int `json:"seed"`

	// SafetyTolerance sets the tolerance level for input and output moderation.
	//
	//	- Range: 0-6
	//	- 0: Most strict
	//	- 6: Least strict
	SafetyTolerance int `json:"safety_tolerance"`

	// OutputFormat specifies the format for the generated image.
	//
	//	 - Supported values: 'jpeg' or 'png'
	OutputFormat string `json:"output_format"`
}

// ReqFlux1Dev represents a request to generate an image using FLUX.1 [dev].
// It contains all necessary parameters for image generation.
type ReqFlux1Dev struct {
	// Prompt is the text prompt for image generation.
	// Required field that guides the image generation process.
	Prompt string `json:"prompt"`

	// Width of the generated image in pixels.
	//
	//	- Must be a multiple of 32
	//	- Minimum: 256
	//	- Maximum: 1440
	//	- Default: 1024
	Width int `json:"width"`

	// Height of the generated image in pixels.
	//
	//	- Must be a multiple of 32
	//	- Minimum: 256
	//	- Maximum: 1440
	//	- Default: 768
	Height int `json:"height"`

	// Steps defines the number of steps for the image generation process.
	Steps int `json:"steps"`

	// PromptUpsampling determines whether to perform upsampling on the prompt.
	//
	//	- If active, automatically modifies the prompt for more creative generation
	//	- Default: false
	PromptUpsampling bool `json:"prompt_upsampling"`

	// Seed is an optional value for reproducibility.
	// When provided with the same parameters, generates identical images.
	Seed int `json:"seed"`

	// Guidance scale for image generation.
	// High guidance scales improve prompt adherence at the cost of reduced realism.
	Guidance int `json:"guidance"`

	// SafetyTolerance sets the tolerance level for input and output moderation.
	//
	//	- Range: 0-6
	//	- 0: Most strict
	//	- 6: Least strict
	SafetyTolerance int `json:"safety_tolerance"`

	// OutputFormat specifies the format for the generated image.
	//
	//	- Supported values: 'jpeg' or 'png'
	OutputFormat string `json:"output_format"`
}
