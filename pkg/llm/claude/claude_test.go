package claude_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/ditto-assistant/backend/pkg/llm"
	"github.com/ditto-assistant/backend/pkg/llm/claude"
	"github.com/ditto-assistant/backend/types/rq"
)

func TestPrompt(t *testing.T) {
	models := []llm.ServiceName{
		llm.ModelClaude3Haiku_20240307,
		llm.ModelClaude35Sonnet,
	}

	for _, model := range models {
		t.Run(model.String(), func(t *testing.T) {
			ctx := context.Background()
			prompt := "Please respond with a random single token of text."

			var rsp llm.StreamResponse
			err := claude.Prompt(ctx, rq.PromptV1{
				Model:      model,
				UserPrompt: prompt,
			}, &rsp)
			if err != nil {
				t.Fatalf("Error calling Prompt: %v", err)
			}

			fmt.Printf("Claude (%s) response:\n", model)
			for token := range rsp.Text {
				if token.Err != nil {
					t.Fatalf("Error in response: %v", token.Err)
				}
				fmt.Print(token.Ok)
				os.Stdout.Sync()
			}
			fmt.Println()

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

	models := []llm.ServiceName{
		llm.ModelClaude3Haiku_20240307,
		llm.ModelClaude35Sonnet,
	}

	for _, model := range models {
		t.Run(model.String(), func(t *testing.T) {
			ctx := context.Background()
			prompt := "Tell a story about a cat named Hat."

			var rsp llm.StreamResponse
			err := claude.Prompt(ctx, rq.PromptV1{
				Model:      model,
				UserPrompt: prompt,
			}, &rsp)
			if err != nil {
				t.Fatalf("Error calling Prompt: %v", err)
			}

			fmt.Printf("Claude (%s) response:\n", model)
			for token := range rsp.Text {
				if token.Err != nil {
					t.Fatalf("Error in response: %v", token.Err)
				}
				fmt.Print(token.Ok)
				os.Stdout.Sync()
			}
			fmt.Println()

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

func TestImage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping image test in short mode")
	}

	models := []llm.ServiceName{
		llm.ModelClaude3Haiku_20240307,
		llm.ModelClaude35Sonnet,
	}

	for _, model := range models {
		t.Run(model.String(), func(t *testing.T) {
			ctx := context.Background()
			prompt := "Describe the damage in this image and estimate the cost to repair it."

			var rsp llm.StreamResponse
			err := claude.Prompt(ctx, rq.PromptV1{
				Model:      model,
				UserPrompt: prompt,
				ImageURL:   "https://f005.backblazeb2.com/file/public-test-files-garage-weasel/olive_test_images/shower_tile_2/after.jpeg",
			}, &rsp)
			if err != nil {
				t.Fatalf("Error calling Prompt: %v", err)
			}

			fmt.Printf("Claude (%s) response:\n", model)
			for token := range rsp.Text {
				if token.Err != nil {
					t.Fatalf("Error in response: %v", token.Err)
				}
				fmt.Print(token.Ok)
				os.Stdout.Sync()
			}
			fmt.Println()

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
