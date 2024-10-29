package stripe

import (
	"net/http"
	"strconv"

	"github.com/ditto-assistant/backend/cfg/envs"
	"github.com/ditto-assistant/backend/cfg/secr"
	"github.com/ditto-assistant/backend/pkg/fbase"
	"github.com/stripe/stripe-go/v80"
	"github.com/stripe/stripe-go/v80/checkout/session"
)

func Setup() {
	stripe.Key = secr.STRIPE_SECRET_KEY.String()
}

type requestCreateCheckoutSession struct {
	UserID     string  `json:"userID"`
	Email      *string `json:"email"`
	SuccessURL *string `json:"successURL"`
	CancelURL  *string `json:"cancelURL"`
	USD        int64   `json:"usd"`
}

func (r requestCreateCheckoutSession) GetUserID() string {
	return r.UserID
}

func CreateCheckoutSession(w http.ResponseWriter, r *http.Request) {
	// Parse the form data
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Get authorization from form
	authHeader := r.FormValue("authorization")
	r.Header.Set("Authorization", authHeader)

	fbAuth, err := fbase.NewAuth(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tok, err := fbAuth.VerifyToken(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	// Parse form values into our request struct
	usd, err := strconv.ParseInt(r.FormValue("usd"), 10, 64)
	if err != nil {
		http.Error(w, "invalid USD amount", http.StatusBadRequest)
		return
	}

	bod := requestCreateCheckoutSession{
		UserID:     r.FormValue("userID"),
		Email:      ptr(r.FormValue("email")),
		SuccessURL: ptr(r.FormValue("successURL")),
		CancelURL:  ptr(r.FormValue("cancelURL")),
		USD:        usd,
	}

	err = tok.Check(bod)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
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
