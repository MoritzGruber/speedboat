package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

// ─── Config ───────────────────────────────────────────────────────────────────

var (
	flagPort    = flag.Int("port", 8080, "HTTP listen port")
	flagHost    = flag.String("host", "", "Bind host (empty = all interfaces)")
	flagRepo    = flag.String("repo", "./ticket-data", "Path to Git ticket repository")
	flagCORS    = flag.String("cors", "http://localhost:5173,http://localhost:4173,http://localhost:3000", "Comma-separated allowed CORS origins")
	flagVerbose = flag.Bool("verbose", false, "Verbose request logging")
)

// ─── Types ────────────────────────────────────────────────────────────────────

type Ticket struct {
	ID          string    `json:"id"`
	Created     time.Time `json:"created"`
	UpdatedAt   time.Time `json:"updated_at"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      string    `json:"status"`   // open | in-progress | review | closed
	Priority    string    `json:"priority"` // low | medium | high | critical
	Tags        []string  `json:"tags"`
	Assignees   []string  `json:"assignees"`
	EstimateH   float64   `json:"estimate_h,omitempty"`
	VoteCount   int       `json:"vote_count"`
	Comments    []Comment `json:"comments"`
	Project     string    `json:"project"`
}

type Comment struct {
	ID     string    `json:"id"`
	Author string    `json:"author"`
	Ts     time.Time `json:"ts"`
	Body   string    `json:"body"`
}

type Project struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Created     time.Time `json:"created"`
}

type BoardColumn struct {
	Status  string   `json:"status"`
	Tickets []Ticket `json:"tickets"`
}

// ─── CRDT primitives (shared by API handler and merge driver) ─────────────────

var priorityOrder = map[string]int{"low": 0, "medium": 1, "high": 2, "critical": 3}

func maxPriority(a, b string) string {
	if priorityOrder[b] > priorityOrder[a] {
		return b
	}
	return a
}

// orSet is a full 3-way Add-Wins OR-Set.
// An element is removed only when BOTH branches removed it relative to base.
func orSet(base, ours, theirs []string) []string {
	baseSet := toSet(base)
	oursSet := toSet(ours)
	theirsSet := toSet(theirs)

	addedByOurs := setDiff(oursSet, baseSet)
	addedByTheirs := setDiff(theirsSet, baseSet)
	removedByBoth := setIntersect(setDiff(baseSet, oursSet), setDiff(baseSet, theirsSet))

	result := setUnion(setUnion(baseSet, addedByOurs), addedByTheirs)
	for k := range removedByBoth {
		delete(result, k)
	}
	out := make([]string, 0, len(result))
	for k := range result {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// orSetApply is used by the API PUT handler: no base, just unions existing + updates.
func orSetApply(existing, updates []string) []string {
	seen := toSet(existing)
	for _, v := range updates {
		seen[v] = true
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func mergeComments(a, b []Comment) []Comment {
	seen := make(map[string]bool, len(a)+len(b))
	merged := make([]Comment, 0, len(a)+len(b))
	for _, c := range append(a, b...) {
		if !seen[c.ID] {
			seen[c.ID] = true
			merged = append(merged, c)
		}
	}
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Ts.Before(merged[j].Ts)
	})
	return merged
}

func toSet(s []string) map[string]bool {
	m := make(map[string]bool, len(s))
	for _, v := range s {
		m[v] = true
	}
	return m
}

func setDiff(a, b map[string]bool) map[string]bool {
	d := make(map[string]bool)
	for k := range a {
		if !b[k] {
			d[k] = true
		}
	}
	return d
}

func setUnion(a, b map[string]bool) map[string]bool {
	u := make(map[string]bool, len(a)+len(b))
	for k := range a {
		u[k] = true
	}
	for k := range b {
		u[k] = true
	}
	return u
}

func setIntersect(a, b map[string]bool) map[string]bool {
	i := make(map[string]bool)
	for k := range a {
		if b[k] {
			i[k] = true
		}
	}
	return i
}

// ─── 3-way CRDT merge  (Git merge driver) ────────────────────────────────────
//
// Per-field strategies:
//   immutable    id, created, project       → always from ours (base establishes these)
//   lww-register title, description,        → later updated_at wins; ours wins on tie
//                status, estimate_h
//   max-register priority                   → max(ours.priority, theirs.priority)
//   or-set       tags, assignees            → add-wins 3-way with base as ancestor
//   p-counter    vote_count                 → max(ours, theirs)
//   append-log   comments                   → union by id, sorted by ts

func mergeThreeWay(base, ours, theirs Ticket) Ticket {
	result := ours // immutable fields (id, created, project) carried from ours

	oTs := ours.UpdatedAt
	tTs := theirs.UpdatedAt

	// lww-register: ticket-level timestamp decides which side wins the mutable scalar fields
	if tTs.After(oTs) {
		if theirs.Title != "" {
			result.Title = theirs.Title
		}
		if theirs.Description != "" {
			result.Description = theirs.Description
		}
		if theirs.Status != "" {
			result.Status = theirs.Status
		}
		if theirs.EstimateH > 0 {
			result.EstimateH = theirs.EstimateH
		}
		result.UpdatedAt = tTs
	}

	// max-register: priority only escalates — never de-escalates
	result.Priority = maxPriority(ours.Priority, theirs.Priority)

	// p-counter: vote count only increases
	if theirs.VoteCount > ours.VoteCount {
		result.VoteCount = theirs.VoteCount
	}

	// or-set: 3-way add-wins using base as common ancestor
	result.Tags = orSet(base.Tags, ours.Tags, theirs.Tags)
	result.Assignees = orSet(base.Assignees, ours.Assignees, theirs.Assignees)

	// append-log: union all comments by id, sorted by timestamp
	result.Comments = mergeComments(ours.Comments, theirs.Comments)

	return result
}

// ─── Apply-patch CRDT merge  (API PUT handler) ───────────────────────────────
//
// Applies a partial update document onto an existing record.
// No base — union-only semantics for sets; LWW at call time for scalars.

func mergeApply(existing, updates Ticket) Ticket {
	if updates.Title != "" {
		existing.Title = updates.Title
	}
	if updates.Description != "" {
		existing.Description = updates.Description
	}
	if updates.Status != "" {
		existing.Status = updates.Status
	}
	if updates.EstimateH > 0 {
		existing.EstimateH = updates.EstimateH
	}
	if updates.Priority != "" {
		existing.Priority = maxPriority(existing.Priority, updates.Priority)
	}
	if updates.VoteCount > existing.VoteCount {
		existing.VoteCount = updates.VoteCount
	}
	if updates.Tags != nil {
		existing.Tags = orSetApply(existing.Tags, updates.Tags)
	}
	if updates.Assignees != nil {
		existing.Assignees = orSetApply(existing.Assignees, updates.Assignees)
	}
	if updates.Comments != nil {
		existing.Comments = mergeComments(existing.Comments, updates.Comments)
	}
	return existing
}

// ─── Git merge driver subcommand ──────────────────────────────────────────────
//
// Git calls:  ticket-store merge-driver %O %A %B
//   %O = base (common ancestor) — read only
//   %A = ours (current branch)  — write result here
//   %B = theirs (incoming)      — read only
//
// Exit 0 always: CRDT resolution is conflict-free by design.

func runMergeDriver(basePath, oursPath, theirsPath string) error {
	load := func(path string) (Ticket, error) {
		data, err := os.ReadFile(path)
		if err != nil {
			return Ticket{}, fmt.Errorf("read %s: %w", path, err)
		}
		var t Ticket
		return t, json.Unmarshal(data, &t)
	}

	base, err := load(basePath)
	if err != nil {
		return err
	}
	ours, err := load(oursPath)
	if err != nil {
		return err
	}
	theirs, err := load(theirsPath)
	if err != nil {
		return err
	}

	merged := mergeThreeWay(base, ours, theirs)

	data, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal merged ticket: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(oursPath, data, 0o644); err != nil {
		return fmt.Errorf("write result to %s: %w", oursPath, err)
	}

	log.Printf("[merge-driver] %s ← CRDT merge(base=%s, theirs=%s)",
		filepath.Base(oursPath), filepath.Base(basePath), filepath.Base(theirsPath))
	return nil
}

// ─── Git Store ────────────────────────────────────────────────────────────────

type GitStore struct {
	mu       sync.RWMutex
	repoPath string
	index    map[string]map[string]*Ticket
	projects map[string]*Project
}

func NewGitStore(repoPath string) (*GitStore, error) {
	s := &GitStore{
		repoPath: repoPath,
		index:    make(map[string]map[string]*Ticket),
		projects: make(map[string]*Project),
	}

	isNew := false
	if _, err := os.Stat(filepath.Join(repoPath, ".git")); os.IsNotExist(err) {
		isNew = true
		if err := s.git("init"); err != nil {
			return nil, fmt.Errorf("git init: %w", err)
		}
		_ = s.git("config", "user.email", "ticket-store@local")
		_ = s.git("config", "user.name", "Ticket Store")
	}

	// (Re-)configure merge driver on every startup so it stays current
	// after the binary is rebuilt or moved.
	if err := s.setupMergeDriver(); err != nil {
		log.Printf("[git] WARNING: could not configure merge driver: %v", err)
	}

	if err := s.buildIndex(); err != nil {
		return nil, fmt.Errorf("build index: %w", err)
	}

	if isNew {
		_ = s.gitCommit("chore: initialise ticket store")
	}
	return s, nil
}

// setupMergeDriver registers this binary as Git's CRDT merge driver.
//
// What it does:
//  1. Writes/appends "tickets/**/*.json merge=ticket-crdt" to .gitattributes
//     (committed into the repo so it travels with any clone)
//  2. Sets merge.ticket-crdt.{name,driver} in .git/config (local)
//     pointing at the absolute path of the currently running binary
//
// The driver entry looks like:
//
//	[merge "ticket-crdt"]
//	    name   = CRDT-aware ticket merger
//	    driver = /abs/path/to/ticket-store merge-driver %O %A %B
func (s *GitStore) setupMergeDriver() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return fmt.Errorf("eval symlinks: %w", err)
	}

	// Warn if running via `go run` — the temp binary path changes each build.
	if strings.Contains(exe, os.TempDir()) || strings.Contains(exe, "go-build") {
		log.Printf("[git] WARNING: running via 'go run'; merge driver path is a temp binary.")
		log.Printf("[git]          Use 'make backend-build && ./ticket-store' for a stable driver path.")
	}

	// ── 1. .gitattributes ───────────────────────────────────────────────────
	const attrLine = "tickets/**/*.json merge=ticket-crdt\n"
	attrPath := filepath.Join(s.repoPath, ".gitattributes")
	existing, _ := os.ReadFile(attrPath)
	if !strings.Contains(string(existing), "merge=ticket-crdt") {
		f, err := os.OpenFile(attrPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return fmt.Errorf("open .gitattributes: %w", err)
		}
		_, _ = f.WriteString(attrLine)
		_ = f.Close()
		log.Printf("[git] .gitattributes updated with ticket-crdt driver mapping")
	}

	// ── 2. .git/config merge driver ─────────────────────────────────────────
	// Format: /abs/path/to/ticket-store merge-driver %O %A %B
	// %O, %A, %B are Git placeholders for base, ours, theirs file paths.
	driver := fmt.Sprintf("%s merge-driver %%O %%A %%B", exe)

	if err := s.git("config", "merge.ticket-crdt.name", "CRDT-aware ticket merger"); err != nil {
		return fmt.Errorf("set merge driver name: %w", err)
	}
	if err := s.git("config", "merge.ticket-crdt.driver", driver); err != nil {
		return fmt.Errorf("set merge driver command: %w", err)
	}

	log.Printf("[git] merge driver registered → %s", driver)
	return nil
}

func (s *GitStore) git(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = s.repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	fmt.Printf("[git] %s: %s\n", strings.Join(args, " "), strings.TrimSpace(string(out)))
	return nil
}

func (s *GitStore) gitOut(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = s.repoPath
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (s *GitStore) gitCommit(msg string) error {
	if err := s.git("add", "-A"); err != nil {
		return err
	}
	cmd := exec.Command("git", "diff", "--cached", "--quiet")
	cmd.Dir = s.repoPath
	if err := cmd.Run(); err != nil {
		fmt.Print("[git] no changes to commit\n")
		return nil // nothing to commit
	}
	fmt.Printf("[git] committing: %s\n", msg)
	return s.git("commit", "-m", "\"", msg, "\"")
}

func (s *GitStore) buildIndex() error {
	ticketsDir := filepath.Join(s.repoPath, "tickets")
	if err := os.MkdirAll(ticketsDir, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(ticketsDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		projID := entry.Name()
		projFile := filepath.Join(ticketsDir, projID, "project.json")
		if data, err := os.ReadFile(projFile); err == nil {
			var proj Project
			if json.Unmarshal(data, &proj) == nil {
				s.projects[projID] = &proj
			}
		}
		if s.projects[projID] == nil {
			s.projects[projID] = &Project{ID: projID, Name: projID, Created: time.Now()}
		}
		s.index[projID] = make(map[string]*Ticket)
		_ = filepath.WalkDir(filepath.Join(ticketsDir, projID), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || d.Name() == "project.json" || filepath.Ext(path) != ".json" {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			var t Ticket
			if json.Unmarshal(data, &t) == nil {
				s.index[projID][t.ID] = &t
			}
			return nil
		})
	}
	return nil
}

// ─── Project operations ───────────────────────────────────────────────────────

func (s *GitStore) ListProjects() []Project {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Project, 0, len(s.projects))
	for _, p := range s.projects {
		result = append(result, *p)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Created.Before(result[j].Created)
	})
	return result
}

func (s *GitStore) CreateProject(proj Project) (*Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	proj.Created = time.Now()
	projDir := filepath.Join(s.repoPath, "tickets", proj.ID)
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		return nil, err
	}
	data, _ := json.MarshalIndent(proj, "", "  ")
	if err := os.WriteFile(filepath.Join(projDir, "project.json"), data, 0o644); err != nil {
		return nil, err
	}
	for _, sub := range []string{"open", "closed"} {
		d := filepath.Join(projDir, sub)
		_ = os.MkdirAll(d, 0o755)
		_ = os.WriteFile(filepath.Join(d, ".gitkeep"), nil, 0o644)
	}
	s.projects[proj.ID] = &proj
	s.index[proj.ID] = make(map[string]*Ticket)
	_ = s.gitCommit("feat: create project " + proj.ID)
	return &proj, nil
}

// ─── Ticket operations ────────────────────────────────────────────────────────

func (s *GitStore) ticketPath(projID, ticketID, status string) string {
	sub := "open"
	if status == "closed" {
		sub = "closed"
	}
	return filepath.Join(s.repoPath, "tickets", projID, sub, ticketID+".json")
}

func (s *GitStore) ListTickets(projID string) ([]Ticket, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	idx, ok := s.index[projID]
	if !ok {
		return nil, fmt.Errorf("project %q not found", projID)
	}
	result := make([]Ticket, 0, len(idx))
	for _, t := range idx {
		result = append(result, *t)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Created.After(result[j].Created)
	})
	return result, nil
}

func (s *GitStore) GetTicket(projID, ticketID string) (*Ticket, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	idx, ok := s.index[projID]
	if !ok {
		return nil, fmt.Errorf("project %q not found", projID)
	}
	t, ok := idx[ticketID]
	if !ok {
		return nil, fmt.Errorf("ticket %q not found", ticketID)
	}
	cp := *t
	return &cp, nil
}

func (s *GitStore) CreateTicket(projID string, t Ticket) (*Ticket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.index[projID]; !ok {
		return nil, fmt.Errorf("project %q not found", projID)
	}
	now := time.Now()
	t.Project = projID
	t.Created = now
	t.UpdatedAt = now
	if t.ID == "" {
		prefix := strings.ToUpper(projID)
		if len(prefix) > 4 {
			prefix = prefix[:4]
		}
		t.ID = fmt.Sprintf("%s-%d", prefix, len(s.index[projID])+1)
	}
	if t.Status == "" {
		t.Status = "open"
	}
	if t.Priority == "" {
		t.Priority = "medium"
	}
	if t.Tags == nil {
		t.Tags = []string{}
	}
	if t.Assignees == nil {
		t.Assignees = []string{}
	}
	if t.Comments == nil {
		t.Comments = []Comment{}
	}

	path := s.ticketPath(projID, t.ID, t.Status)
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	data, _ := json.MarshalIndent(t, "", "  ")
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return nil, err
	}
	s.index[projID][t.ID] = &t
	_ = s.gitCommit(fmt.Sprintf("feat(%s): create %s %q", projID, t.ID, t.Title))
	fmt.Printf("[git] created ticket %s/%s with status %q and priority %q\n", projID, t.ID, t.Status, t.Priority)
	return &t, nil
}

func (s *GitStore) UpdateTicket(projID, ticketID string, updates Ticket) (*Ticket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, ok := s.index[projID]
	if !ok {
		return nil, fmt.Errorf("project %q not found", projID)
	}
	existing, ok := idx[ticketID]
	if !ok {
		return nil, fmt.Errorf("ticket %q not found", ticketID)
	}
	prevStatus := existing.Status
	prevPath := s.ticketPath(projID, ticketID, prevStatus)

	merged := mergeApply(*existing, updates)
	merged.UpdatedAt = time.Now()

	newPath := s.ticketPath(projID, ticketID, merged.Status)
	_ = os.MkdirAll(filepath.Dir(newPath), 0o755)
	data, _ := json.MarshalIndent(merged, "", "  ")
	data = append(data, '\n')
	if err := os.WriteFile(newPath, data, 0o644); err != nil {
		return nil, err
	}
	if prevStatus != merged.Status {
		_ = os.Remove(prevPath)
	}
	s.index[projID][ticketID] = &merged
	_ = s.gitCommit(fmt.Sprintf("update(%s): %s priority=%s status=%s",
		projID, ticketID, merged.Priority, merged.Status))
	fmt.Printf("[git] updated ticket %s/%s: priority=%s status=%s\n", projID, ticketID, merged.Priority, merged.Status)
	return &merged, nil
}

func (s *GitStore) DeleteTicket(projID, ticketID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, ok := s.index[projID]
	if !ok {
		return fmt.Errorf("project %q not found", projID)
	}
	existing, ok := idx[ticketID]
	if !ok {
		return fmt.Errorf("ticket %q not found", ticketID)
	}
	path := s.ticketPath(projID, ticketID, existing.Status)
	if err := os.Remove(path); err != nil {
		return err
	}
	delete(s.index[projID], ticketID)
	_ = s.gitCommit(fmt.Sprintf("delete(%s): %s", projID, ticketID))
	return nil
}

func (s *GitStore) GetBoard(projID string) ([]BoardColumn, error) {
	tickets, err := s.ListTickets(projID)
	if err != nil {
		return nil, err
	}
	statuses := []string{"open", "in-progress", "review", "closed"}
	colMap := make(map[string]*BoardColumn)
	for _, st := range statuses {
		colMap[st] = &BoardColumn{Status: st, Tickets: []Ticket{}}
	}
	for _, t := range tickets {
		col, ok := colMap[t.Status]
		if !ok {
			col = &BoardColumn{Status: t.Status, Tickets: []Ticket{}}
			colMap[t.Status] = col
		}
		col.Tickets = append(col.Tickets, t)
	}
	cols := make([]BoardColumn, 0, len(statuses))
	for _, st := range statuses {
		cols = append(cols, *colMap[st])
	}
	return cols, nil
}

func (s *GitStore) GetHistory(projID, n string) ([]string, error) {
	if n == "" {
		n = "20"
	}
	out, err := s.gitOut("log", "--oneline", "-"+n, "--", "tickets/"+projID+"/")
	if err != nil {
		return nil, err
	}
	var lines []string
	for _, l := range strings.Split(out, "\n") {
		if l != "" {
			lines = append(lines, l)
		}
	}
	return lines, nil
}

// ─── HTTP helpers ─────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// ─── HTTP handlers ────────────────────────────────────────────────────────────

type Server struct{ store *GitStore }

func (sv *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, sv.store.ListProjects())
}

func (sv *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var proj Project
	if err := json.NewDecoder(r.Body).Decode(&proj); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if proj.ID == "" || proj.Name == "" {
		writeError(w, http.StatusBadRequest, "id and name are required")
		return
	}
	created, err := sv.store.CreateProject(proj)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (sv *Server) handleListTickets(w http.ResponseWriter, r *http.Request) {
	projID := chi.URLParam(r, "proj")
	tickets, err := sv.store.ListTickets(projID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, tickets)
}

func (sv *Server) handleCreateTicket(w http.ResponseWriter, r *http.Request) {
	projID := chi.URLParam(r, "proj")
	var t Ticket
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if t.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	created, err := sv.store.CreateTicket(projID, t)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	sv.store.gitCommit(fmt.Sprintf("feat(%s): create ticket %q", projID, created.ID))
	writeJSON(w, http.StatusCreated, created)
}

func (sv *Server) handleGetTicket(w http.ResponseWriter, r *http.Request) {
	projID, ticketID := chi.URLParam(r, "proj"), chi.URLParam(r, "id")
	t, err := sv.store.GetTicket(projID, ticketID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (sv *Server) handleUpdateTicket(w http.ResponseWriter, r *http.Request) {
	projID, ticketID := chi.URLParam(r, "proj"), chi.URLParam(r, "id")
	var updates Ticket
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	updated, err := sv.store.UpdateTicket(projID, ticketID, updates)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (sv *Server) handleDeleteTicket(w http.ResponseWriter, r *http.Request) {
	projID, ticketID := chi.URLParam(r, "proj"), chi.URLParam(r, "id")
	if err := sv.store.DeleteTicket(projID, ticketID); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (sv *Server) handleGetBoard(w http.ResponseWriter, r *http.Request) {
	projID := chi.URLParam(r, "proj")
	cols, err := sv.store.GetBoard(projID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cols)
}

func (sv *Server) handleGetHistory(w http.ResponseWriter, r *http.Request) {
	projID := chi.URLParam(r, "proj")
	lines, err := sv.store.GetHistory(projID, r.URL.Query().Get("n"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, lines)
}

// ─── Main ─────────────────────────────────────────────────────────────────────

func main() {
	// ── Subcommand: merge-driver ─────────────────────────────────────────────
	// Invoked by Git automatically during merges on ticket JSON files.
	// Must be checked BEFORE flag.Parse() to avoid consuming file path args.
	if len(os.Args) >= 2 && os.Args[1] == "merge-driver" {
		if len(os.Args) != 5 {
			fmt.Fprintln(os.Stderr, "usage: ticket-store merge-driver <base> <ours> <theirs>")
			os.Exit(1)
		}
		if err := runMergeDriver(os.Args[2], os.Args[3], os.Args[4]); err != nil {
			// Log the error but still exit 0 — Git must not see a conflict.
			fmt.Fprintf(os.Stderr, "[merge-driver] error: %v (falling back to ours)\n", err)
		}
		os.Exit(0)
	}

	// ── Normal server mode ───────────────────────────────────────────────────
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Ticket Store — Git-backed project management API\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  %s [options]                 Start the HTTP server\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s merge-driver %%O %%A %%B   Git CRDT merge driver (called by Git)\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  %s\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -port 9090 -repo /data/tickets\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -host 0.0.0.0 -cors https://myapp.com -verbose\n", os.Args[0])
	}
	flag.Parse()

	repoPath := *flagRepo
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		log.Fatalf("mkdir %s: %v", repoPath, err)
	}

	store, err := NewGitStore(repoPath)
	if err != nil {
		log.Fatalf("init store: %v", err)
	}

	sv := &Server{store: store}

	r := chi.NewRouter()
	if *flagVerbose {
		r.Use(middleware.Logger)
	}
	r.Use(middleware.Recoverer)

	origins := strings.Split(*flagCORS, ",")
	for i, o := range origins {
		origins[i] = strings.TrimSpace(o)
	}
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: origins,
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Content-Type"},
	}))

	r.Route("/api", func(r chi.Router) {
		r.Get("/projects", sv.handleListProjects)
		r.Post("/projects", sv.handleCreateProject)
		r.Route("/projects/{proj}", func(r chi.Router) {
			r.Get("/board", sv.handleGetBoard)
			r.Get("/history", sv.handleGetHistory)
			r.Get("/tickets", sv.handleListTickets)
			r.Post("/tickets", sv.handleCreateTicket)
			r.Get("/tickets/{id}", sv.handleGetTicket)
			r.Put("/tickets/{id}", sv.handleUpdateTicket)
			r.Delete("/tickets/{id}", sv.handleDeleteTicket)
		})
	})

	addr := fmt.Sprintf("%s:%d", *flagHost, *flagPort)
	log.Printf("Ticket Store API")
	log.Printf("  listening  : %s", addr)
	log.Printf("  repo       : %s", repoPath)
	log.Printf("  cors       : %s", *flagCORS)
	log.Printf("  merge drv  : %s merge-driver %%O %%A %%B", os.Args[0])
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
