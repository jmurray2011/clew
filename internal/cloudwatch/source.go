package cloudwatch

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/jmurray2011/clew/internal/logging"
	"github.com/jmurray2011/clew/internal/source"
	"github.com/jmurray2011/clew/pkg/lru"
)

// Default configuration values
const (
	// DefaultEventChanBuffer is the default buffer size for the event channel
	DefaultEventChanBuffer = 100

	// DefaultLRUCacheCapacity is the default capacity for the LRU dedup cache
	DefaultLRUCacheCapacity = 10000

	// DefaultTailLookback is how far back to start when beginning to tail
	DefaultTailLookback = 5 * time.Second

	// DefaultTailPollInterval is how often to poll for new events when tailing
	DefaultTailPollInterval = 2 * time.Second
)

func init() {
	// Register the cloudwatch scheme with the source registry
	source.Register("cloudwatch", openSource)
}

// Source implements source.Source for AWS CloudWatch Logs.
type Source struct {
	logGroup  string
	client    LogsClient
	profile   string
	region    string
	accountID string
}

// NewSource creates a new CloudWatch log source.
func NewSource(logGroup, profile, region string) (*Source, error) {
	logsClient, err := NewLogsClient(profile, region)
	if err != nil {
		return nil, fmt.Errorf("failed to create CloudWatch Logs client: %w", err)
	}

	// Resolve the actual region from config if not explicitly provided
	resolvedRegion := region
	if resolvedRegion == "" {
		if r, err := GetResolvedRegion(profile, region); err == nil {
			resolvedRegion = r
		} else {
			logging.Debug("Could not determine AWS region: %v", err)
		}
	}

	s := &Source{
		logGroup: logGroup,
		client:   NewClient(logsClient),
		profile:  profile,
		region:   resolvedRegion,
	}

	// Optionally fetch account ID (don't fail if this fails)
	if accountID, err := GetAccountID(profile, region); err == nil {
		s.accountID = accountID
	} else {
		logging.Debug("Could not determine AWS account ID: %v", err)
	}

	return s, nil
}

// NewSourceWithClient creates a new CloudWatch log source with a custom client.
// This is primarily used for testing with mock clients.
func NewSourceWithClient(logGroup string, client LogsClient) *Source {
	return &Source{
		logGroup: logGroup,
		client:   client,
	}
}

// openSource is the SourceOpener for the cloudwatch scheme.
func openSource(u *url.URL) (source.Source, error) {
	logGroup := u.Path
	if logGroup == "" {
		return nil, fmt.Errorf("cloudwatch URI requires a log group path")
	}

	profile := u.Query().Get("profile")
	region := u.Query().Get("region")

	return NewSource(logGroup, profile, region)
}

// Query returns log entries matching the given parameters.
func (s *Source) Query(ctx context.Context, params source.QueryParams) ([]source.Entry, error) {
	// Build CloudWatch Insights query
	query := params.Query
	if query == "" {
		// Build default query with filter
		filterStr := ""
		if params.Filter != nil {
			filterStr = params.Filter.String()
		}
		query = buildInsightsQuery(filterStr, params.Limit)
	}

	cwParams := QueryParams{
		LogGroup:  s.logGroup,
		StartTime: params.StartTime,
		EndTime:   params.EndTime,
		Query:     query,
		Limit:     params.Limit,
	}

	results, err := s.client.RunInsightsQuery(ctx, cwParams)
	if err != nil {
		return nil, err
	}

	// Fetch context if requested
	if params.Context > 0 {
		results, err = s.client.FetchContext(ctx, s.logGroup, results, params.Context)
		if err != nil {
			return nil, err
		}
	}

	// Convert to source.Entry
	return s.convertResults(results), nil
}

// Tail streams log events in real-time.
func (s *Source) Tail(ctx context.Context, params source.TailParams) (<-chan source.Event, error) {
	eventChan := make(chan source.Event, DefaultEventChanBuffer)

	go func() {
		defer close(eventChan)

		// Start with a short lookback to catch recent events
		lastTime := time.Now().Add(-DefaultTailLookback)
		seenEvents := lru.New(DefaultLRUCacheCapacity)

		// Poll at regular intervals
		ticker := time.NewTicker(DefaultTailPollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				endTime := time.Now()

				filterStr := ""
				if params.Filter != nil {
					filterStr = params.Filter.String()
				}

				events, err := s.client.FilterLogEvents(ctx, s.logGroup, filterStr, lastTime, endTime)
				if err != nil {
					// Log transient errors for debugging but don't fail
					logging.Debug("CloudWatch tail transient error: %v", err)
					continue
				}

				for _, e := range events {
					// Deduplicate by message+timestamp using LRU cache
					key := fmt.Sprintf("%s:%s", e.Timestamp.Format(time.RFC3339Nano), e.Message)
					if !seenEvents.Add(key) {
						continue // Already seen
					}

					// Apply regex filter if specified
					if params.Filter != nil && !params.Filter.MatchString(e.Message) {
						continue
					}

					select {
					case eventChan <- source.Event{
						Timestamp: e.Timestamp,
						Message:   e.Message,
						Stream:    e.LogStream,
					}:
					case <-ctx.Done():
						return
					}
				}

				lastTime = endTime
			}
		}
	}()

	return eventChan, nil
}

// GetRecord retrieves a single log entry by its pointer.
func (s *Source) GetRecord(ctx context.Context, ptr string) (*source.Entry, error) {
	result, err := s.client.GetLogRecord(ctx, ptr)
	if err != nil {
		return nil, err
	}

	entries := s.convertResults([]LogResult{result})
	if len(entries) == 0 {
		return nil, fmt.Errorf("no record found for pointer")
	}

	return &entries[0], nil
}

// FetchContext retrieves context lines around a log entry.
func (s *Source) FetchContext(ctx context.Context, entry source.Entry, before, after int) ([]source.Event, []source.Event, error) {
	// CloudWatch only supports fetching lines before
	// Convert entry to LogResult format for FetchContext
	ts, err := parseLogTimestamp(entry.Timestamp.Format("2006-01-02 15:04:05.000"))
	if err != nil {
		return nil, nil, err
	}

	events, err := s.client.GetLogEvents(ctx, s.logGroup, entry.Stream, ts.Add(-ContextLookbackWindow), ts, before)
	if err != nil {
		return nil, nil, err
	}

	var beforeEvents []source.Event
	for _, e := range events {
		beforeEvents = append(beforeEvents, source.Event{
			Timestamp: e.Timestamp,
			Message:   e.Message,
			Stream:    entry.Stream,
		})
	}

	return beforeEvents, nil, nil
}

// ListStreams returns available log streams.
func (s *Source) ListStreams(ctx context.Context) ([]source.StreamInfo, error) {
	streams, err := s.client.ListStreams(ctx, s.logGroup, "", 100, "LastEventTime")
	if err != nil {
		return nil, err
	}

	var result []source.StreamInfo
	for _, st := range streams {
		result = append(result, source.StreamInfo{
			Name:      st.Name,
			Size:      st.StoredBytes,
			FirstTime: st.FirstEventTime,
			LastTime:  st.LastEventTime,
		})
	}

	return result, nil
}

// Type returns the source type identifier.
func (s *Source) Type() string {
	return "cloudwatch"
}

// Metadata returns source metadata for caching and evidence collection.
func (s *Source) Metadata() source.SourceMetadata {
	return source.SourceMetadata{
		Type:      "cloudwatch",
		URI:       s.logGroup,
		Profile:   s.profile,
		Region:    s.region,
		AccountID: s.accountID,
	}
}

// Close releases any resources held by the source.
func (s *Source) Close() error {
	return nil
}

// LogGroup returns the log group name.
func (s *Source) LogGroup() string {
	return s.logGroup
}

// Profile returns the AWS profile.
func (s *Source) Profile() string {
	return s.profile
}

// Region returns the AWS region.
func (s *Source) Region() string {
	return s.region
}

// AccountID returns the AWS account ID.
func (s *Source) AccountID() string {
	return s.accountID
}

// Client returns the underlying CloudWatch client for advanced operations.
func (s *Source) Client() LogsClient {
	return s.client
}

// convertResults converts CloudWatch LogResults to source.Entry slice.
func (s *Source) convertResults(results []LogResult) []source.Entry {
	var entries []source.Entry

	for _, r := range results {
		entry := source.Entry{
			Message:  r.Message,
			Stream:   r.LogStream,
			Source:   s.logGroup,
			Fields:   r.Fields,
			Ptr:      r.Fields["@ptr"], // CloudWatch @ptr
		}

		// Parse timestamp
		if r.Timestamp != "" {
			if ts, err := parseLogTimestamp(r.Timestamp); err == nil {
				entry.Timestamp = ts
			}
		}

		// Convert context events
		if len(r.Context) > 0 {
			for _, c := range r.Context {
				entry.Context.Before = append(entry.Context.Before, source.Event{
					Timestamp: c.Timestamp,
					Message:   c.Message,
					Stream:    r.LogStream,
				})
			}
		}

		entries = append(entries, entry)
	}

	return entries
}

// buildInsightsQuery creates a CloudWatch Insights query.
func buildInsightsQuery(filter string, limit int) string {
	if limit <= 0 {
		limit = 100
	}

	if filter == "" {
		return fmt.Sprintf(`fields @timestamp, @message, @logStream, @ptr
| sort @timestamp desc
| limit %d`, limit)
	}

	return fmt.Sprintf(`fields @timestamp, @message, @logStream, @ptr
| filter @message like /(?i)(%s)/
| sort @timestamp desc
| limit %d`, filter, limit)
}
