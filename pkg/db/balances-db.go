package db

import "context"

func GetDittoTokensPerDollar(ctx context.Context) (int64, error) {
	var count int64
	err := D.QueryRowContext(ctx, "SELECT count FROM tokens_per_dollar WHERE name = 'ditto'").Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}
