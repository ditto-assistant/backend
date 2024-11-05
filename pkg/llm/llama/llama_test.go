package llama_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/ditto-assistant/backend/pkg/llm"
	"github.com/ditto-assistant/backend/pkg/llm/llama"
	"github.com/ditto-assistant/backend/types/rq"
)

func TestPrompt(t *testing.T) {
	ctx := context.Background()
	prompt := "Please respond with a random single token of text."

	var rsp llm.StreamResponse
	m := llama.ModelLlama32Vision
	err := m.Prompt(ctx, rq.PromptV1{
		UserPrompt: prompt,
	}, &rsp)
	if err != nil {
		t.Fatalf("Error calling Prompt: %v", err)
	}

	fmt.Println("Llama's response:")
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

func TestLongPrompt(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping long prompt in short mode")
	}
	ctx := context.Background()
	prompt := "Tell a story about a cat named Hat."

	var rsp llm.StreamResponse
	m := llama.ModelLlama32Vision
	err := m.Prompt(ctx, rq.PromptV1{
		UserPrompt: prompt,
	}, &rsp)
	if err != nil {
		t.Fatalf("Error calling Prompt: %v", err)
	}

	fmt.Println("Llama's response:")
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
	m := llama.ModelLlama32Vision
	err := m.Prompt(ctx, rq.PromptV1{
		UserPrompt: prompt,
		ImageURL:   "https://f005.backblazeb2.com/file/public-test-files-garage-weasel/olive_test_images/shower_tile_2/after.jpeg",
	}, &rsp)
	if err != nil {
		t.Fatalf("Error calling Prompt: %v", err)
	}

	fmt.Println("Llama's response:")
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
	m := llama.ModelLlama32Vision
	err := m.Prompt(ctx, rq.PromptV1{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
	}, &rsp)
	if err != nil {
		t.Fatalf("Error calling Prompt: %v", err)
	}

	fmt.Println("Llama's response:")
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