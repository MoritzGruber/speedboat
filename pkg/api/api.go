package api

import (
	"encoding/json"
	"net/http"

	"github.com/MoritzGruber/speedboat.git/pkg/engine"
	"github.com/MoritzGruber/speedboat.git/pkg/store"
	"github.com/automerge/automerge-go"
)

type Server struct {
	store *store.FileStore
}

// NewServer creates a new API server instance tied to the file store.
func NewServer(store *store.FileStore) *Server {
	return &Server{store: store}
}

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

	// Create a new Automerge doc and map the Go struct values into it
	doc := automerge.New()
	_ = doc.Path("ID").Set(issue.ID)
	_ = doc.Path("Key").Set(issue.Key)

	if issue.Fields != nil {
		_ = doc.Path("Fields").Set(issue.Fields)
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

	// 3. Apply updates specifically to the Automerge document
	if updateData.Key != "" {
		_ = doc.Path("Key").Set(updateData.Key)
	}

	// Patch nested dynamic fields gracefully
	if updateData.Fields != nil {
		fieldsPath := doc.Path("Fields")
		for k, v := range updateData.Fields {
			_ = fieldsPath.Path(k).Set(v)
		}
	}

	// 4. Save the modified document to sync state
	if err := s.store.Save(id, doc); err != nil {
		http.Error(w, "Failed to save issue", http.StatusInternalServerError)
		return
	}

	// 5. Return updated state to the user
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
