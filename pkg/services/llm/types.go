package llm

// FunctionTool represents a function the model can call
type FunctionTool struct {
	// Type of tool (currently only "function" is supported)
	Type string `json:"type"`
	// Function definition
	Function FunctionToolDef `json:"function"`
}

// FunctionToolDef defines a callable function
type FunctionToolDef struct {
	// Name of the function
	Name string `json:"name"`
	// Description of what the function does
	Description string `json:"description"`
	// Parameters the function accepts (in JSON Schema format)
	Parameters interface{} `json:"parameters"`
}

// FunctionToolChoice controls tool usage
type FunctionToolChoice struct {
	// Type of choice ("none", "auto", "function")
	Type string `json:"type"`
	// Optional specific function to call
	Function *FunctionToolDef `json:"function,omitempty"`
}
