package gemini_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/ditto-assistant/backend/pkg/services/llm"
	"github.com/ditto-assistant/backend/pkg/services/llm/gemini"
	"github.com/ditto-assistant/backend/types/rq"
)

func TestPrompt(t *testing.T) {
	models := []gemini.Model{
		gemini.ModelGemini15Flash,
		gemini.ModelGemini15Pro,
	}
	for _, m := range models {
		t.Run(string(m), func(t *testing.T) {
			t.Logf("Testing model: %s", m.PrettyStr())
			ctx := context.Background()
			prompt := "Please respond with a random single token of text."

			var rsp llm.StreamResponse
			err := m.Prompt(ctx, rq.PromptV1{
				UserPrompt: prompt,
			}, &rsp)
			if err != nil {
				t.Fatalf("Error calling Prompt: %v", err)
			}

			fmt.Printf("%s response:\n", m.PrettyStr())
			for token := range rsp.Text {
				if token.Err != nil {
					t.Fatalf("Error in response: %v", token.Err)
				}
				fmt.Print(token.Ok)
				os.Stdout.Sync() // Ensure output is flushed immediately
			}
			fmt.Println() // Add a newline at the end

			// Check token counts after all tokens have been received
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
	err := gemini.ModelGemini15Flash.Prompt(ctx, rq.PromptV1{
		UserPrompt: prompt,
	}, &rsp)
	if err != nil {
		t.Fatalf("Error calling Prompt: %v", err)
	}

	fmt.Println("Gemini's response:")
	for token := range rsp.Text {
		if token.Err != nil {
			t.Fatalf("Error in response: %v", token.Err)
		}
		fmt.Print(token.Ok)
		os.Stdout.Sync() // Ensure output is flushed immediately
	}
	fmt.Println() // Add a newline at the end

	// Check token counts after all tokens have been received
	if rsp.InputTokens == 0 {
		t.Logf("Warning: InputTokens is 0")
	}
	if rsp.OutputTokens == 0 {
		t.Logf("Warning: OutputTokens is 0")
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
	err := gemini.ModelGemini15Flash.Prompt(ctx, rq.PromptV1{
		UserPrompt: prompt,
		ImageURL:   "https://f005.backblazeb2.com/file/public-test-files-garage-weasel/olive_test_images/shower_tile_2/after.jpeg",
	}, &rsp)
	if err != nil {
		t.Fatalf("Error calling Prompt: %v", err)
	}

	fmt.Println("Gemini's response:")
	for token := range rsp.Text {
		if token.Err != nil {
			t.Fatalf("Error in response: %v", token.Err)
		}
		fmt.Print(token.Ok)
		os.Stdout.Sync() // Ensure output is flushed immediately
	}
	fmt.Println() // Add a newline at the end

	// Check token counts after all tokens have been received
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
	systemPrompt := "Always respond with 1 token"
	userPrompt := "Tell me a story"

	var rsp llm.StreamResponse
	err := gemini.ModelGemini15Pro.Prompt(ctx, rq.PromptV1{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
	}, &rsp)
	if err != nil {
		t.Fatalf("Error calling Prompt: %v", err)
	}

	fmt.Println("Gemini's response:")
	responseTokens := 0
	for token := range rsp.Text {
		if token.Err != nil {
			t.Fatalf("Error in response: %v", token.Err)
		}
		fmt.Print(token.Ok)
		os.Stdout.Sync()
		responseTokens++
	}
	fmt.Println() // Add a newline at the end

	if responseTokens != 1 {
		t.Errorf("Expected 1 token, got %d tokens", responseTokens)
	}

	// Check token counts after all tokens have been received
	if rsp.InputTokens == 0 {
		t.Fatalf("InputTokens is 0")
	}
	if rsp.OutputTokens == 0 {
		t.Fatalf("OutputTokens is 0")
	}
	t.Logf("InputTokens: %d, OutputTokens: %d", rsp.InputTokens, rsp.OutputTokens)
}
