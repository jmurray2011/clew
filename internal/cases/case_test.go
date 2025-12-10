package cases

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// testCtx returns a context for testing.
func testCtx() context.Context {
	return context.Background()
}

// newTestManager creates a Manager with a temporary directory for testing.
func newTestManager(t *testing.T) *Manager {
	t.Helper()
	tmpDir := t.TempDir()
	return &Manager{
		casesDir:  filepath.Join(tmpDir, "cases"),
		stateFile: filepath.Join(tmpDir, "state.yaml"),
	}
}

func TestGenerateSlug(t *testing.T) {
	tests := []struct {
		name     string
		title    string
		contains string // Check that the slug contains this string
	}{
		{
			name:     "simple title",
			title:    "My Investigation",
			contains: "my-investigation",
		},
		{
			name:     "title with special chars",
			title:    "Bug #123: Critical Error!",
			contains: "bug-123-critical-error",
		},
		{
			name:     "title with underscores",
			title:    "error_logs_analysis",
			contains: "error-logs-analysis",
		},
		{
			name:     "title with multiple spaces",
			title:    "Too   Many    Spaces",
			contains: "too-many-spaces",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slug := GenerateSlug(tt.title)
			if !containsString(slug, tt.contains) {
				t.Errorf("expected slug to contain %q, got %q", tt.contains, slug)
			}
			// Should always have a date suffix
			if !containsDate(slug) {
				t.Errorf("expected slug to contain date, got %q", slug)
			}
		})
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsSubstring(s, substr)))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestManager_CreateCase(t *testing.T) {
	m := newTestManager(t)

	c, err := m.CreateCase(testCtx(),"Test Investigation", "")
	if err != nil {
		t.Fatalf("CreateCase failed: %v", err)
	}

	if c.Title != "Test Investigation" {
		t.Errorf("expected title 'Test Investigation', got %q", c.Title)
	}
	if c.Status != StatusActive {
		t.Errorf("expected status active, got %q", c.Status)
	}
	if c.ID == "" {
		t.Error("expected non-empty ID")
	}
}

func TestManager_CreateCaseWithCustomID(t *testing.T) {
	m := newTestManager(t)

	c, err := m.CreateCase(testCtx(),"Test Investigation", "custom-id")
	if err != nil {
		t.Fatalf("CreateCase failed: %v", err)
	}

	if c.ID != "custom-id" {
		t.Errorf("expected ID 'custom-id', got %q", c.ID)
	}
}

func TestManager_CreateCaseDuplicate(t *testing.T) {
	m := newTestManager(t)

	_, err := m.CreateCase(testCtx(),"Test", "same-id")
	if err != nil {
		t.Fatalf("first CreateCase failed: %v", err)
	}

	_, err = m.CreateCase(testCtx(),"Test 2", "same-id")
	if err == nil {
		t.Error("expected error for duplicate ID")
	}
}

func TestManager_LoadCase(t *testing.T) {
	m := newTestManager(t)

	created, err := m.CreateCase(testCtx(),"Test Load", "test-load")
	if err != nil {
		t.Fatalf("CreateCase failed: %v", err)
	}

	loaded, err := m.LoadCase(testCtx(),"test-load")
	if err != nil {
		t.Fatalf("LoadCase failed: %v", err)
	}

	if loaded.ID != created.ID {
		t.Errorf("expected ID %q, got %q", created.ID, loaded.ID)
	}
	if loaded.Title != created.Title {
		t.Errorf("expected title %q, got %q", created.Title, loaded.Title)
	}
}

func TestManager_LoadCaseNotFound(t *testing.T) {
	m := newTestManager(t)
	_ = m.EnsureDirectories(testCtx())

	_, err := m.LoadCase(testCtx(),"nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent case")
	}
}

func TestManager_DeleteCase(t *testing.T) {
	m := newTestManager(t)

	_, err := m.CreateCase(testCtx(),"To Delete", "to-delete")
	if err != nil {
		t.Fatalf("CreateCase failed: %v", err)
	}

	err = m.DeleteCase(testCtx(),"to-delete")
	if err != nil {
		t.Fatalf("DeleteCase failed: %v", err)
	}

	// Should not be loadable anymore
	_, err = m.LoadCase(testCtx(),"to-delete")
	if err == nil {
		t.Error("expected error loading deleted case")
	}
}

func TestManager_DeleteCaseNotFound(t *testing.T) {
	m := newTestManager(t)
	_ = m.EnsureDirectories(testCtx())

	err := m.DeleteCase(testCtx(),"nonexistent")
	if err == nil {
		t.Error("expected error deleting nonexistent case")
	}
}

func TestManager_DeleteActiveCaseClearsState(t *testing.T) {
	m := newTestManager(t)

	_, err := m.CreateCase(testCtx(),"Active Case", "active-case")
	if err != nil {
		t.Fatalf("CreateCase failed: %v", err)
	}

	// Verify it's active
	activeCase, err := m.GetActiveCase(testCtx())
	if err != nil {
		t.Fatalf("GetActiveCase failed: %v", err)
	}
	if activeCase == nil || activeCase.ID != "active-case" {
		t.Error("expected case to be active")
	}

	// Delete the active case
	err = m.DeleteCase(testCtx(),"active-case")
	if err != nil {
		t.Fatalf("DeleteCase failed: %v", err)
	}

	// Should have no active case now
	activeCase, err = m.GetActiveCase(testCtx())
	if err != nil {
		t.Fatalf("GetActiveCase failed: %v", err)
	}
	if activeCase != nil {
		t.Error("expected no active case after deletion")
	}
}

func TestManager_ListCases(t *testing.T) {
	m := newTestManager(t)

	// Create several cases
	_, _ = m.CreateCase(testCtx(),"Case 1", "case-1")
	_, _ = m.CreateCase(testCtx(),"Case 2", "case-2")
	_, _ = m.CreateCase(testCtx(),"Case 3", "case-3")

	cases, err := m.ListCases(testCtx())
	if err != nil {
		t.Fatalf("ListCases failed: %v", err)
	}

	if len(cases) != 3 {
		t.Errorf("expected 3 cases, got %d", len(cases))
	}
}

func TestManager_ListCasesEmpty(t *testing.T) {
	m := newTestManager(t)

	cases, err := m.ListCases(testCtx())
	if err != nil {
		t.Fatalf("ListCases failed: %v", err)
	}

	if len(cases) != 0 {
		t.Errorf("expected 0 cases, got %d", len(cases))
	}
}

func TestManager_ListCasesSkipsInvalidFiles(t *testing.T) {
	m := newTestManager(t)
	_ = m.EnsureDirectories(testCtx())

	// Create a valid case
	_, _ = m.CreateCase(testCtx(),"Valid Case", "valid-case")

	// Create an invalid YAML file
	invalidPath := filepath.Join(m.casesDir, "invalid.yaml")
	_ = os.WriteFile(invalidPath, []byte("not: valid: yaml: {{{{"), 0644)

	// Create a non-YAML file (should be ignored)
	txtPath := filepath.Join(m.casesDir, "readme.txt")
	_ = os.WriteFile(txtPath, []byte("This is not a case"), 0644)

	cases, err := m.ListCases(testCtx())
	if err != nil {
		t.Fatalf("ListCases failed: %v", err)
	}

	// Should only have the valid case
	if len(cases) != 1 {
		t.Errorf("expected 1 valid case, got %d", len(cases))
	}
}

func TestManager_SetAndGetActiveCase(t *testing.T) {
	m := newTestManager(t)

	_, err := m.CreateCase(testCtx(),"Case 1", "case-1")
	if err != nil {
		t.Fatalf("CreateCase failed: %v", err)
	}

	// Should be active after creation
	active, err := m.GetActiveCase(testCtx())
	if err != nil {
		t.Fatalf("GetActiveCase failed: %v", err)
	}
	if active == nil || active.ID != "case-1" {
		t.Error("expected case-1 to be active")
	}

	// Create another case
	_, err = m.CreateCase(testCtx(),"Case 2", "case-2")
	if err != nil {
		t.Fatalf("CreateCase failed: %v", err)
	}

	// Now case-2 should be active
	active, err = m.GetActiveCase(testCtx())
	if err != nil {
		t.Fatalf("GetActiveCase failed: %v", err)
	}
	if active == nil || active.ID != "case-2" {
		t.Error("expected case-2 to be active")
	}

	// Switch back to case-1
	err = m.SetActiveCase(testCtx(),"case-1")
	if err != nil {
		t.Fatalf("SetActiveCase failed: %v", err)
	}

	active, err = m.GetActiveCase(testCtx())
	if err != nil {
		t.Fatalf("GetActiveCase failed: %v", err)
	}
	if active == nil || active.ID != "case-1" {
		t.Error("expected case-1 to be active after switch")
	}
}

func TestManager_ClearActiveCase(t *testing.T) {
	m := newTestManager(t)

	_, _ = m.CreateCase(testCtx(),"Test Case", "test-case")

	err := m.ClearActiveCase(testCtx())
	if err != nil {
		t.Fatalf("ClearActiveCase failed: %v", err)
	}

	active, _ := m.GetActiveCase(testCtx())
	if active != nil {
		t.Error("expected no active case after clear")
	}
}

func TestManager_CloseCase(t *testing.T) {
	m := newTestManager(t)

	_, err := m.CreateCase(testCtx(),"Open Case", "open-case")
	if err != nil {
		t.Fatalf("CreateCase failed: %v", err)
	}

	err = m.CloseCase(testCtx(),"Investigation complete")
	if err != nil {
		t.Fatalf("CloseCase failed: %v", err)
	}

	c, err := m.LoadCase(testCtx(),"open-case")
	if err != nil {
		t.Fatalf("LoadCase failed: %v", err)
	}

	if c.Status != StatusClosed {
		t.Errorf("expected status closed, got %q", c.Status)
	}
	if c.Summary != "Investigation complete" {
		t.Errorf("expected summary 'Investigation complete', got %q", c.Summary)
	}
}

func TestManager_AddQueryToTimeline(t *testing.T) {
	m := newTestManager(t)

	_, err := m.CreateCase(testCtx(),"Timeline Test", "timeline-test")
	if err != nil {
		t.Fatalf("CreateCase failed: %v", err)
	}

	entry := TimelineEntry{
		Timestamp: time.Now(),
		Type:      "query",
		SourceURI: "cloudwatch:///my-log-group",
		Filter:    "error",
		Results:   10,
	}

	err = m.AddQueryToTimeline(testCtx(),entry)
	if err != nil {
		t.Fatalf("AddQueryToTimeline failed: %v", err)
	}

	c, err := m.LoadCase(testCtx(),"timeline-test")
	if err != nil {
		t.Fatalf("LoadCase failed: %v", err)
	}

	if len(c.Timeline) != 1 {
		t.Errorf("expected 1 timeline entry, got %d", len(c.Timeline))
	}
	if c.Timeline[0].Filter != "error" {
		t.Errorf("expected filter 'error', got %q", c.Timeline[0].Filter)
	}
}

func TestManager_AddEvidence(t *testing.T) {
	m := newTestManager(t)

	_, err := m.CreateCase(testCtx(),"Evidence Test", "evidence-test")
	if err != nil {
		t.Fatalf("CreateCase failed: %v", err)
	}

	item := EvidenceItem{
		Ptr:       "file:///var/log/app.log#42",
		Message:   "Error occurred",
		Timestamp: time.Now(),
		Stream:    "app.log",
	}

	err = m.AddEvidence(testCtx(),item)
	if err != nil {
		t.Fatalf("AddEvidence failed: %v", err)
	}

	evidence, err := m.GetEvidence(testCtx())
	if err != nil {
		t.Fatalf("GetEvidence failed: %v", err)
	}

	if len(evidence) != 1 {
		t.Errorf("expected 1 evidence item, got %d", len(evidence))
	}
	if evidence[0].Message != "Error occurred" {
		t.Errorf("expected message 'Error occurred', got %q", evidence[0].Message)
	}
}

func TestManager_AnnotateEvidence(t *testing.T) {
	m := newTestManager(t)

	_, err := m.CreateCase(testCtx(),"Annotate Test", "annotate-test")
	if err != nil {
		t.Fatalf("CreateCase failed: %v", err)
	}

	item := EvidenceItem{
		Ptr:       "file:///var/log/app.log#42",
		Message:   "Error occurred",
		Timestamp: time.Now(),
		Stream:    "app.log",
	}

	err = m.AddEvidence(testCtx(),item)
	if err != nil {
		t.Fatalf("AddEvidence failed: %v", err)
	}

	err = m.AnnotateEvidence(testCtx(),"file:///var/log/app.log#42", "This is the root cause")
	if err != nil {
		t.Fatalf("AnnotateEvidence failed: %v", err)
	}

	evidence, _ := m.GetEvidence(testCtx())
	if evidence[0].Annotation != "This is the root cause" {
		t.Errorf("expected annotation 'This is the root cause', got %q", evidence[0].Annotation)
	}
}

func TestManager_GetCaseIDs(t *testing.T) {
	m := newTestManager(t)

	_, _ = m.CreateCase(testCtx(),"Case A", "case-a")
	_, _ = m.CreateCase(testCtx(),"Case B", "case-b")

	ids := m.GetCaseIDs(testCtx())

	if len(ids) != 2 {
		t.Errorf("expected 2 IDs, got %d", len(ids))
	}
}

func TestManager_StateFileCorrupted(t *testing.T) {
	m := newTestManager(t)
	_ = m.EnsureDirectories(testCtx())

	// Write corrupted state file
	_ = os.WriteFile(m.stateFile, []byte("{{{{not yaml"), 0644)

	// LoadState should return error
	_, err := m.LoadState(testCtx())
	if err == nil {
		t.Error("expected error for corrupted state file")
	}
}

func TestManager_DeleteCaseWithCorruptedState(t *testing.T) {
	m := newTestManager(t)

	// Create a case first
	_, err := m.CreateCase(testCtx(),"Test", "test-case")
	if err != nil {
		t.Fatalf("CreateCase failed: %v", err)
	}

	// Corrupt the state file
	_ = os.WriteFile(m.stateFile, []byte("{{{{not yaml"), 0644)

	// Delete should still work (logs warning but doesn't fail)
	err = m.DeleteCase(testCtx(),"test-case")
	if err != nil {
		t.Fatalf("DeleteCase should succeed even with corrupted state: %v", err)
	}

	// Case should be deleted
	_, err = m.LoadCase(testCtx(),"test-case")
	if err == nil {
		t.Error("expected case to be deleted")
	}
}

func TestManager_OpenCase(t *testing.T) {
	m := newTestManager(t)

	// Create two cases
	_, err := m.CreateCase(testCtx(),"Case 1", "case-1")
	if err != nil {
		t.Fatalf("CreateCase failed: %v", err)
	}
	_, err = m.CreateCase(testCtx(),"Case 2", "case-2")
	if err != nil {
		t.Fatalf("CreateCase failed: %v", err)
	}

	// Currently case-2 is active
	active, _ := m.GetActiveCase(testCtx())
	if active.ID != "case-2" {
		t.Fatal("expected case-2 to be active")
	}

	// Switch to case-1
	c, err := m.OpenCase(testCtx(),"case-1")
	if err != nil {
		t.Fatalf("OpenCase failed: %v", err)
	}

	if c.ID != "case-1" {
		t.Errorf("expected ID 'case-1', got %q", c.ID)
	}

	// Verify case-1 is now active
	active, _ = m.GetActiveCase(testCtx())
	if active.ID != "case-1" {
		t.Errorf("expected case-1 to be active, got %q", active.ID)
	}
}

func TestManager_AddNoteToTimeline(t *testing.T) {
	m := newTestManager(t)

	_, err := m.CreateCase(testCtx(),"Note Test", "note-test")
	if err != nil {
		t.Fatalf("CreateCase failed: %v", err)
	}

	err = m.AddNoteToTimeline(testCtx(),"This is my investigation note", "inline")
	if err != nil {
		t.Fatalf("AddNoteToTimeline failed: %v", err)
	}

	c, _ := m.LoadCase(testCtx(),"note-test")
	if len(c.Timeline) != 1 {
		t.Fatalf("expected 1 timeline entry, got %d", len(c.Timeline))
	}
	if c.Timeline[0].Type != "note" {
		t.Errorf("expected type 'note', got %q", c.Timeline[0].Type)
	}
	if c.Timeline[0].Content != "This is my investigation note" {
		t.Errorf("expected content match, got %q", c.Timeline[0].Content)
	}
}

func TestManager_MarkLastQuery(t *testing.T) {
	m := newTestManager(t)

	_, err := m.CreateCase(testCtx(),"Mark Test", "mark-test")
	if err != nil {
		t.Fatalf("CreateCase failed: %v", err)
	}

	// Add a query
	entry := TimelineEntry{
		Type:      "query",
		SourceURI: "cloudwatch:///my-log-group",
		Filter:    "error",
		Results:   10,
	}
	_ = m.AddQueryToTimeline(testCtx(),entry)

	// Mark the last query
	err = m.MarkLastQuery(testCtx())
	if err != nil {
		t.Fatalf("MarkLastQuery failed: %v", err)
	}

	c, _ := m.LoadCase(testCtx(),"mark-test")
	if !c.Timeline[0].Marked {
		t.Error("expected last query to be marked")
	}
}

func TestManager_MarkLastQuery_NoQueries(t *testing.T) {
	m := newTestManager(t)

	_, err := m.CreateCase(testCtx(),"Empty Mark", "empty-mark")
	if err != nil {
		t.Fatalf("CreateCase failed: %v", err)
	}

	err = m.MarkLastQuery(testCtx())
	if err == nil {
		t.Error("expected error when no queries to mark")
	}
}

func TestManager_GetTimeline_FilterByType(t *testing.T) {
	m := newTestManager(t)

	_, err := m.CreateCase(testCtx(),"Timeline Filter", "timeline-filter")
	if err != nil {
		t.Fatalf("CreateCase failed: %v", err)
	}

	// Add a query and a note
	_ = m.AddQueryToTimeline(testCtx(),TimelineEntry{Type: "query", Filter: "error"})
	_ = m.AddNoteToTimeline(testCtx(),"A note", "inline")
	_ = m.AddQueryToTimeline(testCtx(),TimelineEntry{Type: "query", Filter: "warn"})

	// Get all timeline entries
	entries, err := m.GetTimeline(testCtx(),"", false)
	if err != nil {
		t.Fatalf("GetTimeline failed: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}

	// Get only queries
	queries, err := m.GetTimeline(testCtx(),"query", false)
	if err != nil {
		t.Fatalf("GetTimeline failed: %v", err)
	}
	if len(queries) != 2 {
		t.Errorf("expected 2 queries, got %d", len(queries))
	}

	// Get only notes
	notes, err := m.GetTimeline(testCtx(),"note", false)
	if err != nil {
		t.Fatalf("GetTimeline failed: %v", err)
	}
	if len(notes) != 1 {
		t.Errorf("expected 1 note, got %d", len(notes))
	}
}

func TestManager_GetTimeline_MarkedOnly(t *testing.T) {
	m := newTestManager(t)

	_, err := m.CreateCase(testCtx(),"Marked Filter", "marked-filter")
	if err != nil {
		t.Fatalf("CreateCase failed: %v", err)
	}

	// Add queries, mark one
	_ = m.AddQueryToTimeline(testCtx(),TimelineEntry{Type: "query", Filter: "error"})
	_ = m.MarkLastQuery(testCtx())
	_ = m.AddQueryToTimeline(testCtx(),TimelineEntry{Type: "query", Filter: "warn"})

	// Get marked only
	marked, err := m.GetTimeline(testCtx(),"", true)
	if err != nil {
		t.Fatalf("GetTimeline failed: %v", err)
	}
	if len(marked) != 1 {
		t.Errorf("expected 1 marked entry, got %d", len(marked))
	}
}

func TestManager_SetSummary(t *testing.T) {
	m := newTestManager(t)

	_, err := m.CreateCase(testCtx(),"Summary Test", "summary-test")
	if err != nil {
		t.Fatalf("CreateCase failed: %v", err)
	}

	err = m.SetSummary(testCtx(),"This is the investigation summary")
	if err != nil {
		t.Fatalf("SetSummary failed: %v", err)
	}

	c, _ := m.LoadCase(testCtx(),"summary-test")
	if c.Summary != "This is the investigation summary" {
		t.Errorf("expected summary match, got %q", c.Summary)
	}
}

func TestManager_SaveAndLoadPtrCache(t *testing.T) {
	m := newTestManager(t)
	_ = m.EnsureDirectories(testCtx())

	// Save some pointers
	ptrs := []string{"ptr1", "ptr2", "ptr3"}
	err := m.SavePtrCache(testCtx(),ptrs)
	if err != nil {
		t.Fatalf("SavePtrCache failed: %v", err)
	}

	// Load them back
	cache, err := m.LoadPtrCache(testCtx())
	if err != nil {
		t.Fatalf("LoadPtrCache failed: %v", err)
	}

	if len(cache.Ptrs) != 3 {
		t.Errorf("expected 3 pointers, got %d", len(cache.Ptrs))
	}
}

func TestManager_SaveAndLoadPtrCacheWithMetadata(t *testing.T) {
	m := newTestManager(t)
	_ = m.EnsureDirectories(testCtx())

	entries := []PtrEntry{
		{Ptr: "ptr1", SourceURI: "cloudwatch:///log1", Stream: "stream1"},
		{Ptr: "ptr2", SourceURI: "file:///log2", Stream: "stream2"},
	}

	err := m.SavePtrCacheWithMetadata(testCtx(),entries)
	if err != nil {
		t.Fatalf("SavePtrCacheWithMetadata failed: %v", err)
	}

	// Lookup
	entry := m.LookupPtrMetadata(testCtx(),"ptr1")
	if entry == nil {
		t.Fatal("expected to find ptr1")
	}
	if entry.SourceURI != "cloudwatch:///log1" {
		t.Errorf("expected source_uri 'cloudwatch:///log1', got %q", entry.SourceURI)
	}
}

func TestManager_ResolvePtr(t *testing.T) {
	m := newTestManager(t)
	_ = m.EnsureDirectories(testCtx())

	// Save some pointers
	_ = m.SavePtrCache(testCtx(),[]string{"abc123def456", "xyz789mnopqr", "ghi456jkl789"})

	// Resolve by suffix match (ResolvePtr looks for suffix first)
	resolved, err := m.ResolvePtr(testCtx(),"456")
	if err != nil {
		t.Fatalf("ResolvePtr failed: %v", err)
	}
	// Should match "abc123def456" which ends with "456"
	if resolved != "abc123def456" {
		t.Errorf("expected 'abc123def456', got %q", resolved)
	}

	// Resolve by numeric index
	resolved, err = m.ResolvePtr(testCtx(),"2")
	if err != nil {
		t.Fatalf("ResolvePtr failed: %v", err)
	}
	if resolved != "xyz789mnopqr" {
		t.Errorf("expected 'xyz789mnopqr', got %q", resolved)
	}
}

func TestManager_ResolvePtrNotFound(t *testing.T) {
	m := newTestManager(t)
	_ = m.EnsureDirectories(testCtx())

	_ = m.SavePtrCache(testCtx(),[]string{"abc123"})

	_, err := m.ResolvePtr(testCtx(),"xyz")
	if err == nil {
		t.Error("expected error for unmatched prefix")
	}
}

func TestManager_NoActiveCase(t *testing.T) {
	m := newTestManager(t)
	_ = m.EnsureDirectories(testCtx())

	// Operations that require active case should fail gracefully
	err := m.AddNoteToTimeline(testCtx(), "note", "inline")
	if err == nil {
		t.Error("expected error when no active case")
	}

	err = m.MarkLastQuery(testCtx())
	if err == nil {
		t.Error("expected error when no active case")
	}

	err = m.SetSummary(testCtx(), "summary")
	if err == nil {
		t.Error("expected error when no active case")
	}
}

func TestManager_AddEvidenceDuplicate(t *testing.T) {
	m := newTestManager(t)

	_, err := m.CreateCase(testCtx(), "Duplicate Evidence", "dup-evidence")
	if err != nil {
		t.Fatalf("CreateCase failed: %v", err)
	}

	item := EvidenceItem{
		Ptr:       "file:///var/log/app.log#42",
		Message:   "First error",
		Timestamp: time.Now(),
		Stream:    "app.log",
	}

	err = m.AddEvidence(testCtx(), item)
	if err != nil {
		t.Fatalf("first AddEvidence failed: %v", err)
	}

	// Try to add same ptr again
	item.Message = "Second error"
	err = m.AddEvidence(testCtx(), item)
	if err == nil {
		t.Error("expected error for duplicate ptr")
	}
}

func TestManager_AnnotateEvidenceNotFound(t *testing.T) {
	m := newTestManager(t)

	_, err := m.CreateCase(testCtx(), "Annotate Not Found", "annotate-not-found")
	if err != nil {
		t.Fatalf("CreateCase failed: %v", err)
	}

	err = m.AnnotateEvidence(testCtx(), "nonexistent-ptr", "annotation")
	if err == nil {
		t.Error("expected error for nonexistent ptr")
	}
}

func TestManager_GetEvidenceNoActiveCase(t *testing.T) {
	m := newTestManager(t)
	_ = m.EnsureDirectories(testCtx())

	_, err := m.GetEvidence(testCtx())
	if err == nil {
		t.Error("expected error when no active case")
	}
}

func TestManager_CloseCaseNoActiveCase(t *testing.T) {
	m := newTestManager(t)
	_ = m.EnsureDirectories(testCtx())

	err := m.CloseCase(testCtx(), "summary")
	if err == nil {
		t.Error("expected error when no active case")
	}
}

func TestManager_GetTimelineNoActiveCase(t *testing.T) {
	m := newTestManager(t)
	_ = m.EnsureDirectories(testCtx())

	_, err := m.GetTimeline(testCtx(), "", false)
	if err == nil {
		t.Error("expected error when no active case")
	}
}

func TestManager_AddQueryToTimelineNoActiveCase(t *testing.T) {
	m := newTestManager(t)
	_ = m.EnsureDirectories(testCtx())

	// AddQueryToTimeline should silently skip when no active case
	entry := TimelineEntry{Type: "query", Filter: "error"}
	err := m.AddQueryToTimeline(testCtx(), entry)
	if err != nil {
		t.Errorf("AddQueryToTimeline should silently skip when no active case, got: %v", err)
	}
}

func TestManager_ResolvePtrWithMetadata(t *testing.T) {
	m := newTestManager(t)
	_ = m.EnsureDirectories(testCtx())

	entries := []PtrEntry{
		{Ptr: "abc123def456", SourceURI: "cloudwatch:///log1", Stream: "stream1"},
		{Ptr: "xyz789mnopqr", SourceURI: "file:///log2", Stream: "stream2"},
	}
	_ = m.SavePtrCacheWithMetadata(testCtx(), entries)

	// Resolve by suffix
	ptr, meta, err := m.ResolvePtrWithMetadata(testCtx(), "456")
	if err != nil {
		t.Fatalf("ResolvePtrWithMetadata failed: %v", err)
	}
	if ptr != "abc123def456" {
		t.Errorf("expected 'abc123def456', got %q", ptr)
	}
	if meta == nil {
		t.Fatal("expected metadata")
	}
	if meta.SourceURI != "cloudwatch:///log1" {
		t.Errorf("expected source URI 'cloudwatch:///log1', got %q", meta.SourceURI)
	}

	// Resolve by numeric index
	ptr, meta, err = m.ResolvePtrWithMetadata(testCtx(), "2")
	if err != nil {
		t.Fatalf("ResolvePtrWithMetadata by index failed: %v", err)
	}
	if ptr != "xyz789mnopqr" {
		t.Errorf("expected 'xyz789mnopqr', got %q", ptr)
	}
	if meta == nil || meta.SourceURI != "file:///log2" {
		t.Error("expected metadata with file source")
	}
}

func TestManager_ResolvePtrWithMetadata_FullPointer(t *testing.T) {
	m := newTestManager(t)
	_ = m.EnsureDirectories(testCtx())

	entries := []PtrEntry{
		{Ptr: "abc123def456ghijklmnopqrstuvwxyz1234567890abcdefghijklmnop", SourceURI: "cloudwatch:///log1"},
	}
	_ = m.SavePtrCacheWithMetadata(testCtx(), entries)

	// Full pointer (>50 chars) should be returned as-is with metadata lookup
	fullPtr := "abc123def456ghijklmnopqrstuvwxyz1234567890abcdefghijklmnop"
	ptr, meta, err := m.ResolvePtrWithMetadata(testCtx(), fullPtr)
	if err != nil {
		t.Fatalf("ResolvePtrWithMetadata failed: %v", err)
	}
	if ptr != fullPtr {
		t.Errorf("expected full pointer returned, got %q", ptr)
	}
	if meta != nil && meta.SourceURI != "cloudwatch:///log1" {
		t.Errorf("expected source URI lookup, got %v", meta)
	}
}

func TestManager_ResolvePtrEmptyCache(t *testing.T) {
	m := newTestManager(t)
	_ = m.EnsureDirectories(testCtx())

	_, err := m.ResolvePtr(testCtx(), "abc")
	if err == nil {
		t.Error("expected error for empty cache")
	}
}

func TestManager_ResolvePtrAmbiguous(t *testing.T) {
	m := newTestManager(t)
	_ = m.EnsureDirectories(testCtx())

	// Save pointers that share a suffix
	_ = m.SavePtrCache(testCtx(), []string{"aaa123", "bbb123", "ccc123"})

	_, err := m.ResolvePtr(testCtx(), "123")
	if err == nil {
		t.Error("expected error for ambiguous match")
	}
}

func TestManager_ContextCancellation(t *testing.T) {
	m := newTestManager(t)

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// All operations should respect context cancellation
	err := m.EnsureDirectories(ctx)
	if err == nil {
		t.Error("EnsureDirectories should fail with cancelled context")
	}

	_, err = m.LoadCase(ctx, "test")
	if err == nil {
		t.Error("LoadCase should fail with cancelled context")
	}

	err = m.SaveCase(ctx, &Case{ID: "test"})
	if err == nil {
		t.Error("SaveCase should fail with cancelled context")
	}

	err = m.DeleteCase(ctx, "test")
	if err == nil {
		t.Error("DeleteCase should fail with cancelled context")
	}

	_, err = m.LoadState(ctx)
	if err == nil {
		t.Error("LoadState should fail with cancelled context")
	}

	_, err = m.LoadPtrCache(ctx)
	if err == nil {
		t.Error("LoadPtrCache should fail with cancelled context")
	}
}

func TestManager_OpenCaseNotFound(t *testing.T) {
	m := newTestManager(t)
	_ = m.EnsureDirectories(testCtx())

	_, err := m.OpenCase(testCtx(), "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent case")
	}
}

func TestManager_AddEvidenceNoActiveCase(t *testing.T) {
	m := newTestManager(t)
	_ = m.EnsureDirectories(testCtx())

	item := EvidenceItem{
		Ptr:     "test-ptr",
		Message: "test",
	}
	err := m.AddEvidence(testCtx(), item)
	if err == nil {
		t.Error("expected error when no active case")
	}
}

func TestManager_AnnotateEvidenceNoActiveCase(t *testing.T) {
	m := newTestManager(t)
	_ = m.EnsureDirectories(testCtx())

	err := m.AnnotateEvidence(testCtx(), "ptr", "annotation")
	if err == nil {
		t.Error("expected error when no active case")
	}
}

func TestManager_AddEvidenceAddsToTimeline(t *testing.T) {
	m := newTestManager(t)

	_, err := m.CreateCase(testCtx(), "Evidence Timeline", "evidence-timeline")
	if err != nil {
		t.Fatalf("CreateCase failed: %v", err)
	}

	item := EvidenceItem{
		Ptr:       "test-ptr",
		Message:   "Error message",
		SourceURI: "cloudwatch:///my-log-group",
		Stream:    "stream1",
	}
	err = m.AddEvidence(testCtx(), item)
	if err != nil {
		t.Fatalf("AddEvidence failed: %v", err)
	}

	// Verify it was added to timeline
	c, _ := m.LoadCase(testCtx(), "evidence-timeline")
	if len(c.Timeline) != 1 {
		t.Fatalf("expected 1 timeline entry, got %d", len(c.Timeline))
	}
	if c.Timeline[0].Type != "evidence" {
		t.Errorf("expected type 'evidence', got %q", c.Timeline[0].Type)
	}
	if c.Timeline[0].Source != "cloudwatch:///my-log-group" {
		t.Errorf("expected source URI in timeline, got %q", c.Timeline[0].Source)
	}
}

func TestParseIndex(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"1", 1},
		{"10", 10},
		{"0", 0},   // 0 is not valid (1-based)
		{"-1", 0},  // negative not valid
		{"abc", 0}, // non-numeric
		{"", 0},    // empty
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseIndex(tt.input)
			if got != tt.want {
				t.Errorf("parseIndex(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}
