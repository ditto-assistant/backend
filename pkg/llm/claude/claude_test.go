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

	var rsp claude.Response
	err := rsp.Prompt(ctx, prompt)
	if err != nil {
		t.Fatalf("Error calling Prompt: %v", err)
	}

	fmt.Println("Claude's response:")
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

	var rsp claude.Response
	err := rsp.Prompt(ctx, prompt)
	if err != nil {
		t.Fatalf("Error calling Prompt: %v", err)
	}

	fmt.Println("Claude's response:")
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
