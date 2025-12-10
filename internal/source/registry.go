package source

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	clerrors "github.com/jmurray2011/clew/internal/errors"
)

// SourceOpener is a function that opens a source from a parsed URL.
type SourceOpener func(u *url.URL, opts OpenOptions) (Source, error)

// OpenOptions provides default values for source configuration.
// These can be overridden by URI query parameters or alias config.
type OpenOptions struct {
	Profile string // Default AWS profile
	Region  string // Default AWS region
}

// registry holds registered source openers by scheme.
var registry = make(map[string]SourceOpener)

// Register adds a source opener for the given URI scheme.
// This should be called during init() by each source implementation.
func Register(scheme string, opener SourceOpener) {
	registry[scheme] = opener
}

// Open parses a URI and returns the appropriate Source.
// For CloudWatch sources, use OpenWithOptions to specify default profile/region.
func Open(uri string) (Source, error) {
	return OpenWithOptions(uri, OpenOptions{})
}

// OpenWithOptions parses a URI and returns the appropriate Source with default options.
// Supports:
//   - cloudwatch:///log-group (uses -p profile flag)
//   - file:///path/to/file (or bare paths like /var/log/app.log)
//   - s3://bucket/prefix
//   - @alias (resolved from config)
func OpenWithOptions(uri string, opts OpenOptions) (Source, error) {
	// Handle bare paths as file://
	if strings.HasPrefix(uri, "/") || strings.HasPrefix(uri, "./") || strings.HasPrefix(uri, "../") || strings.HasPrefix(uri, "~") {
		// Resolve to absolute path to avoid url.Parse misinterpreting relative paths
		path := expandPath(uri)
		uri = "file://" + path
	}

	// Handle @alias references
	if strings.HasPrefix(uri, "@") {
		return OpenAliasWithOptions(uri[1:], opts)
	}

	// Detect common URI mistakes
	if err := validateURISyntax(uri); err != nil {
		return nil, err
	}

	parsed, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("invalid source URI %q: %w", uri, err)
	}

	opener, ok := registry[parsed.Scheme]
	if !ok {
		return nil, fmt.Errorf("unknown source scheme: %s (available: %s)", parsed.Scheme, availableSchemes())
	}

	return opener(parsed, opts)
}

// validateURISyntax checks for common URI mistakes and returns helpful errors.
func validateURISyntax(uri string) error {
	// Check for @ used instead of ? for query parameters
	// Pattern: scheme:///path@key=value (should be scheme:///path?key=value)
	if idx := strings.Index(uri, "://"); idx > 0 {
		rest := uri[idx+3:]
		// Look for @key=value pattern (not at the start, which would be user@host)
		if atIdx := strings.Index(rest, "@"); atIdx > 0 {
			afterAt := rest[atIdx+1:]
			// Check if it looks like a query parameter (contains =)
			if strings.Contains(afterAt, "=") && !strings.Contains(rest[:atIdx], "?") {
				return fmt.Errorf("invalid URI %q: use '?' for query parameters, not '@'", uri)
			}
		}
	}

	// Check for missing scheme (common: forgetting cloudwatch://)
	if strings.HasPrefix(uri, "///") {
		return fmt.Errorf("invalid URI %q: missing scheme (e.g., cloudwatch:///log-group)", uri)
	}

	return nil
}

// OpenAlias resolves a config alias to a Source.
func OpenAlias(name string) (Source, error) {
	return OpenAliasWithOptions(name, OpenOptions{})
}

// OpenAliasWithOptions resolves a config alias to a Source with default options.
func OpenAliasWithOptions(name string, opts OpenOptions) (Source, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	alias, ok := cfg.Sources[name]
	if !ok {
		available := make([]string, 0, len(cfg.Sources))
		for k := range cfg.Sources {
			available = append(available, k)
		}
		return nil, clerrors.SourceNotFoundError("@"+name, available)
	}

	return OpenWithOptions(alias.URI, opts)
}

// OpenFromPtr opens a source capable of retrieving the given pointer.
// This is used by the `get` command to fetch a single record.
func OpenFromPtr(ptr string, metadata *SourceMetadata) (Source, error) {
	ptrType := ParsePtrType(ptr)

	switch ptrType {
	case PtrTypeLocal:
		info, ok := ParseLocalPtr(ptr)
		if !ok {
			return nil, fmt.Errorf("invalid local pointer: %s", ptr)
		}
		return Open("file://" + info.FilePath)

	case PtrTypeCloudWatch:
		// CloudWatch pointers need metadata to know profile/region
		if metadata == nil {
			return nil, fmt.Errorf("CloudWatch pointer requires cached metadata (profile/region)")
		}
		// Build URI and options from metadata
		uri := fmt.Sprintf("cloudwatch://%s", metadata.URI)
		opts := OpenOptions{
			Profile: metadata.Profile,
			Region:  metadata.Region,
		}
		return OpenWithOptions(uri, opts)

	case PtrTypeS3:
		info, ok := ParseS3Ptr(ptr)
		if !ok {
			return nil, fmt.Errorf("invalid S3 pointer: %s", ptr)
		}
		return Open(fmt.Sprintf("s3://%s/%s", info.Bucket, info.Key))

	default:
		return nil, fmt.Errorf("unknown pointer type: %s", ptr)
	}
}

func availableSchemes() string {
	schemes := make([]string, 0, len(registry))
	for s := range registry {
		schemes = append(schemes, s)
	}
	if len(schemes) == 0 {
		return "(none registered)"
	}
	return strings.Join(schemes, ", ")
}

// expandPath resolves ~ to home directory and converts relative paths to absolute.
func expandPath(path string) string {
	// Expand ~ to home directory
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[2:])
		}
	} else if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			path = home
		}
	}

	// Convert relative paths to absolute
	if !filepath.IsAbs(path) {
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
	}

	return path
}
