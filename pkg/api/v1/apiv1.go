package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/ditto-assistant/backend/pkg/db/users"
	"github.com/ditto-assistant/backend/pkg/fbase"
	"github.com/ditto-assistant/backend/pkg/search"
	"github.com/ditto-assistant/backend/types/rq"
)

type Service struct {
	Auth         fbase.Auth
	SearchClient *search.Client
}

func (s *Service) WebSearch(w http.ResponseWriter, r *http.Request) {
	tok, err := s.Auth.VerifyToken(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	var bod rq.SearchV1
	if err := json.NewDecoder(r.Body).Decode(&bod); err != nil {
		if err == io.EOF {
			http.Error(w, "request body is empty", http.StatusBadRequest)
		} else {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
		return
	}
	err = tok.Check(bod)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	if bod.NumResults == 0 {
		bod.NumResults = 5
	}
	user := users.User{UID: bod.UserID}
	ctx := r.Context()
	if err := user.Get(ctx); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if user.Balance <= 0 {
		http.Error(w, fmt.Sprintf("user balance is: %d", user.Balance), http.StatusPaymentRequired)
		return
	}
	searchRequest := search.Request{
		User:       user,
		Query:      bod.Query,
		NumResults: bod.NumResults,
	}
	search, err := s.SearchClient.Search(ctx, searchRequest)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	search.Text(w)
}

func (s *Service) Balance(w http.ResponseWriter, r *http.Request) {
	tok, err := s.Auth.VerifyToken(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	var bod rq.BalanceV1
	if err := bod.FromQuery(r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	err = tok.Check(bod)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	rsp, err := users.GetBalance(r.Context(), bod)
	if err != nil {
		slog.Error("failed to handle balance request", "uid", bod.UserID, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rsp)
}
