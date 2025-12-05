package cases

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

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

	c, err := m.CreateCase("Test Investigation", "")
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

	c, err := m.CreateCase("Test Investigation", "custom-id")
	if err != nil {
		t.Fatalf("CreateCase failed: %v", err)
	}

	if c.ID != "custom-id" {
		t.Errorf("expected ID 'custom-id', got %q", c.ID)
	}
}

func TestManager_CreateCaseDuplicate(t *testing.T) {
	m := newTestManager(t)

	_, err := m.CreateCase("Test", "same-id")
	if err != nil {
		t.Fatalf("first CreateCase failed: %v", err)
	}

	_, err = m.CreateCase("Test 2", "same-id")
	if err == nil {
		t.Error("expected error for duplicate ID")
	}
}

func TestManager_LoadCase(t *testing.T) {
	m := newTestManager(t)

	created, err := m.CreateCase("Test Load", "test-load")
	if err != nil {
		t.Fatalf("CreateCase failed: %v", err)
	}

	loaded, err := m.LoadCase("test-load")
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
	_ = m.EnsureDirectories()

	_, err := m.LoadCase("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent case")
	}
}

func TestManager_DeleteCase(t *testing.T) {
	m := newTestManager(t)

	_, err := m.CreateCase("To Delete", "to-delete")
	if err != nil {
		t.Fatalf("CreateCase failed: %v", err)
	}

	err = m.DeleteCase("to-delete")
	if err != nil {
		t.Fatalf("DeleteCase failed: %v", err)
	}

	// Should not be loadable anymore
	_, err = m.LoadCase("to-delete")
	if err == nil {
		t.Error("expected error loading deleted case")
	}
}

func TestManager_DeleteCaseNotFound(t *testing.T) {
	m := newTestManager(t)
	_ = m.EnsureDirectories()

	err := m.DeleteCase("nonexistent")
	if err == nil {
		t.Error("expected error deleting nonexistent case")
	}
}

func TestManager_DeleteActiveCaseClearsState(t *testing.T) {
	m := newTestManager(t)

	_, err := m.CreateCase("Active Case", "active-case")
	if err != nil {
		t.Fatalf("CreateCase failed: %v", err)
	}

	// Verify it's active
	activeCase, err := m.GetActiveCase()
	if err != nil {
		t.Fatalf("GetActiveCase failed: %v", err)
	}
	if activeCase == nil || activeCase.ID != "active-case" {
		t.Error("expected case to be active")
	}

	// Delete the active case
	err = m.DeleteCase("active-case")
	if err != nil {
		t.Fatalf("DeleteCase failed: %v", err)
	}

	// Should have no active case now
	activeCase, err = m.GetActiveCase()
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
	_, _ = m.CreateCase("Case 1", "case-1")
	_, _ = m.CreateCase("Case 2", "case-2")
	_, _ = m.CreateCase("Case 3", "case-3")

	cases, err := m.ListCases()
	if err != nil {
		t.Fatalf("ListCases failed: %v", err)
	}

	if len(cases) != 3 {
		t.Errorf("expected 3 cases, got %d", len(cases))
	}
}

func TestManager_ListCasesEmpty(t *testing.T) {
	m := newTestManager(t)

	cases, err := m.ListCases()
	if err != nil {
		t.Fatalf("ListCases failed: %v", err)
	}

	if len(cases) != 0 {
		t.Errorf("expected 0 cases, got %d", len(cases))
	}
}

func TestManager_ListCasesSkipsInvalidFiles(t *testing.T) {
	m := newTestManager(t)
	_ = m.EnsureDirectories()

	// Create a valid case
	_, _ = m.CreateCase("Valid Case", "valid-case")

	// Create an invalid YAML file
	invalidPath := filepath.Join(m.casesDir, "invalid.yaml")
	_ = os.WriteFile(invalidPath, []byte("not: valid: yaml: {{{{"), 0644)

	// Create a non-YAML file (should be ignored)
	txtPath := filepath.Join(m.casesDir, "readme.txt")
	_ = os.WriteFile(txtPath, []byte("This is not a case"), 0644)

	cases, err := m.ListCases()
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

	_, err := m.CreateCase("Case 1", "case-1")
	if err != nil {
		t.Fatalf("CreateCase failed: %v", err)
	}

	// Should be active after creation
	active, err := m.GetActiveCase()
	if err != nil {
		t.Fatalf("GetActiveCase failed: %v", err)
	}
	if active == nil || active.ID != "case-1" {
		t.Error("expected case-1 to be active")
	}

	// Create another case
	_, err = m.CreateCase("Case 2", "case-2")
	if err != nil {
		t.Fatalf("CreateCase failed: %v", err)
	}

	// Now case-2 should be active
	active, err = m.GetActiveCase()
	if err != nil {
		t.Fatalf("GetActiveCase failed: %v", err)
	}
	if active == nil || active.ID != "case-2" {
		t.Error("expected case-2 to be active")
	}

	// Switch back to case-1
	err = m.SetActiveCase("case-1")
	if err != nil {
		t.Fatalf("SetActiveCase failed: %v", err)
	}

	active, err = m.GetActiveCase()
	if err != nil {
		t.Fatalf("GetActiveCase failed: %v", err)
	}
	if active == nil || active.ID != "case-1" {
		t.Error("expected case-1 to be active after switch")
	}
}

func TestManager_ClearActiveCase(t *testing.T) {
	m := newTestManager(t)

	_, _ = m.CreateCase("Test Case", "test-case")

	err := m.ClearActiveCase()
	if err != nil {
		t.Fatalf("ClearActiveCase failed: %v", err)
	}

	active, _ := m.GetActiveCase()
	if active != nil {
		t.Error("expected no active case after clear")
	}
}

func TestManager_CloseCase(t *testing.T) {
	m := newTestManager(t)

	_, err := m.CreateCase("Open Case", "open-case")
	if err != nil {
		t.Fatalf("CreateCase failed: %v", err)
	}

	err = m.CloseCase("Investigation complete")
	if err != nil {
		t.Fatalf("CloseCase failed: %v", err)
	}

	c, err := m.LoadCase("open-case")
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

	_, err := m.CreateCase("Timeline Test", "timeline-test")
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

	err = m.AddQueryToTimeline(entry)
	if err != nil {
		t.Fatalf("AddQueryToTimeline failed: %v", err)
	}

	c, err := m.LoadCase("timeline-test")
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

	_, err := m.CreateCase("Evidence Test", "evidence-test")
	if err != nil {
		t.Fatalf("CreateCase failed: %v", err)
	}

	item := EvidenceItem{
		Ptr:       "file:///var/log/app.log#42",
		Message:   "Error occurred",
		Timestamp: time.Now(),
		Stream:    "app.log",
	}

	err = m.AddEvidence(item)
	if err != nil {
		t.Fatalf("AddEvidence failed: %v", err)
	}

	evidence, err := m.GetEvidence()
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

	_, err := m.CreateCase("Annotate Test", "annotate-test")
	if err != nil {
		t.Fatalf("CreateCase failed: %v", err)
	}

	item := EvidenceItem{
		Ptr:       "file:///var/log/app.log#42",
		Message:   "Error occurred",
		Timestamp: time.Now(),
		Stream:    "app.log",
	}

	err = m.AddEvidence(item)
	if err != nil {
		t.Fatalf("AddEvidence failed: %v", err)
	}

	err = m.AnnotateEvidence("file:///var/log/app.log#42", "This is the root cause")
	if err != nil {
		t.Fatalf("AnnotateEvidence failed: %v", err)
	}

	evidence, _ := m.GetEvidence()
	if evidence[0].Annotation != "This is the root cause" {
		t.Errorf("expected annotation 'This is the root cause', got %q", evidence[0].Annotation)
	}
}

func TestManager_GetCaseIDs(t *testing.T) {
	m := newTestManager(t)

	_, _ = m.CreateCase("Case A", "case-a")
	_, _ = m.CreateCase("Case B", "case-b")

	ids := m.GetCaseIDs()

	if len(ids) != 2 {
		t.Errorf("expected 2 IDs, got %d", len(ids))
	}
}

func TestManager_StateFileCorrupted(t *testing.T) {
	m := newTestManager(t)
	_ = m.EnsureDirectories()

	// Write corrupted state file
	_ = os.WriteFile(m.stateFile, []byte("{{{{not yaml"), 0644)

	// LoadState should return error
	_, err := m.LoadState()
	if err == nil {
		t.Error("expected error for corrupted state file")
	}
}

func TestManager_DeleteCaseWithCorruptedState(t *testing.T) {
	m := newTestManager(t)

	// Create a case first
	_, err := m.CreateCase("Test", "test-case")
	if err != nil {
		t.Fatalf("CreateCase failed: %v", err)
	}

	// Corrupt the state file
	_ = os.WriteFile(m.stateFile, []byte("{{{{not yaml"), 0644)

	// Delete should still work (logs warning but doesn't fail)
	err = m.DeleteCase("test-case")
	if err != nil {
		t.Fatalf("DeleteCase should succeed even with corrupted state: %v", err)
	}

	// Case should be deleted
	_, err = m.LoadCase("test-case")
	if err == nil {
		t.Error("expected case to be deleted")
	}
}
