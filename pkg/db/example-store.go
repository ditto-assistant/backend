package db

import (
	"context"
	"fmt"

	"github.com/ditto-assistant/backend/pkg/llm"
)

const querySearch = "SELECT prompt, response FROM examples ORDER BY vector_distance_cos(em_prompt, ?) LIMIT ?"

type opt struct {
	searchK int
}

type SearchOption func(*opt)

func WithK(k int) SearchOption { return func(o *opt) { o.searchK = k } }

func SearchExamples(ctx context.Context, em llm.Embedding, opts ...SearchOption) ([]llm.Example, error) {
	opt := &opt{
		searchK: 5,
	}
	for _, o := range opts {
		o(opt)
	}
	emBytes := em.Binary()
	rows, err := D.QueryContext(ctx, querySearch, emBytes, opt.searchK)
	if err != nil {
		return nil, fmt.Errorf("error querying database: %w", err)
	}
	defer rows.Close()

	examples := make([]llm.Example, 0, opt.searchK)
	for rows.Next() {
		var example llm.Example
		err = rows.Scan(&example.Prompt, &example.Response)
		if err != nil {
			return nil, fmt.Errorf("error scanning row: %w", err)
		}
		examples = append(examples, example)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over rows: %w", err)
	}

	return examples, nil
}
