set windows-shell := ["powershell.exe", "-NoLogo", "-Command"]

set dotenv-load
run:
	genkit start
go:
	go run main.go

deploy:
	gcloud run deploy --port 3400 \
		--set-env-vars GCLOUD_PROJECT=ditto-app-dev \
		--set-env-vars GCLOUD_LOCATION=us-central1 \
		--source . \

kill:
	lsof -i :3400 | grep LISTEN | awk '{print $2}' | xargs kill

db *ARGS:
	go run cmd/dbmgr/main.go {{ARGS}}

search *ARGS:
	go run cmd/dbmgr/main.go search {{ARGS}}

build:
	docker build -t ditto-backend .