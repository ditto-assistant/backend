package rp

type BalanceV1 struct {
	BalanceRaw  int64  `json:"balanceRaw"`
	Balance     string `json:"balance"`
	USD         string `json:"usd"`
	Images      string `json:"images"`
	ImagesRaw   int64  `json:"imagesRaw"`
	Searches    string `json:"searches"`
	SearchesRaw int64  `json:"searchesRaw"`
}

func (BalanceV1) Zeroes() BalanceV1 {
	return BalanceV1{
		Balance:  "0",
		Images:   "0",
		Searches: "0",
	}
}
