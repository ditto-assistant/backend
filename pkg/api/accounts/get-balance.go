package accounts

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/ditto-assistant/backend/pkg/db/users"
	auth "github.com/ditto-assistant/backend/pkg/fbase"
	"github.com/ditto-assistant/backend/types/rq"
)

func GetBalanceV1(w http.ResponseWriter, r *http.Request) {
	fbAuth, err := auth.NewAuth(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tok, err := fbAuth.VerifyToken(r)
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

	rsp, err := users.HandleGetBalance(r.Context(), bod)
	if err != nil {
		slog.Error("failed to handle balance request", "uid", bod.UserID, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rsp)
}
