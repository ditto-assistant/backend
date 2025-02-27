package firestoremem

import (
	"cloud.google.com/go/firestore"
	"github.com/ditto-assistant/backend/pkg/services/filestorage"
)

type Client struct {
	firestore *firestore.Client
	fsClient  *filestorage.Client
}

func NewClient(firestore *firestore.Client, fsClient *filestorage.Client) *Client {
	return &Client{firestore: firestore, fsClient: fsClient}
}

func (cl *Client) conversationsRef(userID string) *firestore.CollectionRef {
	return cl.firestore.Collection("memory").Doc(userID).Collection("conversations")
}
