// Package cases provides case management for log investigations.
package cases

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/jmurray2011/clew/internal/logging"
	"gopkg.in/yaml.v3"
)

// Pre-compiled regexes (avoids repeated compilation)
var (
	multiHyphenRe = regexp.MustCompile(`-+`)
	datePatternRe = regexp.MustCompile(`\d{4}-\d{2}-\d{2}`)
)

// CaseStatus represents the status of an investigation case.
type CaseStatus string

const (
	StatusActive CaseStatus = "active"
	StatusClosed CaseStatus = "closed"
)

// Case represents an investigation case.
type Case struct {
	ID        string     `yaml:"id"`
	Title     string     `yaml:"title"`
	Created   time.Time  `yaml:"created"`
	Updated   time.Time  `yaml:"updated"`
	Status    CaseStatus `yaml:"status"`
	Summary   string     `yaml:"summary,omitempty"`
	Timeline  []TimelineEntry `yaml:"timeline,omitempty"`
	Evidence  []EvidenceItem  `yaml:"evidence,omitempty"`
}

// TimelineEntry represents an entry in the case timeline.
type TimelineEntry struct {
	Timestamp   time.Time `yaml:"ts"`
	Type        string    `yaml:"type"`                   // query, note, evidence
	SourceURI   string    `yaml:"source_uri,omitempty"`   // Generic source URI (cloudwatch://, file://, etc.)
	SourceType  string    `yaml:"source_type,omitempty"`  // cloudwatch, local, s3
	Profile     string    `yaml:"profile,omitempty"`      // AWS profile used (local reference)
	AccountID   string    `yaml:"account_id,omitempty"`   // AWS account ID (universal identifier)
	Command     string    `yaml:"command,omitempty"`      // for queries
	LogGroup    string    `yaml:"log_group,omitempty"`    // Deprecated: use SourceURI
	Filter      string    `yaml:"filter,omitempty"`
	Query       string    `yaml:"query,omitempty"`
	StartTime   time.Time `yaml:"start_time,omitempty"`
	EndTime     time.Time `yaml:"end_time,omitempty"`
	Results     int       `yaml:"results,omitempty"`
	Marked      bool      `yaml:"marked,omitempty"`
	Content     string    `yaml:"content,omitempty"`      // for notes
	Source      string    `yaml:"source,omitempty"`       // inline, file, editor (for notes)
}

// EvidenceItem represents a log entry saved as evidence.
type EvidenceItem struct {
	Ptr         string            `yaml:"ptr"`
	Message     string            `yaml:"message"`
	Timestamp   time.Time         `yaml:"timestamp"`
	SourceURI   string            `yaml:"source_uri,omitempty"` // Generic source URI
	SourceType  string            `yaml:"source_type,omitempty"` // cloudwatch, local, s3
	Stream      string            `yaml:"stream"`               // Log stream name or filename
	LogGroup    string            `yaml:"log_group,omitempty"`  // Deprecated: use SourceURI
	LogStream   string            `yaml:"log_stream,omitempty"` // Deprecated: use Stream
	Profile     string            `yaml:"profile,omitempty"`    // AWS profile used (local reference)
	AccountID   string            `yaml:"account_id,omitempty"` // AWS account ID (universal identifier)
	CollectedAt time.Time         `yaml:"collected_at"`
	Annotation  string            `yaml:"annotation,omitempty"`
	RawFields   map[string]string `yaml:"raw_fields,omitempty"` // Full log record fields for auditing
}

// State represents the global clew state (active case, etc.)
type State struct {
	ActiveCase string `yaml:"active_case,omitempty"`
}

// Manager handles case operations.
type Manager struct {
	casesDir  string
	stateFile string
}

// NewManager creates a new case manager.
func NewManager() (*Manager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	clewDir := filepath.Join(home, ".clew")
	casesDir := filepath.Join(clewDir, "cases")
	stateFile := filepath.Join(clewDir, "state.yaml")

	return &Manager{
		casesDir:  casesDir,
		stateFile: stateFile,
	}, nil
}

// EnsureDirectories creates the necessary directories if they don't exist.
func (m *Manager) EnsureDirectories(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(m.casesDir, 0755); err != nil {
		return fmt.Errorf("failed to create cases directory: %w", err)
	}
	return nil
}

// GenerateSlug creates a URL-safe slug from a title.
func GenerateSlug(title string) string {
	// Convert to lowercase
	slug := strings.ToLower(title)

	// Replace spaces and underscores with hyphens
	slug = strings.ReplaceAll(slug, " ", "-")
	slug = strings.ReplaceAll(slug, "_", "-")

	// Remove any character that isn't alphanumeric or hyphen
	var result strings.Builder
	for _, r := range slug {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' {
			result.WriteRune(r)
		}
	}
	slug = result.String()

	// Collapse multiple hyphens using pre-compiled regex
	slug = multiHyphenRe.ReplaceAllString(slug, "-")

	// Trim leading/trailing hyphens
	slug = strings.Trim(slug, "-")

	// Add date suffix if not already present
	if !containsDate(slug) {
		slug = fmt.Sprintf("%s-%s", slug, time.Now().Format("2006-01-02"))
	}

	return slug
}

// containsDate checks if the string contains a date pattern.
func containsDate(s string) bool {
	return datePatternRe.MatchString(s)
}

// CreateCase creates a new case with the given title.
func (m *Manager) CreateCase(ctx context.Context, title, customID string) (*Case, error) {
	if err := m.EnsureDirectories(ctx); err != nil {
		return nil, err
	}

	id := customID
	if id == "" {
		id = GenerateSlug(title)
	}

	// Check if case already exists
	casePath := m.getCasePath(id)
	if _, err := os.Stat(casePath); err == nil {
		return nil, fmt.Errorf("case %q already exists", id)
	}

	now := time.Now()
	c := &Case{
		ID:      id,
		Title:   title,
		Created: now,
		Updated: now,
		Status:  StatusActive,
	}

	if err := m.SaveCase(ctx, c); err != nil {
		return nil, err
	}

	// Set as active case
	if err := m.SetActiveCase(ctx, id); err != nil {
		return nil, err
	}

	return c, nil
}

// SaveCase saves a case to disk.
func (m *Manager) SaveCase(ctx context.Context, c *Case) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.Updated = time.Now()

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal case: %w", err)
	}

	casePath := m.getCasePath(c.ID)
	if err := os.WriteFile(casePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write case file: %w", err)
	}

	return nil
}

// LoadCase loads a case from disk.
func (m *Manager) LoadCase(ctx context.Context, id string) (*Case, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	casePath := m.getCasePath(id)
	data, err := os.ReadFile(casePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("case %q not found", id)
		}
		return nil, fmt.Errorf("failed to read case file: %w", err)
	}

	var c Case
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("failed to parse case file: %w", err)
	}

	return &c, nil
}

// DeleteCase removes a case from disk.
func (m *Manager) DeleteCase(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	casePath := m.getCasePath(id)
	if _, err := os.Stat(casePath); os.IsNotExist(err) {
		return fmt.Errorf("case %q not found", id)
	}

	// If this is the active case, clear it
	state, err := m.LoadState(ctx)
	if err != nil {
		logging.Warn("Could not load state while deleting case: %v", err)
	} else if state != nil && state.ActiveCase == id {
		if err := m.ClearActiveCase(ctx); err != nil {
			return err
		}
	}

	if err := os.Remove(casePath); err != nil {
		return fmt.Errorf("failed to delete case: %w", err)
	}

	return nil
}

// ListCases returns all cases.
func (m *Manager) ListCases(ctx context.Context) ([]*Case, error) {
	if err := m.EnsureDirectories(ctx); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(m.casesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read cases directory: %w", err)
	}

	var cases []*Case
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		id := strings.TrimSuffix(entry.Name(), ".yaml")
		c, err := m.LoadCase(ctx, id)
		if err != nil {
			logging.Warn("Skipping invalid case file %s: %v", entry.Name(), err)
			continue
		}
		cases = append(cases, c)
	}

	return cases, nil
}

// GetActiveCase returns the currently active case, if any.
func (m *Manager) GetActiveCase(ctx context.Context) (*Case, error) {
	state, err := m.LoadState(ctx)
	if err != nil {
		return nil, nil // No state file is OK
	}

	if state.ActiveCase == "" {
		return nil, nil
	}

	return m.LoadCase(ctx, state.ActiveCase)
}

// SetActiveCase sets the active case.
func (m *Manager) SetActiveCase(ctx context.Context, id string) error {
	state := &State{ActiveCase: id}
	return m.SaveState(ctx, state)
}

// ClearActiveCase clears the active case.
func (m *Manager) ClearActiveCase(ctx context.Context) error {
	state := &State{}
	return m.SaveState(ctx, state)
}

// LoadState loads the global state.
func (m *Manager) LoadState(ctx context.Context) (*State, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(m.stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return &State{}, nil
		}
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	var state State
	if err := yaml.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse state file: %w", err)
	}

	return &state, nil
}

// SaveState saves the global state.
func (m *Manager) SaveState(ctx context.Context, state *State) error {
	if err := m.EnsureDirectories(ctx); err != nil {
		return err
	}

	data, err := yaml.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	if err := os.WriteFile(m.stateFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	return nil
}

// getCasePath returns the path to a case file.
func (m *Manager) getCasePath(id string) string {
	return filepath.Join(m.casesDir, id+".yaml")
}

// CloseCase closes the active case.
func (m *Manager) CloseCase(ctx context.Context, summary string) error {
	c, err := m.GetActiveCase(ctx)
	if err != nil {
		return err
	}
	if c == nil {
		return fmt.Errorf("no active case")
	}

	c.Status = StatusClosed
	if summary != "" {
		c.Summary = summary
	}

	if err := m.SaveCase(ctx, c); err != nil {
		return err
	}

	return m.ClearActiveCase(ctx)
}

// OpenCase opens/switches to an existing case.
func (m *Manager) OpenCase(ctx context.Context, id string) (*Case, error) {
	c, err := m.LoadCase(ctx, id)
	if err != nil {
		return nil, err
	}

	if err := m.SetActiveCase(ctx, id); err != nil {
		return nil, err
	}

	return c, nil
}

// GetCaseIDs returns a list of all case IDs (for tab completion).
func (m *Manager) GetCaseIDs(ctx context.Context) []string {
	cases, err := m.ListCases(ctx)
	if err != nil {
		return nil
	}

	ids := make([]string, len(cases))
	for i, c := range cases {
		ids[i] = c.ID
	}
	return ids
}

// AddQueryToTimeline adds a query to the active case timeline.
func (m *Manager) AddQueryToTimeline(ctx context.Context, entry TimelineEntry) error {
	c, err := m.GetActiveCase(ctx)
	if err != nil {
		return err
	}
	if c == nil {
		return nil // No active case, silently skip
	}

	entry.Timestamp = time.Now()
	entry.Type = "query"
	c.Timeline = append(c.Timeline, entry)

	return m.SaveCase(ctx, c)
}

// AddNoteToTimeline adds a note to the active case timeline.
func (m *Manager) AddNoteToTimeline(ctx context.Context, content, source string) error {
	c, err := m.GetActiveCase(ctx)
	if err != nil {
		return err
	}
	if c == nil {
		return fmt.Errorf("no active case")
	}

	entry := TimelineEntry{
		Timestamp: time.Now(),
		Type:      "note",
		Content:   content,
		Source:    source,
	}
	c.Timeline = append(c.Timeline, entry)

	return m.SaveCase(ctx, c)
}

// MarkLastQuery marks the most recent query in the timeline as significant.
func (m *Manager) MarkLastQuery(ctx context.Context) error {
	c, err := m.GetActiveCase(ctx)
	if err != nil {
		return err
	}
	if c == nil {
		return fmt.Errorf("no active case")
	}

	// Find the last query entry
	for i := len(c.Timeline) - 1; i >= 0; i-- {
		if c.Timeline[i].Type == "query" {
			c.Timeline[i].Marked = true
			return m.SaveCase(ctx, c)
		}
	}

	return fmt.Errorf("no query found in timeline")
}

// GetTimeline returns the timeline entries for the active case.
func (m *Manager) GetTimeline(ctx context.Context, filterType string, markedOnly bool) ([]TimelineEntry, error) {
	c, err := m.GetActiveCase(ctx)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, fmt.Errorf("no active case")
	}

	var entries []TimelineEntry
	for _, e := range c.Timeline {
		if filterType != "" && e.Type != filterType {
			continue
		}
		if markedOnly && !e.Marked {
			continue
		}
		entries = append(entries, e)
	}

	return entries, nil
}

// SetSummary sets the summary for the active case.
func (m *Manager) SetSummary(ctx context.Context, summary string) error {
	c, err := m.GetActiveCase(ctx)
	if err != nil {
		return err
	}
	if c == nil {
		return fmt.Errorf("no active case")
	}

	c.Summary = summary
	return m.SaveCase(ctx, c)
}

// AddEvidence adds an evidence item to the active case.
func (m *Manager) AddEvidence(ctx context.Context, item EvidenceItem) error {
	c, err := m.GetActiveCase(ctx)
	if err != nil {
		return err
	}
	if c == nil {
		return fmt.Errorf("no active case")
	}

	// Check for duplicate @ptr
	for _, e := range c.Evidence {
		if e.Ptr == item.Ptr {
			return fmt.Errorf("evidence with this @ptr already exists")
		}
	}

	item.CollectedAt = time.Now()
	c.Evidence = append(c.Evidence, item)

	// Also add to timeline
	// Prefer SourceURI over deprecated LogGroup
	sourceDisplay := item.SourceURI
	if sourceDisplay == "" {
		sourceDisplay = item.LogGroup
	}
	entry := TimelineEntry{
		Timestamp: time.Now(),
		Type:      "evidence",
		Content:   item.Message,
		Source:    sourceDisplay,
	}
	c.Timeline = append(c.Timeline, entry)

	return m.SaveCase(ctx, c)
}

// AnnotateEvidence updates the annotation on an evidence item.
func (m *Manager) AnnotateEvidence(ctx context.Context, ptr, annotation string) error {
	c, err := m.GetActiveCase(ctx)
	if err != nil {
		return err
	}
	if c == nil {
		return fmt.Errorf("no active case")
	}

	for i := range c.Evidence {
		if c.Evidence[i].Ptr == ptr {
			c.Evidence[i].Annotation = annotation
			return m.SaveCase(ctx, c)
		}
	}

	return fmt.Errorf("evidence with @ptr not found")
}

// GetEvidence returns all evidence for the active case.
func (m *Manager) GetEvidence(ctx context.Context) ([]EvidenceItem, error) {
	c, err := m.GetActiveCase(ctx)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, fmt.Errorf("no active case")
	}

	return c.Evidence, nil
}

// PtrEntry holds a @ptr value with its metadata for cross-source support.
type PtrEntry struct {
	Ptr        string `yaml:"ptr"`
	SourceURI  string `yaml:"source_uri,omitempty"`  // Generic source URI
	SourceType string `yaml:"source_type,omitempty"` // cloudwatch, local, s3
	Stream     string `yaml:"stream,omitempty"`      // Log stream name or filename
	LogGroup   string `yaml:"log_group,omitempty"`   // Deprecated: use SourceURI
	LogStream  string `yaml:"log_stream,omitempty"`  // Deprecated: use Stream
	Profile    string `yaml:"profile,omitempty"`     // AWS profile used (local reference)
	AccountID  string `yaml:"account_id,omitempty"`  // AWS account ID (universal identifier)
}

// PtrCache holds recently seen @ptr values from queries.
type PtrCache struct {
	Ptrs      []string   `yaml:"ptrs,omitempty"`    // Legacy: simple ptr list
	Entries   []PtrEntry `yaml:"entries,omitempty"` // New: ptrs with metadata
	UpdatedAt time.Time  `yaml:"updated_at"`
}

// getPtrCachePath returns the path to the pointer cache file.
func (m *Manager) getPtrCachePath() string {
	return filepath.Join(filepath.Dir(m.casesDir), "ptr_cache.yaml")
}

// SavePtrCache saves a list of @ptr values from query results (legacy).
func (m *Manager) SavePtrCache(ctx context.Context, ptrs []string) error {
	if err := m.EnsureDirectories(ctx); err != nil {
		return err
	}

	cache := PtrCache{
		Ptrs:      ptrs,
		UpdatedAt: time.Now(),
	}

	data, err := yaml.Marshal(cache)
	if err != nil {
		return fmt.Errorf("failed to marshal ptr cache: %w", err)
	}

	if err := os.WriteFile(m.getPtrCachePath(), data, 0644); err != nil {
		return fmt.Errorf("failed to write ptr cache: %w", err)
	}

	return nil
}

// SavePtrCacheWithMetadata saves @ptr values with their log group and profile metadata.
// This enables cross-account evidence collection by preserving the source context.
func (m *Manager) SavePtrCacheWithMetadata(ctx context.Context, entries []PtrEntry) error {
	if err := m.EnsureDirectories(ctx); err != nil {
		return err
	}

	// Also maintain legacy Ptrs list for backward compatibility
	ptrs := make([]string, len(entries))
	for i, e := range entries {
		ptrs[i] = e.Ptr
	}

	cache := PtrCache{
		Ptrs:      ptrs,
		Entries:   entries,
		UpdatedAt: time.Now(),
	}

	data, err := yaml.Marshal(cache)
	if err != nil {
		return fmt.Errorf("failed to marshal ptr cache: %w", err)
	}

	if err := os.WriteFile(m.getPtrCachePath(), data, 0644); err != nil {
		return fmt.Errorf("failed to write ptr cache: %w", err)
	}

	return nil
}

// LoadPtrCache loads the cached @ptr values.
func (m *Manager) LoadPtrCache(ctx context.Context) (*PtrCache, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(m.getPtrCachePath())
	if err != nil {
		if os.IsNotExist(err) {
			return &PtrCache{}, nil
		}
		return nil, fmt.Errorf("failed to read ptr cache: %w", err)
	}

	var cache PtrCache
	if err := yaml.Unmarshal(data, &cache); err != nil {
		return nil, fmt.Errorf("failed to parse ptr cache: %w", err)
	}

	return &cache, nil
}

// LookupPtrMetadata finds the metadata for a given @ptr value.
// Returns nil if no metadata is found (e.g., from legacy cache or direct ptr input).
func (m *Manager) LookupPtrMetadata(ctx context.Context, ptr string) *PtrEntry {
	cache, err := m.LoadPtrCache(ctx)
	if err != nil || cache == nil {
		return nil
	}

	// Search in entries for exact match
	for _, entry := range cache.Entries {
		if entry.Ptr == ptr {
			return &entry
		}
	}

	return nil
}

// ResolvePtrWithMetadata resolves a short @ptr prefix to the full pointer and its metadata.
// Returns the full pointer and metadata if exactly one match is found.
// Supports numeric index (1, 2, 3...) to select from cached results.
func (m *Manager) ResolvePtrWithMetadata(ctx context.Context, prefix string) (string, *PtrEntry, error) {
	// If it looks like a full pointer (long enough), look up metadata directly
	if len(prefix) > 50 {
		entry := m.LookupPtrMetadata(ctx, prefix)
		return prefix, entry, nil
	}

	cache, err := m.LoadPtrCache(ctx)
	if err != nil {
		return "", nil, err
	}

	if len(cache.Ptrs) == 0 && len(cache.Entries) == 0 {
		return "", nil, fmt.Errorf("no cached pointers from recent queries. Run a query first")
	}

	// Check if prefix is a numeric index (1-based)
	if idx := parseIndex(prefix); idx > 0 {
		// Prefer entries if available
		if idx <= len(cache.Entries) {
			entry := cache.Entries[idx-1]
			return entry.Ptr, &entry, nil
		}
		// Fall back to legacy ptrs
		if idx <= len(cache.Ptrs) {
			return cache.Ptrs[idx-1], nil, nil
		}
	}

	// Strip leading @ if present (user may type @UN4G or just UN4G)
	searchTerm := strings.TrimPrefix(prefix, "@")

	// Try suffix match (display shows last 12 chars as @suffix)
	if len(cache.Entries) > 0 {
		var matches []PtrEntry
		for _, entry := range cache.Entries {
			if strings.HasSuffix(entry.Ptr, searchTerm) {
				matches = append(matches, entry)
			}
		}

		if len(matches) == 1 {
			return matches[0].Ptr, &matches[0], nil
		}
		if len(matches) > 1 {
			return "", nil, fmt.Errorf("ambiguous @ptr suffix %q matches %d entries", prefix, len(matches))
		}
	}

	// Fall back to legacy ptrs
	var matches []string
	for _, ptr := range cache.Ptrs {
		if strings.HasSuffix(ptr, searchTerm) {
			matches = append(matches, ptr)
		}
	}

	if len(matches) == 1 {
		return matches[0], nil, nil
	}
	if len(matches) > 1 {
		return "", nil, fmt.Errorf("ambiguous @ptr suffix %q matches %d entries", prefix, len(matches))
	}

	return "", nil, fmt.Errorf("no @ptr found matching %q", prefix)
}

// ResolvePtr resolves a short @ptr suffix to the full pointer.
// Returns the full pointer if exactly one match is found.
// Supports numeric index (1, 2, 3...) to select from cached results.
// Returns an error if no matches or multiple matches are found.
func (m *Manager) ResolvePtr(ctx context.Context, prefix string) (string, error) {
	// If it looks like a full pointer (long enough), just return it
	if len(prefix) > 50 {
		return prefix, nil
	}

	cache, err := m.LoadPtrCache(ctx)
	if err != nil {
		return "", err
	}

	if len(cache.Ptrs) == 0 {
		return "", fmt.Errorf("no cached pointers from recent queries. Run a query first")
	}

	// Check if prefix is a numeric index (1-based)
	if idx := parseIndex(prefix); idx > 0 && idx <= len(cache.Ptrs) {
		return cache.Ptrs[idx-1], nil
	}

	// Strip leading @ if present (user may type @UN4G or just UN4G)
	searchTerm := strings.TrimPrefix(prefix, "@")

	// Try suffix match first (display shows last 12 chars as @suffix)
	var matches []string
	for _, ptr := range cache.Ptrs {
		if strings.HasSuffix(ptr, searchTerm) {
			matches = append(matches, ptr)
		}
	}

	// If no suffix matches, try substring match
	if len(matches) == 0 {
		for _, ptr := range cache.Ptrs {
			if strings.Contains(ptr, searchTerm) {
				matches = append(matches, ptr)
			}
		}
	}

	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no @ptr found matching %q", prefix)
	case 1:
		return matches[0], nil
	default:
		// Show indexed list with the END of each pointer (where they differ)
		var hints []string
		for i, m := range matches {
			// Show last 20 chars where the unique bits are
			suffix := m
			if len(suffix) > 20 {
				suffix = "..." + suffix[len(suffix)-20:]
			}
			hints = append(hints, fmt.Sprintf("[%d] %s", i+1, suffix))
		}
		return "", fmt.Errorf("ambiguous: %d pointers match. Use index (1, 2, ...) to select:\n  %s",
			len(matches), strings.Join(hints, "\n  "))
	}
}

// parseIndex parses a string as a 1-based index. Returns 0 if not a valid index.
func parseIndex(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}
