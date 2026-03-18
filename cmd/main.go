package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/MoritzGruber/speedboat.git/pkg/api"
	"github.com/MoritzGruber/speedboat.git/pkg/connector"
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
	searchURL := fmt.Sprintf("%s/rest/api/2/search?jql=project=%s&maxResults=50&fields=summary,description,status,priority", jiraBaseURL, projectKey)
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

		_ = doc.Path("ID").Set(issue.ID)
		_ = doc.Path("Key").Set(issue.Key)

		fields := doc.Path("Fields")
		if issue.Fields != nil {
			// Extract title from Jira's "summary"
			title := ""
			if summary, ok := issue.Fields["summary"].(string); ok {
				title = summary
			}

			// Extract description
			desc := ""
			if description, ok := issue.Fields["description"].(string); ok {
				desc = description
			}

			// Flatten Jira's nested status object into a simple string
			statusName := "open" // default fallback
			if statusObj, ok := issue.Fields["status"].(map[string]interface{}); ok {
				if name, ok := statusObj["name"].(string); ok {
					statusName = name
				}
			}

			// Flatten Jira's nested priority object into a simple string
			priorityName := "medium" // default fallback
			if prioObj, ok := issue.Fields["priority"].(map[string]interface{}); ok {
				if name, ok := prioObj["name"].(string); ok {
					priorityName = name
				}
			}

			// Set the clean, flattened scalar values into the Automerge document
			_ = fields.Path("title").Set(title)
			_ = fields.Path("summary").Set(title) // Kept for safety if UI expects summary
			_ = fields.Path("description").Set(desc)
			_ = fields.Path("status").Set(statusName)
			_ = fields.Path("priority").Set(priorityName)
		}

		docsToSave[issue.ID] = doc
	}

	// 4. BatchUpsert the documents into the local FileStore
	if err := fileStore.BatchUpsert(docsToSave); err != nil {
		slog.Error("Failed to batch upsert issues to store", "error", err)
	} else {
		slog.Info("Filestore population complete.", "upserted", len(docsToSave))
	}
	// 4. Start the HTTP Server
	mux := http.NewServeMux()
	jiraConn := &connector.JiraCollector{
		BaseURL: jiraBaseURL,
		Token:   personalAccessToken,
		Client:  http.DefaultClient,
	}

	// Pass it to the server
	server := api.NewServer(fileStore, jiraConn)
	server.RegisterRoutes(mux)

	go startPolling(fileStore, jiraConn)

	handlerWithCORS := api.WithCORS(mux)

	slog.Info("Starting server on :8080...")
	if err := http.ListenAndServe(":8080", handlerWithCORS); err != nil {
		slog.Error("Server failed", "error", err)
	}
}

func startPolling(fileStore *store.FileStore, jiraConn *connector.JiraCollector) {
	// Setup the 500ms ticker
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	// Optional but highly recommended: Modify the JQL to only fetch recently updated issues
	// jiraConn.JQL = fmt.Sprintf("project=%s AND updated >= -1m", projectKey)

	for range ticker.C {
		issues, err := jiraConn.List()
		if err != nil {
			slog.Error("Polling Jira failed", "error", err)
			continue
		}

		for _, issue := range issues {
			// 1. Try to load the existing document
			doc, err := fileStore.Load(issue.ID)
			isNew := false

			if err != nil {
				// 2. If it doesn't exist locally, create a new one
				doc = automerge.New()
				_ = doc.Path("id").Set(issue.ID)
				_ = doc.Path("key").Set(issue.Key)
				isNew = true
			}

			// 3. Sync Jira fields into the Automerge document
			fieldsPath := doc.Path("Fields")
			if issue.Fields != nil {
				if summary, ok := issue.Fields["summary"].(string); ok {
					_ = fieldsPath.Path("title").Set(summary)
					_ = fieldsPath.Path("summary").Set(summary)
				}
				if description, ok := issue.Fields["description"].(string); ok {
					_ = fieldsPath.Path("description").Set(description)
				}
				// You can also sync status and priority here if needed
			}

			// 4. Save the updated or new document back to the FileStore
			if err := fileStore.Save(issue.ID, doc); err != nil {
				slog.Error("Failed to save polled issue to store", "id", issue.ID, "error", err)
			} else if isNew {
				slog.Info("Polled and created new issue locally", "key", issue.Key)
			}
		}
	}
}
