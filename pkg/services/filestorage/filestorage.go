package filestorage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/ditto-assistant/backend/cfg/envs"
	"github.com/ditto-assistant/backend/cfg/secr"
	"github.com/omniaura/mapcache"
)

const presignTTL = 24 * time.Hour

type Client struct {
	S3            *s3.S3
	urlCache      *mapcache.MapCache[string, string]
	contentBucket *string
}

func NewClient(ctx context.Context) (*Client, error) {
	urlCache, err := mapcache.New[string, string](
		mapcache.WithTTL(presignTTL/2),
		mapcache.WithCleanup(ctx, presignTTL),
	)
	if err != nil {
		return nil, err
	}
	s3Config := &aws.Config{
		Credentials: credentials.NewStaticCredentials(envs.BACKBLAZE_KEY_ID, secr.BACKBLAZE_API_KEY.String(), ""),
		Region:      aws.String(envs.DITTO_CONTENT_REGION),
		Endpoint:    aws.String(envs.DITTO_CONTENT_ENDPOINT),
	}
	mySession, err := session.NewSession(s3Config)
	if err != nil {
		return nil, err
	}
	s3 := s3.New(mySession)
	cl := &Client{
		S3:            s3,
		urlCache:      urlCache,
		contentBucket: aws.String(envs.DITTO_CONTENT_BUCKET),
	}
	return cl, nil
}

func (cl *Client) PresignURL(ctx context.Context, userID, url string) (string, error) {
	return cl.urlCache.Get(url, func() (string, error) {
		urlParts := strings.Split(url, "?")
		if len(urlParts) == 0 {
			return "", fmt.Errorf("failed to get filename from URL: %s", url)
		}
		filename := strings.TrimPrefix(urlParts[0], envs.DITTO_CONTENT_PREFIX)
		filename = strings.TrimPrefix(filename, envs.DALL_E_PREFIX)
		filename = strings.TrimPrefix(filename, userID+"/")
		filename = strings.TrimPrefix(filename, "generated-images/") // Remove any existing folder prefix
		key := fmt.Sprintf("%s/generated-images/%s", userID, filename)
		input := &s3.GetObjectInput{
			Bucket: cl.contentBucket,
			Key:    aws.String(key),
		}
		objReq, _ := cl.S3.GetObjectRequest(input)
		return objReq.Presign(presignTTL)
	})
}
