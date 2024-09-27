set dotenv-load
run:
	genkit start
go:
	go run main.go

deploy:
	gcloud run deploy --port 3400 \
		--set-env-vars GCLOUD_PROJECT=ditto-app-dev \
		--set-env-vars GCLOUD_LOCATION=us-central1 \
		--set-env-vars GCLOUD_REGION=us-central1 \
		--source . \