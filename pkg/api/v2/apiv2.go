package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/ditto-assistant/backend/pkg/core"
	"github.com/ditto-assistant/backend/types/rq"
)

type Service struct {
	cl *core.Client
}

func NewService(cl *core.Client) *Service {
	return &Service{
		cl: cl,
	}
}

func (s *Service) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v2/get-memories", s.GetMemories)
}

func (s *Service) GetMemories(w http.ResponseWriter, r *http.Request) {
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
	err = tok.Check(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	slog = slog.With("user_id", req.UserID)
	rsp, err := s.cl.Memories.GetMemoriesV2(r.Context(), &req)
	if err != nil {
		slog.Error("Failed to get memories", "error", err)
		http.Error(w, "Failed to get memories", http.StatusInternalServerError)
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
