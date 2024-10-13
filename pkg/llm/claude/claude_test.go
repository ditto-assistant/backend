package claude_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/ditto-assistant/backend/pkg/llm/claude"
)

func TestPrompt(t *testing.T) {
	ctx := context.Background()
	prompt := "Please respond with a random single token of text."

	streamChan, err := claude.Prompt(ctx, prompt)
	if err != nil {
		t.Fatalf("Error calling Prompt: %v", err)
	}

	fmt.Println("Claude's response:")
	for token := range streamChan {
		fmt.Print(token)
		os.Stdout.Sync() // Ensure output is flushed immediately
	}
	fmt.Println() // Add a newline at the end
}

func TestLongPrompt(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping long prompt in short mode")
	}
	ctx := context.Background()
	prompt := "Tell a story about a cat named Hat."

	streamChan, err := claude.Prompt(ctx, prompt)
	if err != nil {
		t.Fatalf("Error calling Prompt: %v", err)
	}

	fmt.Println("Claude's response:")
	for token := range streamChan {
		fmt.Print(token)
		os.Stdout.Sync() // Ensure output is flushed immediately
	}
	fmt.Println() // Add a newline at the end
}
