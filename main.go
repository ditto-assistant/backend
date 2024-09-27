package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/firebase"
	"github.com/firebase/genkit/go/plugins/vertexai"
	"github.com/rs/cors"
)

func main() {
	ctx := context.Background()
	if err := vertexai.Init(ctx, &vertexai.Config{
		ProjectID: "ditto-app-dev",
		Location:  "us-central1",
	}); err != nil {
		log.Fatal(err)
	}
	type PromptRequest struct {
		UserID         string `json:"userID"`
		UserPrompt     string `json:"userPrompt"`
		SystemPrompt   string `json:"systemPrompt"`
		Model          string `json:"model,omitempty"`
		ImageURL       string `json:"imageURL,omitempty"`
		UsersOpenaiKey string `json:"usersOpenaiKey,omitempty"`
	}
	firebaseAuth, err := firebase.NewAuth(ctx, func(authContext genkit.AuthContext, input any) error {
		in, ok := input.(PromptRequest) // The type must match the input type of the flow.
		if !ok {
			return fmt.Errorf("request body type is incorrect: %T", input)
		}
		if len(authContext) == 0 {
			return fmt.Errorf("authContext is empty; input uid: %s", in.UserID)
		}
		uid, ok := authContext["uid"]
		if !ok {
			return fmt.Errorf("authContext missing uid: %v", authContext)
		}
		if uid, ok := uid.(string); !ok {
			return fmt.Errorf("authContext uid is not a string: %v", uid)
		} else if uid != in.UserID {
			return fmt.Errorf("user ID does not match: authContext uid: %v != input uid: %s", uid, in.UserID)
		}
		return nil
	}, true)
	if err != nil {
		log.Fatalf("failed to set up Firebase auth: %v", err)
	}

	genkit.DefineStreamingFlow("v1/prompt",
		func(ctx context.Context, input PromptRequest, callback func(context.Context, string) error) (string, error) {
			m := vertexai.Model("gemini-1.5-flash")
			if m == nil {
				return "", errors.New("promptFlow: failed to find model")
			}
			resp, err := m.Generate(ctx,
				ai.NewGenerateRequest(
					&ai.GenerationCommonConfig{Temperature: 0.5},
					ai.NewSystemTextMessage(input.SystemPrompt),
					ai.NewUserTextMessage(input.UserPrompt),
				),
				func(ctx context.Context, grc *ai.GenerateResponseChunk) error { return callback(ctx, grc.Text()) },
			)
			if err != nil {
				return "", err
			}
			return resp.Text(), nil
		},
		genkit.WithFlowAuth(firebaseAuth),
	)

	mux := genkit.NewFlowServeMux(nil)
	corsMiddleware := cors.New(cors.Options{
		AllowedOrigins: []string{"http://localhost:3000", "https://assistant.heyditto.ai"}, // Allow all origins
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"*"}, // Allow all headers
		MaxAge:         86400,         // 24 hours
	})
	handler := corsMiddleware.Handler(mux)
	server := &http.Server{
		Addr:    ":3400",
		Handler: handler,
	}

	// TODO: Handle graceful shutdown
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
}
