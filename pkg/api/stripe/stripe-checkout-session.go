package stripe

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/ditto-assistant/backend/cfg/secr"
	"github.com/ditto-assistant/backend/pkg/fbase"
	"github.com/stripe/stripe-go/v80"
	"github.com/stripe/stripe-go/v80/checkout/session"
)

func SetupStripe() {
	stripe.Key = secr.STRIPE_SECRET_KEY.String()
}

type requestCreateCheckoutSession struct {
	UserID     string `json:"userID"`
	SuccessURL string `json:"successURL"`
	CancelURL  string `json:"cancelURL"`
}

func (r requestCreateCheckoutSession) GetUserID() string {
	return r.UserID
}

func CreateCheckoutSession(w http.ResponseWriter, r *http.Request) {
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
	var bod requestCreateCheckoutSession
	err = json.NewDecoder(r.Body).Decode(&bod)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	err = tok.Check(bod)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	params := &stripe.CheckoutSessionParams{
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				// Provide the exact Price ID (for example, pr_1234) of the product you want to sell
				Price:              stripe.String("{{PRICE_ID}}"),
				Quantity:           stripe.Int64(1),
				AdjustableQuantity: &stripe.CheckoutSessionLineItemAdjustableQuantityParams{},
			},
		},
		Mode:         stripe.String(string(stripe.CheckoutSessionModePayment)),
		SuccessURL:   stripe.String(bod.SuccessURL),
		CancelURL:    stripe.String(bod.CancelURL),
		AutomaticTax: &stripe.CheckoutSessionAutomaticTaxParams{Enabled: stripe.Bool(true)},
	}

	s, err := session.New(params)

	if err != nil {
		log.Printf("session.New: %v", err)
	}

	http.Redirect(w, r, s.URL, http.StatusSeeOther)
}
