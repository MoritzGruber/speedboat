package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"

	"github.com/MoritzGruber/speedboat.git/pkg/api"
	"github.com/MoritzGruber/speedboat.git/pkg/connector"
	"github.com/MoritzGruber/speedboat.git/pkg/engine"
	"github.com/MoritzGruber/speedboat.git/pkg/store"
)

func main() {
	// Load configuration from environment variables
	jiraBaseURL := os.Getenv("JIRA_BASE_URL")
	personalAccessToken := os.Getenv("JIRA_TOKEN")

	// Basic validation to ensure environment variables are set
	if jiraBaseURL == "" || personalAccessToken == "" {
		slog.Error("Missing required environment variables (JIRA_BASE_URL, JIRA_TOKEN)")
		os.Exit(1)
	}

	// Initialize the Jira client using the Connetor interface to access Update/Get
	var client engine.Connetor = &connector.JiraCollector{
		BaseURL: jiraBaseURL,
		Token:   personalAccessToken,
		Client:  &http.Client{},
	}

	ticketKey := "STACKITPMO-7" // You can also use the ID "5093630" here

	// 1. Prepare the payload
	// Note: Jira uses "summary" for the ticket title.
	updateData := engine.Issue{
		Fields: map[string]interface{}{
			"summary":     "foobar",
			"description": "<p> fizz <b>buzz</b></p>",
		},
	}

	slog.Info("Updating ticket...", "ticket", ticketKey)

	// 2. Update the ticket
	_, err := client.Update(ticketKey, updateData)
	if err != nil {
		slog.Error("Failed to update ticket", "error", err)
		os.Exit(1)
	}

	// 3. Fetch the updated ticket using Get
	updatedIssue, err := client.Get(ticketKey)
	if err != nil {
		slog.Error("Failed to get updated ticket", "error", err)
		os.Exit(1)
	}

	// 4. Marshal the response to JSON to log the full response body clearly
	fullJSON, err := json.MarshalIndent(updatedIssue, "", "  ")
	if err != nil {
		slog.Error("Failed to marshal issue to JSON", "error", err)
		os.Exit(1)
	}

	// 5. Log using slog
	slog.Info("Successfully fetched updated ticket",
		"key", updatedIssue.Key,
		"jsonResponse", string(fullJSON),
	)

	// Initialize the file-based Automerge store
	store, err := store.NewFileStore("./data/storage")
	if err != nil {
		slog.Error("Failed to initialize store", "error", err)
		return
	}

	// Create a Go 1.22 compliant Multiplexer and register the API routes
	mux := http.NewServeMux()
	server := api.NewServer(store)
	server.RegisterRoutes(mux)

	slog.Info("Starting server on :8080...")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		slog.Error("Server failed", "error", err)
	}
}
