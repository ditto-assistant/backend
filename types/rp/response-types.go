package rp

type BalanceV1 struct {
	Balance  string `json:"balance"`
	USD      string `json:"usd"`
	Images   string `json:"images"`
	Searches string `json:"searches"`
}

func (BalanceV1) Zeroes() BalanceV1 {
	return BalanceV1{
		Balance:  "0",
		Images:   "0",
		Searches: "0",
	}
}
