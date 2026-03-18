package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/MoritzGruber/speedboat.git/pkg/api"
	"github.com/MoritzGruber/speedboat.git/pkg/engine"
	"github.com/MoritzGruber/speedboat.git/pkg/store"
	"github.com/automerge/automerge-go"
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

	// 1. Initialize the file-based Automerge store BEFORE fetching data
	// (Renamed variable to fileStore to prevent shadowing the store package)
	fileStore, err := store.NewFileStore("./data/storage")
	if err != nil {
		slog.Error("Failed to initialize store", "error", err)
		return
	}

	// 2. Populate the filestore with an initial list request
	projectKey := "STACKITPMO"
	slog.Info("Fetching initial issues from Jira...", "project", projectKey)

	// Build the search URL. Adjust maxResults if you expect more than 50 initial tickets.
	searchURL := fmt.Sprintf("%s/rest/api/2/search?jql=project=%s&maxResults=50", jiraBaseURL, projectKey)

	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		slog.Error("Failed to create request", "error", err)
		os.Exit(1)
	}
	req.Header.Set("Authorization", "Bearer "+personalAccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Error("Failed to fetch issues from Jira", "error", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Error("Jira API returned non-200 status", "status", resp.StatusCode)
		os.Exit(1)
	}

	var searchResult struct {
		Issues []engine.Issue `json:"issues"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&searchResult); err != nil {
		slog.Error("Failed to decode Jira response", "error", err)
		os.Exit(1)
	}

	slog.Info("Successfully fetched issues from Jira", "count", len(searchResult.Issues))

	// 3. Convert Jira issues to Automerge documents
	docsToSave := make(map[string]*automerge.Doc)

	for _, issue := range searchResult.Issues {
		doc := automerge.New()

		_ = doc.Path("id").Set(issue.ID)
		_ = doc.Path("key").Set(issue.Key)

		// Explicitly create the Fields map in the CRDT
		fields := doc.Path("fields")
		if issue.Fields != nil {
			for k, v := range issue.Fields {
				// This ensures every key from Jira is mirrored in the Automerge Map
				_ = fields.Path(k).Set(v)
			}

			// Ensure "status" and "priority" exist even if Jira doesn't provide them
			// to prevent the frontend from breaking.
			if _, ok := issue.Fields["status"]; !ok {
				_ = fields.Path("status").Set("open")
			}
			if _, ok := issue.Fields["priority"]; !ok {
				_ = fields.Path("priority").Set("medium")
			}
		}

		docsToSave[issue.ID] = doc // Use ID as the filename to match API expectations
	}

	// 4. BatchUpsert the documents into the local FileStore
	if err := fileStore.BatchUpsert(docsToSave); err != nil {
		slog.Error("Failed to batch upsert issues to store", "error", err)
	} else {
		slog.Info("Filestore population complete.", "upserted", len(docsToSave))
	}
	// 4. Start the HTTP Server
	mux := http.NewServeMux()
	server := api.NewServer(fileStore)
	server.RegisterRoutes(mux)
	handlerWithCORS := api.WithCORS(mux)

	slog.Info("Starting server on :8080...")
	if err := http.ListenAndServe(":8080", handlerWithCORS); err != nil {
		slog.Error("Server failed", "error", err)
	}
}
