package llm

import (
	"bytes"
	"encoding/binary"
	"log"

	"github.com/firebase/genkit/go/ai"
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
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Version     string      `json:"version"`
	ServiceName ServiceName `json:"service_name"`
}

type Example struct {
	Prompt       string    `json:"prompt"`
	Response     string    `json:"response"`
	EmPrompt     Embedding `json:"-" db:"type:blob"`
	EmPromptResp Embedding `json:"-" db:"type:blob"`
}

type GenkitMetadata struct {
	LatencyMs float64             `json:"latencyMs"`
	Usage     *ai.GenerationUsage `json:"usage"`
}
