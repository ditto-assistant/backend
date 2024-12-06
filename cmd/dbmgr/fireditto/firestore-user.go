package fireditto

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	firebase "firebase.google.com/go/v4"
)

func (f *Command) PrintUser(ctx context.Context) error {
	email, userID := f.Email, f.UID
	app, err := firebase.NewApp(ctx, nil)
	if err != nil {
		return fmt.Errorf("error creating firebase app: %w", err)
	}
	// If email is provided, get the user ID first
	if email != "" {
		auth, err := app.Auth(ctx)
		if err != nil {
			return fmt.Errorf("error getting auth client: %w", err)
		}
		user, err := auth.GetUserByEmail(ctx, email)
		if err != nil {
			return fmt.Errorf("error getting user by email: %w", err)
		}
		userID = user.UID
		slog.Info("User ID", "userID", userID)
	}
	fs, err := app.Firestore(ctx)
	if err != nil {
		return fmt.Errorf("error getting firestore client: %w", err)
	}
	convDocs, err := fs.
		Collection("memory").
		Doc(userID).
		Collection("conversations").
		OrderBy("timestamp", f.Order()).
		Limit(f.Limit).
		Offset(f.Offset).
		Documents(ctx).
		GetAll()
	if err != nil {
		return fmt.Errorf("error getting conversations from Firestore: %w", err)
	}
	slog.Info("User conversations", "userID", userID, "count", len(convDocs))
	for _, doc := range convDocs {
		var conv struct {
			Prompt    string    `firestore:"prompt"`
			Response  string    `firestore:"response"`
			Timestamp time.Time `firestore:"timestamp"`
		}
		if err := doc.DataTo(&conv); err != nil {
			slog.Error("Error unmarshaling conversation", "error", err, "docID", doc.Ref.ID)
			continue
		}
		promptLinks := parseImageLinks(conv.Prompt)
		respLinks := parseImageLinks(conv.Response)
		if len(conv.Prompt) > 100 {
			conv.Prompt = conv.Prompt[:100] + "..."
		}
		if len(conv.Response) > 100 {
			conv.Response = conv.Response[:100] + "..."
		}
		fmt.Printf("%s\n\033[32mUser: %s\033[0m\n", conv.Timestamp.Format(time.DateTime), conv.Prompt)
		if len(promptLinks) > 0 {
			fmt.Printf("\033[32mUser Images: %v\033[0m\n", promptLinks)
		}
		fmt.Printf("\033[36mDitto: %s\033[0m\n", conv.Response)
		if len(respLinks) > 0 {
			fmt.Printf("\033[36mDitto Images: %v\033[0m\n", respLinks)
		}
		fmt.Println()
	}
	return nil
}

// var reImageLinks = regexp.MustCompile(`!\[(?:image|DittoImage)\]\((.*?)\)`)

func parseImageLinks(text string) []string {
	const prefixImageAttachment = "![image]("
	const prefixDittoImageAttachment = "![DittoImage]("
	const suffixImageAttachment = ")"
	var links []string

	// Handle ![image]() links
	remaining := text
	for {
		imgIdx := strings.Index(remaining, prefixImageAttachment)
		if imgIdx == -1 {
			break
		}
		start := imgIdx + len(prefixImageAttachment)
		afterPrefix := remaining[start:]
		closeIdx := strings.Index(afterPrefix, suffixImageAttachment)
		if closeIdx == -1 {
			break
		}
		links = append(links, afterPrefix[:closeIdx])
		remaining = afterPrefix[closeIdx:]
	}

	// Handle ![DittoImage]() links
	remaining = text
	for {
		imgIdx := strings.Index(remaining, prefixDittoImageAttachment)
		if imgIdx == -1 {
			break
		}
		start := imgIdx + len(prefixDittoImageAttachment)
		afterPrefix := remaining[start:]
		closeIdx := strings.Index(afterPrefix, suffixImageAttachment)
		if closeIdx == -1 {
			break
		}
		links = append(links, afterPrefix[:closeIdx])
		remaining = afterPrefix[closeIdx:]
	}

	return links
}
