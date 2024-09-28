package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/ditto-assistant/backend/pkg/img"
	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/firebase"
	"github.com/firebase/genkit/go/plugins/vertexai"
	"github.com/rs/cors"
)

type HasUserID interface {
	GetUserID() string
}

var _, _ HasUserID = ChatRequestV2{}, PromptRequestV1{}

type ChatRequestV2 struct {
	UserID string `json:"userID"`
}

func (c ChatRequestV2) GetUserID() string { return c.UserID }

type PromptRequestV1 struct {
	UserID         string `json:"userID"`
	UserPrompt     string `json:"userPrompt"`
	SystemPrompt   string `json:"systemPrompt"`
	Model          string `json:"model,omitempty"`
	ImageURL       string `json:"imageURL,omitempty"`
	UsersOpenaiKey string `json:"usersOpenaiKey,omitempty"`
}

func (p PromptRequestV1) GetUserID() string { return p.UserID }

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGKILL, syscall.SIGINT)

	if err := vertexai.Init(ctx, &vertexai.Config{
		ProjectID: "ditto-app-dev",
		Location:  "us-central1",
	}); err != nil {
		log.Fatal(err)
	}

	firebaseAuth, err := firebase.NewAuth(ctx, func(authContext genkit.AuthContext, input any) error {
		in, ok := input.(HasUserID) // The type must match the input type of the flow.
		if !ok {
			return fmt.Errorf("request body type is incorrect: %T", input)
		}
		uidIn := in.GetUserID()
		if len(authContext) == 0 {
			return fmt.Errorf("authContext is empty; input uid: %s", uidIn)
		}
		uidAuth, ok := authContext["uid"]
		if !ok {
			return fmt.Errorf("authContext missing uid: %v", authContext)
		}
		if uidAuth, ok := uidAuth.(string); !ok {
			return fmt.Errorf("authContext uid is not a string: %v", uidAuth)
		} else if uidAuth != uidIn {
			return fmt.Errorf("user ID does not match: authContext uid: %v != input uid: %s", uidAuth, uidIn)
		}
		return nil
	}, true)
	if err != nil {
		log.Fatalf("failed to set up Firebase auth: %v", err)
	}

	// genkit.DefineStreamingFlow("v2/chat")
	genkit.DefineStreamingFlow("v1/prompt",
		func(ctx context.Context, in PromptRequestV1, callback func(context.Context, string) error) (string, error) {
			if in.Model != "" {
				if !vertexai.IsDefinedModel(in.Model) {
					return "", fmt.Errorf("promptFlow: model not found: %s", in.Model)
				}
			} else {
				in.Model = "gemini-1.5-pro"
			}
			m := vertexai.Model(in.Model)
			messages := []*ai.Message{
				ai.NewSystemTextMessage(in.SystemPrompt),
				ai.NewUserTextMessage(in.UserPrompt),
			}
			if in.ImageURL != "" {
				imgPart, err := img.NewPart(ctx, in.ImageURL)
				if err != nil {
					return "", err
				}
				messages = append(messages, ai.NewUserMessage(imgPart))
			}
			cfg := &ai.GenerationCommonConfig{Temperature: 0.5}
			resp, err := m.Generate(ctx,
				ai.NewGenerateRequest(cfg, messages...),
				func(ctx context.Context, grc *ai.GenerateResponseChunk) error {
					if callback == nil {
						return nil
					}
					return callback(ctx, grc.Text())
				},
			)
			if err != nil {
				return "", err
			}
			return resp.Text(), nil
		},
		genkit.WithFlowAuth(firebaseAuth),
	)

	go func() {
		err := genkit.Init(ctx, &genkit.Options{FlowAddr: "-"})
		if err != nil {
			log.Fatalf("failed to initialize genkit: %v", err)
		}
	}()

	corsMiddleware := cors.New(cors.Options{
		AllowedOrigins: []string{"http://localhost:3000", "https://assistant.heyditto.ai"}, // Allow all origins
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"*"}, // Allow all headers
		MaxAge:         86400,         // 24 hours
	})
	mux := genkit.NewFlowServeMux(nil)
	handler := corsMiddleware.Handler(mux)
	server := &http.Server{
		Addr:    ":3400",
		Handler: handler,
	}

	go func() {
		select {
		case sig := <-sigChan:
			slog.Info("Received SIG; shutting down", "signal", sig)
			server.Shutdown(ctx)
		}
	}()

	slog.Debug("Starting server", "addr", server.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
	os.Exit(0)
}
