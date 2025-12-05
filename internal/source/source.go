package source

import "context"

// Source is the interface that all log backends must implement.
type Source interface {
	// Query returns log entries matching the given parameters.
	Query(ctx context.Context, params QueryParams) ([]Entry, error)

	// Tail streams log events in real-time. The returned channel is closed
	// when the context is cancelled or an error occurs.
	Tail(ctx context.Context, params TailParams) (<-chan Event, error)

	// GetRecord retrieves a single log entry by its pointer.
	GetRecord(ctx context.Context, ptr string) (*Entry, error)

	// FetchContext retrieves context lines around a log entry.
	// Returns (beforeLines, afterLines, error).
	FetchContext(ctx context.Context, entry Entry, before, after int) ([]Event, []Event, error)

	// ListStreams returns available log streams or files within this source.
	ListStreams(ctx context.Context) ([]StreamInfo, error)

	// Type returns the source type identifier (e.g., "cloudwatch", "local", "s3").
	Type() string

	// Metadata returns source metadata for caching and evidence collection.
	Metadata() SourceMetadata

	// Close releases any resources held by the source.
	Close() error
}
