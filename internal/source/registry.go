package source

import (
	"fmt"
	"net/url"
	"strings"
)

// SourceOpener is a function that opens a source from a parsed URL.
type SourceOpener func(u *url.URL) (Source, error)

// registry holds registered source openers by scheme.
var registry = make(map[string]SourceOpener)

// Register adds a source opener for the given URI scheme.
// This should be called during init() by each source implementation.
func Register(scheme string, opener SourceOpener) {
	registry[scheme] = opener
}

// Open parses a URI and returns the appropriate Source.
// Supports:
//   - cloudwatch:///log-group?profile=x&region=y
//   - file:///path/to/file (or bare paths like /var/log/app.log)
//   - s3://bucket/prefix
//   - @alias (resolved from config)
func Open(uri string) (Source, error) {
	// Handle bare paths as file://
	if strings.HasPrefix(uri, "/") || strings.HasPrefix(uri, "./") || strings.HasPrefix(uri, "../") {
		uri = "file://" + uri
	}

	// Handle @alias references
	if strings.HasPrefix(uri, "@") {
		return OpenAlias(uri[1:])
	}

	parsed, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("invalid source URI %q: %w", uri, err)
	}

	opener, ok := registry[parsed.Scheme]
	if !ok {
		return nil, fmt.Errorf("unknown source scheme: %s (available: %s)", parsed.Scheme, availableSchemes())
	}

	return opener(parsed)
}

// OpenAlias resolves a config alias to a Source.
func OpenAlias(name string) (Source, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	alias, ok := cfg.Sources[name]
	if !ok {
		available := make([]string, 0, len(cfg.Sources))
		for k := range cfg.Sources {
			available = append(available, "@"+k)
		}
		return nil, fmt.Errorf("unknown source alias @%s (available: %s)", name, strings.Join(available, ", "))
	}

	return Open(alias.URI)
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
		// Build URI from metadata
		uri := fmt.Sprintf("cloudwatch://%s", metadata.URI)
		if metadata.Profile != "" || metadata.Region != "" {
			uri += "?"
			params := url.Values{}
			if metadata.Profile != "" {
				params.Set("profile", metadata.Profile)
			}
			if metadata.Region != "" {
				params.Set("region", metadata.Region)
			}
			uri += params.Encode()
		}
		return Open(uri)

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
