package stripe

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/ditto-assistant/backend/cfg/secr"
	"github.com/stripe/stripe-go/v80"
	"github.com/stripe/stripe-go/v80/webhook"
)

// func main() {
//   // This is your test secret API key.
//   stripe.Key = "sk_test_51Q73h5ICo0zHqarySfvYwr7hqfpm63rwy473KF7Vnr78o0bnLOt3Hh6FDetIC5UBlFvlk514hLUvFwRCikPR12io00LttPDapQ"

//   http.HandleFunc("/webhook", handleWebhook)
//   addr := "localhost:4242"
//   log.Printf("Listening on %s", addr)
//   log.Fatal(http.ListenAndServe(addr, nil))
// }

func HandleWebhook(w http.ResponseWriter, req *http.Request) {
	const MaxBodyBytes = int64(65536)
	req.Body = http.MaxBytesReader(w, req.Body, MaxBodyBytes)
	payload, err := io.ReadAll(req.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading request body: %v\n", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	event := stripe.Event{}

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
	// Unmarshal the event data into an appropriate struct depending on its Type
	switch event.Type {
	case "checkout.session.async_payment_failed":
		var session stripe.CheckoutSession
		err := json.Unmarshal(event.Data.Raw, &session)
		if err != nil {
			slog.Error("error parsing webhook JSON", "error", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		slog.Info("async payment failed",
			"session_id", session.ID,
			"customer", session.Customer.ID,
			"payment_status", session.PaymentStatus)
		// TODO: Handle failed payment - notify user, update order status, etc.

	case "checkout.session.async_payment_succeeded":
		var session stripe.CheckoutSession
		err := json.Unmarshal(event.Data.Raw, &session)
		if err != nil {
			slog.Error("error parsing webhook JSON", "error", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		slog.Info("async payment succeeded",
			"session_id", session.ID,
			"customer", session.Customer.ID,
			"payment_status", session.PaymentStatus)
		// TODO: Handle successful payment - fulfill order, update database, etc.

	case "checkout.session.completed":
		var session stripe.CheckoutSession
		err := json.Unmarshal(event.Data.Raw, &session)
		if err != nil {
			slog.Error("error parsing webhook JSON", "error", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		uid := session.Metadata["userID"]
		invoice := session.Invoice
		slog.Info("checkout session completed",
			"session_id", session.ID,
			// "customer", session.Customer.ID,
			"payment_status", session.PaymentStatus,
			"userID", uid,
			"invoice", invoice)
		// TODO: Handle completed session - confirm order completion, send confirmation email, etc.

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
		invoice := paymentIntent.Invoice
		slog.Info("successful payment", "amount", paymentIntent.Amount, "userID", uid, "invoice", invoice, "id", id)
		// Then define and call a func to handle the successful payment intent.
		// handlePaymentIntentSucceeded(paymentIntent)

	case "payment_method.attached":
		var paymentMethod stripe.PaymentMethod
		err := json.Unmarshal(event.Data.Raw, &paymentMethod)
		if err != nil {
			slog.Error("error parsing webhook JSON", "error", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// Then define and call a func to handle the successful attachment of a PaymentMethod.
		// handlePaymentMethodAttached(paymentMethod)
	default:
		slog.Debug("unhandled event type", "type", event.Type)
	}

	w.WriteHeader(http.StatusOK)
}
