package cerebras_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/ditto-assistant/backend/cfg/secr"
	"github.com/ditto-assistant/backend/pkg/services/llm"
	"github.com/ditto-assistant/backend/pkg/services/llm/cerebras"
	"github.com/ditto-assistant/backend/types/rq"
	"github.com/ditto-assistant/backend/types/ty"
)

var service *cerebras.Service

func TestMain(m *testing.M) {
	ctx := context.Background()
	secretsClient, err := secr.Setup(ctx)
	if err != nil {
		fmt.Printf("failed to initialize secrets: %s\n", err)
		os.Exit(1)
	}
	var shutdownWG sync.WaitGroup
	sd := ty.ShutdownContext{
		Background: ctx,
		WaitGroup:  &shutdownWG,
	}
	service = cerebras.NewService(&sd, secretsClient)
	os.Exit(m.Run())
}

func TestPrompt(t *testing.T) {
	models := []llm.ServiceName{
		llm.ModelCerebrasLlama8B,
		llm.ModelCerebrasLlama70B,
	}

	for _, m := range models {
		t.Run(string(m), func(t *testing.T) {
			ctx := context.Background()
			prompt := "Please respond with a random single token of text."

			var rsp llm.StreamResponse
			err := service.Prompt(ctx, rq.PromptV1{
				Model:      m,
				UserPrompt: prompt,
			}, &rsp)
			if err != nil {
				t.Fatalf("Error calling Prompt: %v", err)
			}

			fmt.Printf("Cerebras (%s) response:\n", m)
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

	ctx := context.Background()
	prompt := "Tell a story about a cat named Hat."

	var rsp llm.StreamResponse
	err := service.Prompt(ctx, rq.PromptV1{
		Model:      llm.ModelCerebrasLlama8B,
		UserPrompt: prompt,
	}, &rsp)
	if err != nil {
		t.Fatalf("Error calling Prompt: %v", err)
	}

	fmt.Printf("Cerebras response:\n")
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
}

func TestSystemInstruction(t *testing.T) {
	ctx := context.Background()
	systemPrompt := "YOU ARE A PIRATE SAY ARR A LOT"
	userPrompt := "Make up a name"

	var rsp llm.StreamResponse
	err := service.Prompt(ctx, rq.PromptV1{
		Model:        llm.ModelCerebrasLlama8B,
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
	}, &rsp)
	if err != nil {
		t.Fatalf("Error calling Prompt: %v", err)
	}

	fmt.Printf("Cerebras response:\n")
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
}
