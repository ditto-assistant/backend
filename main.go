package main

import (
	"context"
	"errors"
	"fmt"
	"log"

	// Import the Genkit core libraries.
	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"

	// Import the Google Cloud Vertex AI plugin.
	"github.com/firebase/genkit/go/plugins/vertexai"
)

func main() {
	ctx := context.Background()

	// Initialize the Vertex AI plugin. When you pass an empty string for the
	// projectID parameter, the Vertex AI plugin will use the value from the
	// GCLOUD_PROJECT environment variable. When you pass an empty string for
	// the location parameter, the plugin uses the default value, us-central1.
	if err := vertexai.Init(ctx, nil); err != nil {
		log.Fatal(err)
	}

	// Define a simple flow that prompts an LLM to generate menu suggestions.
	genkit.DefineFlow("menu", func(ctx context.Context, input string) (string, error) {
		// The Vertex AI API provides access to several generative models. Here,
		// we specify gemini-1.5-flash.
		m := vertexai.Model("gemini-1.5-flash")
		if m == nil {
			return "", errors.New("menuSuggestionFlow: failed to find model")
		}

		// Construct a request and send it to the model API.
		resp, err := m.Generate(ctx,
			ai.NewGenerateRequest(
				&ai.GenerationCommonConfig{Temperature: 1},
				ai.NewUserTextMessage(fmt.Sprintf(`Suggest an item for the menu of a %s themed restaurant`, input))),
			nil)
		if err != nil {
			return "", err
		}

		// Handle the response from the model API. In this sample, we just
		// convert it to a string, but more complicated flows might coerce the
		// response into structured output or chain the response into another
		// LLM call, etc.
		text := resp.Text()
		return text, nil
	})

	// Initialize Genkit and start a flow server. This call must come last,
	// after all of your plug-in configuration and flow definitions. When you
	// pass a nil configuration to Init, Genkit starts a local flow server,
	// which you can interact with using the developer UI.
	if err := genkit.Init(ctx,
		&genkit.Options{FlowAddr: ":3400"},
	); err != nil {
		log.Fatal(err)
	}
}
