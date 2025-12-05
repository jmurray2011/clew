package cmd

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/jmurray2011/clew/internal/cases"
	"github.com/jmurray2011/clew/internal/output"
	"github.com/jmurray2011/clew/internal/source"

	"github.com/spf13/cobra"
)

var getCmd = &cobra.Command{
	Use:   "get <ptr>",
	Short: "Get a log event by pointer",
	Long: `Fetch the full details of a log event using its pointer.

Pointers are unique identifiers returned in query results. They can be:
  - A short number (e.g., "45") referencing a recent query result
  - A CloudWatch @ptr string (base64-encoded)
  - A file pointer (e.g., "file:///path/to/file#linenum")

Examples:
  # Get by short reference from recent query
  clew get 45

  # Get a CloudWatch log event by @ptr
  clew get "CmAKJgoiNjI3..."

  # Get a local file line
  clew get "file:///var/log/app.log#1542"

  # Output as JSON for parsing
  clew get 45 -o json`,
	Args: cobra.ExactArgs(1),
	RunE: runGet,
}

func init() {
	rootCmd.AddCommand(getCmd)
}

func runGet(cmd *cobra.Command, args []string) error {
	ptrArg := args[0]

	// Resolve short pointer reference from cache if it's a number
	ptr, metadata, err := resolvePtr(ptrArg)
	if err != nil {
		return err
	}

	// Open source from pointer
	src, err := source.OpenFromPtr(ptr, metadata)
	if err != nil {
		return fmt.Errorf("failed to open source for pointer: %w", err)
	}
	defer func() { _ = src.Close() }()

	ctx := context.Background()
	Debugf("Fetching record with pointer: %s", ptr)
	Debugf("Source type: %s", src.Type())

	entry, err := src.GetRecord(ctx, ptr)
	if err != nil {
		return err
	}

	formatter := output.NewFormatter(getOutputFormat(), os.Stdout)
	return formatter.FormatEntries([]source.Entry{*entry})
}

// resolvePtr resolves a pointer argument to a full pointer and optional metadata.
// If ptrArg is a number, it looks up the pointer in the cached results.
// Otherwise, it returns the pointer as-is.
func resolvePtr(ptrArg string) (string, *source.SourceMetadata, error) {
	// Try to parse as a number (short reference)
	if num, err := strconv.Atoi(ptrArg); err == nil && num > 0 {
		return resolvePtrFromCache(num)
	}

	// Check for short CloudWatch suffix (last 12 chars)
	if len(ptrArg) <= 12 && !strings.HasPrefix(ptrArg, "file://") && !strings.HasPrefix(ptrArg, "s3://") {
		return resolvePtrBySuffix(ptrArg)
	}

	// It's a full pointer
	return ptrArg, nil, nil
}

// resolvePtrFromCache looks up pointer #N from the cached query results.
func resolvePtrFromCache(num int) (string, *source.SourceMetadata, error) {
	mgr, err := cases.NewManager()
	if err != nil {
		return "", nil, fmt.Errorf("failed to access pointer cache: %w", err)
	}

	cache, err := mgr.LoadPtrCache()
	if err != nil {
		return "", nil, fmt.Errorf("failed to load pointer cache: %w", err)
	}

	entries := cache.Entries

	// Adjust for 1-based indexing
	idx := num - 1
	if idx < 0 || idx >= len(entries) {
		return "", nil, fmt.Errorf("pointer #%d not found (cache has %d entries)", num, len(entries))
	}

	entry := entries[idx]

	// Build metadata from cache entry
	// Prefer new fields, fall back to deprecated fields for backward compat
	uri := entry.SourceURI
	if uri == "" {
		uri = entry.LogGroup
	}
	metadata := &source.SourceMetadata{
		Type:      entry.SourceType,
		URI:       uri,
		Profile:   entry.Profile,
		AccountID: entry.AccountID,
	}

	return entry.Ptr, metadata, nil
}

// resolvePtrBySuffix finds a cached pointer by its suffix (last N chars).
func resolvePtrBySuffix(suffix string) (string, *source.SourceMetadata, error) {
	mgr, err := cases.NewManager()
	if err != nil {
		return "", nil, fmt.Errorf("failed to access pointer cache: %w", err)
	}

	cache, err := mgr.LoadPtrCache()
	if err != nil {
		return "", nil, fmt.Errorf("failed to load pointer cache: %w", err)
	}

	var matches []cases.PtrEntry
	for _, entry := range cache.Entries {
		if strings.HasSuffix(entry.Ptr, suffix) {
			matches = append(matches, entry)
		}
	}

	if len(matches) == 0 {
		// Not found in cache - treat as full pointer
		return suffix, nil, nil
	}

	if len(matches) > 1 {
		return "", nil, fmt.Errorf("pointer suffix %q is ambiguous (matches %d entries)", suffix, len(matches))
	}

	entry := matches[0]
	// Prefer new fields, fall back to deprecated fields for backward compat
	uri := entry.SourceURI
	if uri == "" {
		uri = entry.LogGroup
	}
	metadata := &source.SourceMetadata{
		Type:      entry.SourceType,
		URI:       uri,
		Profile:   entry.Profile,
		AccountID: entry.AccountID,
	}

	return entry.Ptr, metadata, nil
}
