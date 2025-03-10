package rp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/ditto-assistant/backend/cfg/envs"
	"github.com/ditto-assistant/backend/pkg/services/filestorage"
	"golang.org/x/sync/errgroup"
)

// RespondWithJSON writes a JSON response to the given writer
func RespondWithJSON(w http.ResponseWriter, statusCode int, payload interface{}) {
	response, err := json.Marshal(payload)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "Error marshalling JSON response"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	w.Write(response)
}

// SuccessResponse represents a simple success message response
type SuccessResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// Encryption Response Types

// CreateEncryptedPromptResponse represents the response for creating an encrypted prompt
type CreateEncryptedPromptResponse struct {
	PromptID string `json:"promptId"`
}

// SaveEncryptedResponseResponse represents the response for saving an encrypted response
type SaveEncryptedResponseResponse struct {
	Success bool `json:"success"`
}

// MigrationResponse represents the response for migrating user conversations to encrypted format
type MigrationResponse struct {
	Success             bool     `json:"success"`
	MigratedCount       int      `json:"migratedCount"`
	FailedCount         int      `json:"failedCount,omitempty"`
	FailedConversations []string `json:"failedConversations,omitempty"`
}

type BalanceV1 struct {
	BalanceRaw         int64      `json:"balanceRaw"`
	Balance            string     `json:"balance"`
	USD                string     `json:"usd"`
	Images             string     `json:"images"`
	ImagesRaw          int64      `json:"imagesRaw"`
	Searches           string     `json:"searches"`
	SearchesRaw        int64      `json:"searchesRaw"`
	DropAmountRaw      int64      `json:"dropAmountRaw,omitempty"`
	DropAmount         string     `json:"dropAmount,omitempty"`
	TotalAirdroppedRaw int64      `json:"totalAirdroppedRaw,omitempty"`
	TotalAirdropped    string     `json:"totalAirdropped,omitempty"`
	LastAirdropAt      *time.Time `json:"lastAirdropAt,omitempty"`
}

func (BalanceV1) Zeroes() BalanceV1 {
	return BalanceV1{
		Balance:     "0",
		BalanceRaw:  0,
		Images:      "0",
		ImagesRaw:   0,
		Searches:    "0",
		SearchesRaw: 0,
		USD:         "$0.00",
	}
}

// Memory represents a conversation memory with vector similarity
type Memory struct {
	ID                 string             `json:"id"`
	Score              float32            `json:"score"`
	Prompt             string             `json:"prompt" firestore:"prompt"`
	Response           string             `json:"response" firestore:"response"`
	Timestamp          time.Time          `json:"timestamp" firestore:"timestamp"`
	VectorDistance     float32            `json:"vector_distance" firestore:"vector_distance"`
	EmbeddingPrompt5   firestore.Vector32 `json:"-" firestore:"embedding_prompt_5"`
	EmbeddingResponse5 firestore.Vector32 `json:"-" firestore:"embedding_response_5"`
	Depth              int                `json:"depth" firestore:"-"`
	Children           []Memory           `json:"children,omitempty" firestore:"-"`

	// Encryption fields
	IsEncrypted       bool   `json:"is_encrypted,omitempty" firestore:"is_encrypted"`
	EncryptedPrompt   string `json:"encrypted_prompt,omitempty" firestore:"encrypted_prompt"`
	EncryptedResponse string `json:"encrypted_response,omitempty" firestore:"encrypted_response"`
	EncryptionKeyID   string `json:"encryption_key_id,omitempty" firestore:"encryption_key_id"`
	EncryptionVersion int    `json:"encryption_version,omitempty" firestore:"encryption_version"`
}

// MemoriesV1 represents the response for getting memories
type MemoriesV1 struct {
	Memories []Memory `json:"memories"`
}

type MemoriesV2 struct {
	LongTerm  []Memory `json:"longTerm"`
	ShortTerm []Memory `json:"shortTerm"`
}

func FormatToolsResponse(response *string) {
	switch {
	case strings.Contains(*response, "Script Generated and Downloaded.**"):
		parts := strings.Split(*response, "- Task:")
		if len(parts) > 1 {
			if strings.Contains(parts[0], "HTML") {
				*response = "<HTML_SCRIPT>" + parts[1]
			} else if strings.Contains(parts[0], "OpenSCAD") {
				*response = "<OPENSCAD>" + parts[1]
			}
		}
	case strings.Contains(*response, "Image Task:"):
		parts := strings.Split(*response, "Image Task:")
		if len(parts) > 1 {
			*response = "<IMAGE_GENERATION>" + parts[1]
		}
	case strings.Contains(*response, "Google Search Query:"):
		parts := strings.Split(*response, "Google Search Query:")
		if len(parts) > 1 {
			*response = "<GOOGLE_SEARCH>" + parts[1]
		}
	}
}

func (mem *Memory) FormatResponse() {
	FormatToolsResponse(&mem.Response)
}

func (mem *Memory) StripImages() {
	TrimStuff(&mem.Prompt, "![image](", ")", nil)
	TrimStuff(&mem.Response, "![DittoImage](", ")", nil)
}

func (mem *Memory) PresignImages(ctx context.Context, userID string, cl *filestorage.Client) error {
	group, ctx := errgroup.WithContext(ctx)
	group.Go(func() error {
		return TrimStuff(&mem.Prompt, "![image](", ")", func(url *string) error {
			if !strings.HasPrefix(*url, envs.DITTO_CONTENT_PREFIX) &&
				!strings.HasPrefix(*url, envs.DALL_E_PREFIX) {
				return nil
			}
			presignedURL, err := cl.PresignURL(ctx, userID, *url)
			if err != nil {
				return err
			}
			*url = presignedURL
			return nil
		})
	})
	group.Go(func() error {
		return TrimStuff(&mem.Response, "![DittoImage](", ")", func(url *string) error {
			if !strings.HasPrefix(*url, envs.DITTO_CONTENT_PREFIX) &&
				!strings.HasPrefix(*url, envs.DALL_E_PREFIX) {
				return nil
			}
			presignedURL, err := cl.PresignURL(ctx, userID, *url)
			if err != nil {
				return err
			}
			*url = presignedURL
			return nil
		})
	})
	return group.Wait()
}

func TrimStuff(s *string, prefix, suffix string, replaceFunc func(*string) error) error {
	result := *s
	searchText := *s
	for {
		idx := strings.Index(searchText, prefix)
		if idx == -1 {
			break
		}
		start := idx + len(prefix)
		if start >= len(searchText) {
			break
		}
		afterPrefix := searchText[start:]
		closeIdx := strings.Index(afterPrefix, suffix)
		if closeIdx == -1 {
			break
		}
		resultIdx := strings.Index(result, searchText[idx:start+closeIdx+1])
		if resultIdx == -1 {
			searchText = searchText[start+closeIdx+1:]
			continue
		}
		if replaceFunc == nil {
			result = result[:resultIdx] + result[resultIdx+len(prefix)+len(afterPrefix[:closeIdx])+len(suffix):]
		} else {
			url := afterPrefix[:closeIdx]
			url = strings.TrimSuffix(url, "\n") // Trim any trailing newlines that might have been added
			err := replaceFunc(&url)
			if err != nil {
				return err
			}
			result = result[:resultIdx] + prefix + url + suffix + result[resultIdx+len(prefix)+len(afterPrefix[:closeIdx])+len(suffix):]
		}
		searchText = searchText[start+closeIdx+1:]
	}
	*s = result
	return nil
}

func (mem *Memory) String() string {
	return fmt.Sprintf("**Memory (%s)**\n\n**User:**\n%s\n\n**Ditto:**\n%s\n\n",
		mem.Timestamp.Format("2006-01-02 15:04:05"),
		mem.Prompt,
		mem.Response,
	)
}

func (m MemoriesV2) Bytes() []byte {
	var b bytes.Buffer
	b.WriteString("# Memories\n\n")
	if len(m.LongTerm) > 0 {
		b.WriteString("## Long-Term Memory (Cosine Similarity)\n\n")
		b.WriteString("*Most relevant prompt/response pairs from user's prompt history*\n\n")
		for _, mem := range m.LongTerm {
			writeMemoryWithChildren(&b, &mem, 0)
		}
		b.WriteRune('\n')
	}
	if len(m.ShortTerm) > 0 {
		b.WriteString("## Short-Term Memory (Recent)\n\n")
		b.WriteString("*Most recent prompt/response pairs*\n\n")
		for _, mem := range m.ShortTerm {
			b.WriteString(mem.String())
		}
		b.WriteRune('\n')
	}
	return b.Bytes()
}

// writeMemoryWithChildren recursively writes a memory and its children using markdown.
func writeMemoryWithChildren(b *bytes.Buffer, mem *Memory, indent int) {
	headingLevel := min(3+mem.Depth, 6)
	headingMarks := strings.Repeat("#", headingLevel)
	fmt.Fprintf(b, "%s Memory Layer (Depth %d)\n\n", headingMarks, mem.Depth)
	b.WriteString(mem.String())
	if len(mem.Children) > 0 {
		fmt.Fprintf(b, "%s Related Memories\n\n", strings.Repeat("#", headingLevel+1))
		for _, child := range mem.Children {
			writeMemoryWithChildren(b, &child, indent+1)
		}
		b.WriteRune('\n')
	}
}
