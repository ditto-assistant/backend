package stripe

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/ditto-assistant/backend/cfg/secr"
	"github.com/ditto-assistant/backend/pkg/db"
	"github.com/ditto-assistant/backend/pkg/numfmt"
	"github.com/stripe/stripe-go/v80"
	"github.com/stripe/stripe-go/v80/webhook"
)

// calculateTokens determines the number of tokens based on payment amount in cents
func calculateTokens(cents int64) int64 {
	switch cents {
	case 1000: // $10
		return 11_000_000_000 // 11B tokens (10% bonus)
	case 2500: // $25
		return 30_000_000_000 // 30B tokens (20% bonus)
	case 5000: // $50
		return 65_000_000_000 // 65B tokens (30% bonus)
	case 7500: // $75
		return 100_000_000_000 // 100B tokens (33% bonus)
	case 10000: // $100
		return 150_000_000_000 // 150B tokens (50% bonus)
	default:
		// Default: $1 per 1B tokens
		return (cents / 100) * 1_000_000_000
	}
}

func HandleWebhook(w http.ResponseWriter, req *http.Request) {
	const MaxBodyBytes = int64(65536)
	req.Body = http.MaxBytesReader(w, req.Body, MaxBodyBytes)
	payload, err := io.ReadAll(req.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading request body: %v\n", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	var event stripe.Event
	if err := json.Unmarshal(payload, &event); err != nil {
		slog.Error("webhook error while parsing basic request", "error", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Replace this endpoint secret with your endpoint's unique secret
	// If you are testing with the CLI, find the secret by running 'stripe listen'
	// If you are using an endpoint defined with the API or dashboard, look in your webhook settings
	// at https://dashboard.stripe.com/webhooks
	endpointSecret := secr.STRIPE_WEBHOOK_SECRET.String()
	signatureHeader := req.Header.Get("Stripe-Signature")
	event, err = webhook.ConstructEvent(payload, signatureHeader, endpointSecret)
	if err != nil {
		slog.Error("webhook signature verification failed", "error", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	switch event.Type {
	case "payment_intent.succeeded":
		var paymentIntent stripe.PaymentIntent
		err := json.Unmarshal(event.Data.Raw, &paymentIntent)
		if err != nil {
			slog.Error("error parsing webhook JSON", "error", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		uid := paymentIntent.Metadata["userID"]
		id := paymentIntent.ID
		tokens := calculateTokens(paymentIntent.Amount)
		slog.Info("successful payment",
			"amount", paymentIntent.Amount,
			"userID", uid,
			"payment_id", id,
			"tokens", tokens,
		)
		p := db.Purchase{
			PaymentID: id,
			Cents:     paymentIntent.Amount,
			Tokens:    tokens,
		}
		err = p.Insert(uid)
		if err != nil {
			slog.Error("error inserting purchase", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		slog.Info("purchase inserted", "id", p.ID, "tokens", numfmt.LargeNumber(p.Tokens))

	default:
		slog.Debug("unhandled event type", "type", event.Type)
	}

	w.WriteHeader(http.StatusOK)
}