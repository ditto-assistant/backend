package stripe

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"sync"

	"github.com/ditto-assistant/backend/cfg/envs"
	"github.com/ditto-assistant/backend/cfg/secr"
	"github.com/ditto-assistant/backend/pkg/services/authfirebase"
	"github.com/stripe/stripe-go/v80"
	"github.com/stripe/stripe-go/v80/checkout/session"
)

func (cl *Client) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/stripe/checkout-session", cl.CreateCheckoutSession)
	mux.HandleFunc("POST /v1/stripe/webhook", cl.HandleWebhook)
}

type Client struct {
	secr          *secr.Client
	auth          *authfirebase.Client
	mu            sync.RWMutex
	webhookSecret string
}

func NewClient(secr *secr.Client, auth *authfirebase.Client) *Client {
	return &Client{secr: secr, auth: auth}
}

type requestCreateCheckoutSession struct {
	UserID     string  `json:"userID"`
	Email      *string `json:"email"`
	SuccessURL *string `json:"successURL"`
	CancelURL  *string `json:"cancelURL"`
	USD        int64   `json:"usd"`
}

const secretKeyId = "STRIPE_SECRET_KEY"

func (cl *Client) setupCheckoutSession(ctx context.Context) error {
	cl.mu.RLock()
	if stripe.Key != "" {
		cl.mu.RUnlock()
		return nil
	}
	cl.mu.RUnlock()

	cl.mu.Lock()
	defer cl.mu.Unlock()
	if stripe.Key != "" {
		return nil
	}
	stripeKey, err := cl.secr.FetchEnv(ctx, secretKeyId)
	if err != nil {
		return err
	}
	stripe.Key = stripeKey
	slog.Debug("loaded stripe secret key", "secret_id", secretKeyId)
	return nil
}

func (cl *Client) CreateCheckoutSession(w http.ResponseWriter, r *http.Request) {
	if err := cl.setupCheckoutSession(r.Context()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tok, err := cl.auth.VerifyTokenFromForm(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	userID := r.FormValue("userID")
	err = tok.Check(userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	usd, err := strconv.ParseInt(r.FormValue("usd"), 10, 64)
	if err != nil {
		http.Error(w, "invalid USD amount", http.StatusBadRequest)
		return
	}
	bod := requestCreateCheckoutSession{
		UserID:     userID,
		Email:      ptr(r.FormValue("email")),
		SuccessURL: ptr(r.FormValue("successURL")),
		CancelURL:  ptr(r.FormValue("cancelURL")),
		USD:        usd,
	}

	var priceID string
	quantity := int64(1)
	if bod.USD < 10 {
		priceID = envs.PRICE_ID_TOKENS_1B
		quantity = bod.USD
	} else if bod.USD == 10 {
		priceID = envs.PRICE_ID_TOKENS_11B
	} else if bod.USD == 25 {
		priceID = envs.PRICE_ID_TOKENS_30B
	} else if bod.USD == 50 {
		priceID = envs.PRICE_ID_TOKENS_30B
		quantity = 2
	} else if bod.USD == 75 {
		priceID = envs.PRICE_ID_TOKENS_100B
	} else if bod.USD == 100 {
		priceID = envs.PRICE_ID_TOKENS_150B
	} else {
		priceID = envs.PRICE_ID_TOKENS_1B
		quantity = bod.USD
	}

	params := &stripe.CheckoutSessionParams{
		Metadata: map[string]string{
			"userID": bod.UserID,
		},
		CustomerEmail: bod.Email,
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Quantity: stripe.Int64(quantity),
				AdjustableQuantity: &stripe.CheckoutSessionLineItemAdjustableQuantityParams{
					Enabled: stripe.Bool(true),
				},
				Price: stripe.String(priceID),
			},
		},
		Mode:         stripe.String(string(stripe.CheckoutSessionModePayment)),
		SuccessURL:   bod.SuccessURL,
		CancelURL:    bod.CancelURL,
		AutomaticTax: &stripe.CheckoutSessionAutomaticTaxParams{Enabled: stripe.Bool(true)},
		PaymentIntentData: &stripe.CheckoutSessionPaymentIntentDataParams{
			Metadata: map[string]string{
				"userID": bod.UserID,
			},
		},
	}

	s, err := session.New(params)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, s.URL, http.StatusSeeOther)
}

// Helper function to convert string to pointer
// If the string is empty, return nil
func ptr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
