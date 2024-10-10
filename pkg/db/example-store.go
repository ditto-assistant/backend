package db

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"log"
)

type Embedding []float32

func (e Embedding) Binary() []byte {
	var buf bytes.Buffer
	buf.Grow(len(e) * 4)
	for _, v := range e {
		err := binary.Write(&buf, binary.LittleEndian, v)
		if err != nil {
			log.Fatalf("error converting float32 to bytes: %s", err)
		}
	}
	return buf.Bytes()
}

type Tool struct {
	Name           string  `json:"name"`
	Description    string  `json:"description"`
	Version        string  `json:"version"`
	CostPerCall    float64 `json:"cost_per_call"`
	CostMultiplier float64 `json:"cost_multiplier"`
	BaseTokens     int     `json:"base_tokens"`
}

type Example struct {
	Prompt       string    `json:"prompt"`
	Response     string    `json:"response"`
	EmPrompt     Embedding `json:"-" db:"type:blob"`
	EmPromptResp Embedding `json:"-" db:"type:blob"`
}

const querySearch = "SELECT prompt, response FROM examples ORDER BY vector_distance_cos(em_prompt, ?) LIMIT ?"

type opt struct {
	searchK int
}
type SearchOption func(*opt)

func WithK(k int) SearchOption {
	return func(o *opt) {
		o.searchK = k
	}
}

func SearchExamples(ctx context.Context, em Embedding, opts ...SearchOption) ([]Example, error) {
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

	examples := make([]Example, 0, opt.searchK)
	for rows.Next() {
		var example Example
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
