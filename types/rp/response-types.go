package rp

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/ditto-assistant/backend/cfg/envs"
	"github.com/ditto-assistant/backend/pkg/services/filestorage"
	"golang.org/x/sync/errgroup"
)

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
	ID              string             `json:"id"`
	Score           float32            `json:"score"`
	Prompt          string             `json:"prompt" firestore:"prompt"`
	Response        string             `json:"response" firestore:"response"`
	Timestamp       time.Time          `json:"timestamp" firestore:"timestamp"`
	VectorDistance  float32            `json:"vector_distance" firestore:"vector_distance"`
	EmbeddingVector firestore.Vector32 `json:"-" firestore:"embedding_vector"`
	Depth           int                `json:"depth" firestore:"-"`
	Children        []Memory           `json:"children,omitempty" firestore:"-"`
}

// MemoriesV1 represents the response for getting memories
type MemoriesV1 struct {
	Memories []Memory `json:"memories"`
}

type MemoriesV2 struct {
	LongTerm  []Memory `json:"longTerm"`
	ShortTerm []Memory `json:"shortTerm"`
}

func (mem *Memory) FormatResponse() {
	switch {
	case strings.Contains(mem.Response, "Script Generated and Downloaded.**"):
		parts := strings.Split(mem.Response, "- Task:")
		if len(parts) > 1 {
			if strings.Contains(parts[0], "HTML") {
				mem.Response = "<HTML_SCRIPT>" + parts[1]
			} else if strings.Contains(parts[0], "OpenSCAD") {
				mem.Response = "<OPENSCAD>" + parts[1]
			}
		}
	case strings.Contains(mem.Response, "Image Task:"):
		parts := strings.Split(mem.Response, "Image Task:")
		if len(parts) > 1 {
			mem.Response = "<IMAGE_GENERATION>" + parts[1]
		}
	case strings.Contains(mem.Response, "Google Search Query:"):
		parts := strings.Split(mem.Response, "Google Search Query:")
		if len(parts) > 1 {
			mem.Response = "<GOOGLE_SEARCH>" + parts[1]
		}
	case strings.Contains(mem.Response, "Home Assistant Task:"):
		parts := strings.Split(mem.Response, "Home Assistant Task:")
		if len(parts) > 1 {
			cleaned := strings.TrimSpace(strings.ReplaceAll(
				strings.ReplaceAll(parts[1], "Task completed successfully.", ""),
				"Task failed.", "",
			))
			mem.Response = "<GOOGLE_HOME> " + cleaned
		}
	}
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
	return fmt.Sprintf("User (%s): %s\nDitto: %s\n",
		mem.Timestamp.Format("2006-01-02 15:04:05"),
		mem.Prompt,
		mem.Response,
	)
}

func (m MemoriesV2) Bytes() []byte {
	var b bytes.Buffer
	if len(m.LongTerm) > 0 {
		b.WriteString("## Long Term Memory\n")
		b.WriteString("- Most relevant prompt/response pairs from the user's prompt history are indexed using cosine similarity and are shown below.\n")
		b.WriteString("<LongTermMemory>\n")

		// Write root memories and their children recursively
		for _, mem := range m.LongTerm {
			writeMemoryWithChildren(&b, &mem, 1)
		}

		b.WriteString("</LongTermMemory>\n\n")
	}
	if len(m.ShortTerm) > 0 {
		b.WriteString("## Short Term Memory\n")
		b.WriteString("- Most recent prompt/response pairs are shown below.\n")
		b.WriteString("<ShortTermMemory>\n")
		for _, mem := range m.ShortTerm {
			b.WriteString(mem.String())
		}
		b.WriteString("</ShortTermMemory>\n\n")
	}
	return b.Bytes()
}

// writeMemoryWithChildren recursively writes a memory and its children with proper indentation
func writeMemoryWithChildren(b *bytes.Buffer, mem *Memory, indent int) {
	indentStr := strings.Repeat("  ", indent)

	// Write the memory layer opening tag
	b.WriteString(fmt.Sprintf("%s<MemoryLayer depth=\"%d\">\n", indentStr, mem.Depth))

	// Write the current memory
	b.WriteString(indentStr + "  " + mem.String())

	// Write children if any exist
	if len(mem.Children) > 0 {
		b.WriteString(fmt.Sprintf("%s  <RelatedMemories>\n", indentStr))
		for _, child := range mem.Children {
			writeMemoryWithChildren(b, &child, indent+2)
		}
		b.WriteString(fmt.Sprintf("%s  </RelatedMemories>\n", indentStr))
	}

	// Write the memory layer closing tag
	b.WriteString(fmt.Sprintf("%s</MemoryLayer>\n", indentStr))
}
