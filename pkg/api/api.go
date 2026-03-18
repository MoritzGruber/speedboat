package api

import (
	"encoding/json"
	"net/http"

	"github.com/MoritzGruber/speedboat.git/pkg/connector"
	"github.com/MoritzGruber/speedboat.git/pkg/engine"
	"github.com/MoritzGruber/speedboat.git/pkg/store"
	"github.com/automerge/automerge-go"
)

type Server struct {
	store     *store.FileStore
	connector *connector.JiraCollector // Inject the connector
}

// Update the constructor
func NewServer(store *store.FileStore, conn *connector.JiraCollector) *Server {
	return &Server{store: store, connector: conn}
}

// RegisterRoutes registers the API routes to a standard mux (Go 1.22+ recommended).
// RegisterRoutes registers the API routes to a standard mux (Go 1.22+ recommended).
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /issues", s.ListIssues)
	mux.HandleFunc("POST /issues", s.CreateIssue)
	mux.HandleFunc("GET /issues/{id}", s.GetIssue)
	mux.HandleFunc("PATCH /issues/{id}", s.UpdateIssue)
	mux.HandleFunc("DELETE /issues/{id}", s.DeleteIssue)
}

func (s *Server) ListIssues(w http.ResponseWriter, r *http.Request) {
	docs, err := s.store.ListAll()
	if err != nil {
		http.Error(w, "Failed to list issues", http.StatusInternalServerError)
		return
	}

	var issues []engine.Issue
	for _, doc := range docs {
		issue, err := automerge.As[*engine.Issue](doc.Root())
		if err == nil && issue != nil {
			issues = append(issues, *issue)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(issues)
}

func (s *Server) CreateIssue(w http.ResponseWriter, r *http.Request) {
	var issue engine.Issue
	if err := json.NewDecoder(r.Body).Decode(&issue); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	if issue.ID == "" {
		http.Error(w, "Issue ID is required", http.StatusBadRequest)
		return
	}

	doc := automerge.New()
	_ = doc.Path("id").Set(issue.ID)
	_ = doc.Path("key").Set(issue.Key)

	if issue.Fields != nil {
		fPath := doc.Path("Fields")
		for k, v := range issue.Fields {
			_ = fPath.Path(k).Set(v)
		}
	}

	if err := s.store.Save(issue.ID, doc); err != nil {
		http.Error(w, "Failed to save issue", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(issue)
}

func (s *Server) GetIssue(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	doc, err := s.store.Load(id)
	if err != nil {
		http.Error(w, "Issue not found", http.StatusNotFound)
		return
	}

	issue, err := automerge.As[*engine.Issue](doc.Root())
	if err != nil {
		http.Error(w, "Failed to parse document", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(issue)
}

func (s *Server) UpdateIssue(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	// 1. Load the existing CRDT Document
	doc, err := s.store.Load(id)
	if err != nil {
		http.Error(w, "Issue not found", http.StatusNotFound)
		return
	}

	// 2. Parse incoming patch
	var updateData engine.Issue
	if err := json.NewDecoder(r.Body).Decode(&updateData); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	// NEW: Prepare a map for the Jira update
	jiraFields := make(map[string]interface{})

	// 3. Apply updates specifically to the Automerge document
	if updateData.Key != "" {
		_ = doc.Path("Key").Set(updateData.Key)
	}

	// Patch nested dynamic fields gracefully
	if updateData.Fields != nil {
		fieldsPath := doc.Path("Fields")
		for k, v := range updateData.Fields {
			_ = fieldsPath.Path(k).Set(v)

			// NEW: Map internal fields to Jira's schema if they are modified
			if k == "title" {
				jiraFields["summary"] = v
			} else if k == "description" {
				jiraFields["description"] = v
			}
		}
	}

	// 4. Save the modified document locally to sync state
	if err := s.store.Save(id, doc); err != nil {
		http.Error(w, "Failed to save issue locally", http.StatusInternalServerError)
		return
	}

	// NEW: 5. Push changes back to Jira
	if len(jiraFields) > 0 {
		_, err := s.connector.Update(id, engine.Issue{Fields: jiraFields})
		if err != nil {
			// Note: The local save worked, but Jira failed.
			// You might want to log this or return a specific status code.
			http.Error(w, "Saved locally but failed to sync to Jira", http.StatusBadGateway)
			return
		}
	}

	// 6. Return updated state to the user
	updatedIssue, _ := automerge.As[*engine.Issue](doc.Root())
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updatedIssue)
}

func (s *Server) DeleteIssue(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.store.Delete(id); err != nil {
		http.Error(w, "Failed to delete issue", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// WithCORS is a middleware that injects CORS headers and handles OPTIONS preflight requests.
func WithCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "*") // Change "*" to specific domains in production
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, Authorization")

		// Intercept and handle preflight OPTIONS requests
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Pass down the request to the actual multiplexer/handler
		next.ServeHTTP(w, r)
	})
}
