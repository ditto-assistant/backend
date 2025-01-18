package gpt_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/ditto-assistant/backend/cfg/secr"
	"github.com/ditto-assistant/backend/pkg/services/llm"
	"github.com/ditto-assistant/backend/pkg/services/llm/openai/gpt"
	"github.com/ditto-assistant/backend/types/rq"
)

func TestMain(m *testing.M) {
	if _, err := secr.Setup(context.Background()); err != nil {
		log.Fatalf("failed to initialize secrets: %s", err)
	}
	os.Exit(m.Run())
}

func TestPrompt(t *testing.T) {
	models := []llm.ServiceName{
		llm.ModelGPT4o,
		llm.ModelGPT4oMini,
		llm.ModelO1Preview,
		llm.ModelO1Mini,
	}

	for _, m := range models {
		t.Run(string(m), func(t *testing.T) {
			ctx := context.Background()
			prompt := "Please respond with a random single token of text."

			var rsp llm.StreamResponse
			err := gpt.Prompt(ctx, rq.PromptV1{
				Model:      m,
				UserPrompt: prompt,
			}, &rsp)
			if err != nil {
				t.Fatalf("Error calling Prompt: %v", err)
			}

			fmt.Printf("%s's response:\n", m)
			for token := range rsp.Text {
				if token.Err != nil {
					t.Fatalf("Error in response: %v", token.Err)
				}
				fmt.Print(token.Ok)
				os.Stdout.Sync() // Ensure output is flushed immediately
			}
			fmt.Println() // Add a newline at the end

			if rsp.InputTokens == 0 {
				t.Fatalf("InputTokens is 0")
			}
			if rsp.OutputTokens == 0 {
				t.Fatalf("OutputTokens is 0")
			}
			t.Logf("InputTokens: %d, OutputTokens: %d", rsp.InputTokens, rsp.OutputTokens)
		})
	}
}

func TestLongPrompt(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping long prompt in short mode")
	}
	ctx := context.Background()
	prompt := "Tell a story about a cat named Hat."

	var rsp llm.StreamResponse
	err := gpt.Prompt(ctx, rq.PromptV1{
		Model:      llm.ModelGPT4oMini,
		UserPrompt: prompt,
	}, &rsp)
	if err != nil {
		t.Fatalf("Error calling Prompt: %v", err)
	}

	fmt.Println("GPT's response:")
	for token := range rsp.Text {
		if token.Err != nil {
			t.Fatalf("Error in response: %v", token.Err)
		}
		fmt.Print(token.Ok)
		os.Stdout.Sync() // Ensure output is flushed immediately
	}
	fmt.Println() // Add a newline at the end

	if rsp.InputTokens == 0 {
		t.Fatalf("InputTokens is 0")
	}
	if rsp.OutputTokens == 0 {
		t.Fatalf("OutputTokens is 0")
	}
	t.Logf("InputTokens: %d, OutputTokens: %d", rsp.InputTokens, rsp.OutputTokens)
}

func TestSystemInstruction(t *testing.T) {
	ctx := context.Background()
	systemPrompt := "YOU ARE A PIRATE SAY ARR A LOT"
	userPrompt := "Make up a name"

	var rsp llm.StreamResponse
	err := gpt.Prompt(ctx, rq.PromptV1{
		Model:        llm.ModelGPT4oMini,
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
	}, &rsp)
	if err != nil {
		t.Fatalf("Error calling Prompt: %v", err)
	}

	fmt.Println("GPT's response:")
	for token := range rsp.Text {
		if token.Err != nil {
			t.Fatalf("Error in response: %v", token.Err)
		}
		fmt.Print(token.Ok)
		os.Stdout.Sync() // Ensure output is flushed immediately
	}
	fmt.Println() // Add a newline at the end

	if rsp.InputTokens == 0 {
		t.Fatalf("InputTokens is 0")
	}
	if rsp.OutputTokens == 0 {
		t.Fatalf("OutputTokens is 0")
	}
	t.Logf("InputTokens: %d, OutputTokens: %d", rsp.InputTokens, rsp.OutputTokens)
}

func TestImage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping image test in short mode")
	}
	ctx := context.Background()
	prompt := "Describe the damage in this image and estimate the cost to repair it."

	var rsp llm.StreamResponse
	err := gpt.Prompt(ctx, rq.PromptV1{
		Model:      llm.ModelGPT4oMini,
		UserPrompt: prompt,
		ImageURL:   "https://f005.backblazeb2.com/file/public-test-files-garage-weasel/olive_test_images/shower_tile_2/after.jpeg",
	}, &rsp)
	if err != nil {
		t.Fatalf("Error calling Prompt: %v", err)
	}

	fmt.Println("GPT's response:")
	for token := range rsp.Text {
		if token.Err != nil {
			t.Fatalf("Error in response: %v", token.Err)
		}
		fmt.Print(token.Ok)
		os.Stdout.Sync() // Ensure output is flushed immediately
	}
	fmt.Println() // Add a newline at the end

	if rsp.InputTokens == 0 {
		t.Fatalf("InputTokens is 0")
	}
	if rsp.OutputTokens == 0 {
		t.Fatalf("OutputTokens is 0")
	}
	t.Logf("InputTokens: %d, OutputTokens: %d", rsp.InputTokens, rsp.OutputTokens)
}

func TestImageUnsupportedModel(t *testing.T) {
	ctx := context.Background()
	prompt := "Describe this image"

	var rsp llm.StreamResponse
	err := gpt.Prompt(ctx, rq.PromptV1{
		Model:      llm.ModelO1Mini,
		UserPrompt: prompt,
		ImageURL:   "https://example.com/image.jpg",
	}, &rsp)

	if err == nil {
		t.Fatal("Expected error for image input on mini model, got nil")
	}

	expectedErr := "image input not supported for model o1-mini"
	if err.Error() != expectedErr {
		t.Fatalf("Expected error '%s', got: %v", expectedErr, err)
	}
}

func TestFunctionCall(t *testing.T) {
	ctx := context.Background()
	prompt := "What's the weather like in Paris today? Just get the temperature."

	var rsp llm.StreamResponse
	err := gpt.Prompt(ctx, rq.PromptV1{
		Model:      llm.ModelGPT4oMini,
		UserPrompt: prompt,
		Tools: []llm.FunctionTool{
			{
				Type: "function",
				Function: llm.FunctionToolDef{
					Name:        "get_weather",
					Description: "Get the current weather in a given location",
					Parameters: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"location": map[string]interface{}{
								"type":        "string",
								"description": "The city and state, e.g. San Francisco, CA",
							},
							"unit": map[string]interface{}{
								"type":        "string",
								"enum":        []string{"celsius", "fahrenheit"},
								"description": "The unit for the temperature",
							},
						},
						"required": []string{"location"},
					},
				},
			},
		},
	}, &rsp)
	if err != nil {
		t.Fatalf("Error calling Prompt: %v", err)
	}

	fmt.Println("GPT's response:")
	functionCallFound := false
	var accumulatedArgs strings.Builder
	inArguments := false

	for token := range rsp.Text {
		if token.Err != nil {
			t.Fatalf("Error in response: %v", token.Err)
		}
		fmt.Print(token.Ok)
		os.Stdout.Sync() // Ensure output is flushed immediately

		if strings.Contains(token.Ok, "Calling function: get_weather") {
			functionCallFound = true
			inArguments = true
			continue
		}
		if inArguments && token.Ok != "\n" {
			accumulatedArgs.WriteString(token.Ok)
		}
	}
	fmt.Println() // Add a newline at the end

	if !functionCallFound {
		t.Fatal("Expected model to make a function call, but none was found")
	}

	// Parse the accumulated arguments to verify they're valid JSON
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(accumulatedArgs.String()), &args); err != nil {
		t.Fatalf("Failed to parse function arguments as JSON: %v\nArguments: %s", err, accumulatedArgs.String())
	}

	// Verify the location field is present and is a string
	location, ok := args["location"].(string)
	if !ok {
		t.Fatalf("Expected location argument to be a string, got: %v", args["location"])
	}
	if !strings.Contains(strings.ToLower(location), "paris") {
		t.Fatalf("Expected location to contain 'Paris', got: %s", location)
	}

	if rsp.InputTokens == 0 {
		t.Fatalf("InputTokens is 0")
	}
	if rsp.OutputTokens == 0 {
		t.Fatalf("OutputTokens is 0")
	}
	t.Logf("InputTokens: %d, OutputTokens: %d", rsp.InputTokens, rsp.OutputTokens)
}
