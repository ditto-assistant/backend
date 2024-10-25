PROJECT_ID="ditto-app-dev"
MODEL="mistral-nemo"
MODEL_VERSION="2407"
REGION="europe-west4" #or “us-central1”


url="https://$REGION-aiplatform.googleapis.com/v1/projects/$PROJECT_ID/locations/$REGION/publishers/mistralai/models/$MODEL@$MODEL_VERSION:streamRawPredict"


curl \
  -X POST \
  -H "Authorization: Bearer $(gcloud auth print-access-token)" \
  -H "Content-Type: application/json" \
  $url \
  --data '{
  "model": "'"$MODEL"'",
  "temperature": 0,
  "messages": [
    {
      "role": "user",
      "content": "What is the best French cheese?"
    }
  ]
}'