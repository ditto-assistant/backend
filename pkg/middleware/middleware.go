package middleware

import "github.com/rs/cors"

func NewCors() *cors.Cors {
	return cors.New(cors.Options{
		AllowedOrigins: []string{
			"http://localhost:3000",
			"http://localhost:4173",
			"https://assistant.heyditto.ai",
			"https://ditto-app-dev.web.app",
			"https://ditto-app-dev-*.web.app",
		},
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"*"}, // Allow all headers
		MaxAge:         86400,         // 24 hours
	})
}
