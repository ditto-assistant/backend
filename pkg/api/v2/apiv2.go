package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/ditto-assistant/backend/pkg/core"
	"github.com/ditto-assistant/backend/pkg/services/db"
	"github.com/ditto-assistant/backend/pkg/services/db/users"
	"github.com/ditto-assistant/backend/pkg/services/llm"
	"github.com/ditto-assistant/backend/pkg/services/llm/cerebras"
	"github.com/ditto-assistant/backend/pkg/services/llm/claude"
	"github.com/ditto-assistant/backend/pkg/services/llm/gemini"
	"github.com/ditto-assistant/backend/pkg/services/llm/llama"
	"github.com/ditto-assistant/backend/pkg/services/llm/mistral"
	"github.com/ditto-assistant/backend/pkg/services/llm/openai/gpt"
	"github.com/ditto-assistant/backend/types/rq"
	"github.com/ditto-assistant/backend/types/ty"
)

func (s *Service) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v2/get-memories", s.GetMemoriesV2)
	mux.HandleFunc("POST /api/v2/prompt", s.PromptV2)
}

type Service struct {
	cl *core.Client
	sd ty.ShutdownContext
}

func NewService(cl *core.Client, sd ty.ShutdownContext) *Service {
	return &Service{
		cl: cl,
		sd: sd,
	}
}

func (s *Service) GetMemoriesV2(w http.ResponseWriter, r *http.Request) {
	slog := slog.With("handler", "GetMemoriesV2")
	tok, err := s.cl.Auth.VerifyToken(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	var req rq.GetMemoriesV2
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Error("Failed to decode request body", "error", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	err = tok.Check(req.UserID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	slog = slog.With("userID", req.UserID)
	rsp, err := s.cl.Memories.GetMemoriesV2(r.Context(), &req)
	if err != nil {
		slog.Error("Failed to get memories", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	switch r.Header.Get("Accept") {
	case "application/json":
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rsp)
	case "text/plain":
		w.Header().Set("Content-Type", "text/plain")
		w.Write(rsp.Bytes())
	default:
		http.Error(w, "Unsupported media type", http.StatusUnsupportedMediaType)
	}
}

// PromptV2 handles the v2 prompt endpoint with SSE streaming
func (s *Service) PromptV2(w http.ResponseWriter, r *http.Request) {
	slog := slog.With("handler", "PromptV2")
	tok, err := s.cl.Auth.VerifyToken(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	var bod rq.PromptV1
	if err := json.NewDecoder(r.Body).Decode(&bod); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	err = tok.Check(bod.UserID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	user := users.User{UID: bod.UserID}
	ctx := r.Context()
	if err := user.GetByUID(ctx, db.D); err != nil {
		slog.Error("failed to get user", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	slog = slog.With("action", "prompt", "userID", bod.UserID, "model", bod.Model, "email", user.Email.String)
	// llama32 is free
	if user.Balance <= 0 && bod.Model != llm.ModelLlama32 {
		slog.Error("user balance is 0", "balance", user.Balance)
		http.Error(w, fmt.Sprintf("user balance is: %d", user.Balance), http.StatusPaymentRequired)
		return
	}
	bod.Model, err = llama.ModelCompat(bod.Model, bod.ImageURL)
	if err != nil {
		slog.Error("invalid llama model", "error", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var rsp llm.StreamResponse
	switch bod.Model {
	case
		llm.ModelClaude3Haiku, llm.ModelClaude3Haiku_20240307,
		llm.ModelClaude35Sonnet, llm.ModelClaude35Sonnet_20240620,
		llm.ModelClaude35SonnetV2, llm.ModelClaude35SonnetV2_20241022,
		llm.ModelClaude35Haiku, llm.ModelClaude35Haiku_20241022:
		err = claude.Prompt(ctx, bod, &rsp)
		if err != nil {
			slog.Error("failed to prompt Claude", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	case llm.ModelGemini15Flash:
		m := gemini.ModelGemini15Flash
		err = m.Prompt(ctx, bod, &rsp)
		if err != nil {
			slog.Error("failed to prompt "+m.PrettyStr(), "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	case llm.ModelGemini15Pro:
		m := gemini.ModelGemini15Pro
		err = m.Prompt(ctx, bod, &rsp)
		if err != nil {
			slog.Error("failed to prompt "+m.PrettyStr(), "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	case llm.ModelMistralNemo, llm.ModelMistralLarge:
		err = mistral.Prompt(ctx, bod, &rsp)
		if err != nil {
			slog.Error("failed to prompt "+bod.Model.String(), "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	case llm.ModelLlama33_70bInstruct:
		err = llama.Prompt(ctx, bod, &rsp)
		if err != nil {
			slog.Error("failed to prompt "+llama.ModelLlama32.PrettyStr(), "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	case
		llm.ModelO1Mini, llm.ModelO1Mini_20240912,
		llm.ModelO1Preview, llm.ModelO1Preview_20240912,
		llm.ModelGPT4oMini, llm.ModelGPT4oMini_20240718,
		llm.ModelGPT4o, llm.ModelGPT4o_1120:
		err = gpt.Prompt(ctx, bod, &rsp)
		if err != nil {
			slog.Error("failed to prompt "+bod.Model.String(), "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	case llm.ModelCerebrasLlama8B, llm.ModelCerebrasLlama70B:
		cerebrasClient := cerebras.NewService(&s.sd, s.cl.Secr)
		err = cerebrasClient.Prompt(ctx, bod, &rsp)
		if err != nil {
			slog.Error("failed to prompt "+bod.Model.String(), "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	default:
		slog.Info("unsupported model", "model", bod.Model)
		http.Error(w, fmt.Sprintf("unsupported model: %s", bod.Model), http.StatusBadRequest)
		return
	}

	// Set up SSE response headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		slog.Error("Streaming not supported")
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Process and send tokens as SSE events
	for token := range rsp.Text {
		if token.Err != nil {
			slog.Error("failed to stream token", "error", token.Err)
			// Send error event
			errEvent := map[string]string{
				"type": "error",
				"data": token.Err.Error(),
			}
			eventJSON, _ := json.Marshal(errEvent)
			fmt.Fprintf(w, "data: %s\n\n", eventJSON)
			flusher.Flush()
			return
		}

		// Send text event
		textEvent := map[string]string{
			"type": "text",
			"data": token.Ok,
		}
		eventJSON, _ := json.Marshal(textEvent)
		fmt.Fprintf(w, "data: %s\n\n", eventJSON)
		flusher.Flush()
	}

	// Send done event
	fmt.Fprintf(w, "data: {\"type\":\"done\"}\n\n")
	flusher.Flush()

	s.sd.Run(func(ctx context.Context) {
		slog.Debug("receipt", "input_tokens", rsp.InputTokens, "output_tokens", rsp.OutputTokens)
		receipt := db.Receipt{
			UserID:       user.ID,
			InputTokens:  int64(rsp.InputTokens),
			OutputTokens: int64(rsp.OutputTokens),
			ServiceName:  bod.Model,
		}
		if err := receipt.Insert(ctx); err != nil {
			slog.Error("failed to insert receipt", "error", err)
		}
	})
}
