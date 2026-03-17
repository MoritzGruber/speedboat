package main

import (
	"log/slog"
	"os"

	"github.com/automerge/automerge-go"
)

// Document represents our data.
// Notice we no longer rely on json tags. The automerge map keys
// must match these exported field names exactly.
type Document struct {
	Title    string
	Status   string
	Priority string
}

func main() {
	// Initialize slog to output to standard out
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// 1. Peer 1 creates the initial document
	doc1 := automerge.New()

	// FIX: Keys must exactly match the struct field names (Title, Status)
	_ = doc1.Path("Title").Set("Project Alpha")
	_ = doc1.Path("Status").Set("planning")

	printDoc(logger, "Peer 1 (Initial)", doc1)

	// 2. Simulate sending the document over the network to Peer 2
	savedBytes := doc1.Save()
	doc2, err := automerge.Load(savedBytes)
	if err != nil {
		logger.Error("Failed to load doc2", "error", err)
		os.Exit(1)
	}

	// 3. OFFLINE EDITS: Both peers make changes independently

	// Peer 1 updates the existing status
	_ = doc1.Path("Status").Set("in-progress")

	// Peer 2 adds a completely new field (Priority)
	_ = doc2.Path("Priority").Set("high")

	logger.Info("--- After Offline Edits ---")
	printDoc(logger, "Peer 1", doc1)
	printDoc(logger, "Peer 2", doc2)

	// 4. MERGE: Peer 2 reconnects to Peer 1, and they merge states
	//
	_, err = doc1.Merge(doc2)
	if err != nil {
		logger.Error("Failed to merge documents", "error", err)
		os.Exit(1)
	}

	logger.Info("--- After Merge ---")
	printDoc(logger, "Peer 1 (Merged)", doc1)
}

// printDoc uses slog to log the parsed struct values
func printDoc(logger *slog.Logger, label string, doc *automerge.Doc) {
	// automerge.As maps the CRDT data back into our struct
	data, err := automerge.As[*Document](doc.Root())
	if err != nil {
		logger.Error("Failed to parse document", "label", label, "error", err)
		return
	}

	logger.Info("Document State",
		"peer", label,
		"Title", data.Title,
		"Status", data.Status,
		"Priority", data.Priority,
	)
}
