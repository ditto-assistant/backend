package api

import (
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/ditto-assistant/backend/pkg/service"
	"github.com/omniaura/mapcache"
)

type Service struct {
	sc       service.Context
	s3       *s3.S3
	urlCache *mapcache.MapCache[string, string]
}

func NewService(sc service.Context) *Service {
	return &Service{
		sc: sc,
	}
}
