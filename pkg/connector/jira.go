package connector

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/MoritzGruber/jaj.git/pkg/engine"
)

// SearchResults represents the response from the /rest/api/2/search endpoint.
type SearchResults struct {
	StartAt    int            `json:"startAt"`
	MaxResults int            `json:"maxResults"`
	Total      int            `json:"total"`
	Issues     []engine.Issue `json:"issues"`
}

// JiraCollector implements the Collector interface for a private Jira instance.
type JiraCollector struct {
	BaseURL string
	Token   string
	JQL     string
	Client  *http.Client
}

// Get fetches a single ticket by its ID or Key.
func (j *JiraCollector) Get(id string) (engine.Issue, error) {
	reqURL := fmt.Sprintf("%s/rest/api/2/issue/%s", j.BaseURL, url.PathEscape(id))

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return engine.Issue{}, fmt.Errorf("failed to create GET request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer "+j.Token)
	req.Header.Add("Accept", "application/json")

	resp, err := j.Client.Do(req)
	if err != nil {
		return engine.Issue{}, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return engine.Issue{}, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var issue engine.Issue
	if err := json.NewDecoder(resp.Body).Decode(&issue); err != nil {
		return engine.Issue{}, fmt.Errorf("failed to decode JSON response: %w", err)
	}

	return issue, nil
}

// Update modifies a single ticket by its ID or Key.
func (j *JiraCollector) Update(id string, issue engine.Issue) (engine.Issue, error) {
	reqURL := fmt.Sprintf("%s/rest/api/2/issue/%s", j.BaseURL, url.PathEscape(id))

	// Jira expects the payload to have a top-level "fields" object for updates
	payload := map[string]interface{}{
		"fields": issue.Fields,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return engine.Issue{}, fmt.Errorf("failed to marshal update payload: %w", err)
	}

	req, err := http.NewRequest("PUT", reqURL, bytes.NewBuffer(body))
	if err != nil {
		return engine.Issue{}, fmt.Errorf("failed to create PUT request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer "+j.Token)
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-Type", "application/json")

	resp, err := j.Client.Do(req)
	if err != nil {
		return engine.Issue{}, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	// Jira returns 204 No Content on a successful PUT update
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return engine.Issue{}, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Since PUT usually returns 204 No Content, we fetch the updated issue to return the exact new state
	return j.Get(id)
}

// List fetches all tickets matching the Collector's JQL from the Jira instance.
func (j *JiraCollector) List() ([]engine.Issue, error) {
	var allIssues []engine.Issue
	startAt := 0
	maxResults := 50 // Standard pagination size

	for {
		// Construct the request URL with pagination and JQL
		reqURL, err := url.Parse(fmt.Sprintf("%s/rest/api/2/search", j.BaseURL))
		if err != nil {
			return nil, fmt.Errorf("failed to parse URL: %w", err)
		}

		q := reqURL.Query()
		q.Set("jql", j.JQL)
		q.Set("startAt", fmt.Sprintf("%d", startAt))
		q.Set("maxResults", fmt.Sprintf("%d", maxResults))
		q.Set("fields", "summary,description,status,issuetype,assignee")
		reqURL.RawQuery = q.Encode()

		// Create the HTTP request
		req, err := http.NewRequest("GET", reqURL.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		// Use Bearer token authentication for Personal Access Tokens (Server/DC)
		req.Header.Add("Authorization", "Bearer "+j.Token)
		req.Header.Add("Accept", "application/json")

		// Execute the request
		resp, err := j.Client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("HTTP request failed: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(bodyBytes))
		}

		// Decode the JSON response
		var searchRes SearchResults
		if err := json.NewDecoder(resp.Body).Decode(&searchRes); err != nil {
			return nil, fmt.Errorf("failed to decode JSON response: %w", err)
		}

		allIssues = append(allIssues, searchRes.Issues...)

		// Check if we have fetched all pages
		if startAt+len(searchRes.Issues) >= searchRes.Total || len(searchRes.Issues) == 0 {
			break
		}

		if len(allIssues) >= 100 {
			break
		}

		// Move to the next page
		startAt += maxResults
	}

	return allIssues, nil
}
