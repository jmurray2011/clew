package cloudwatch

import (
	"context"
	"fmt"
	"time"

	"github.com/jmurray2011/clew/internal/logging"
	"github.com/jmurray2011/clew/pkg/lru"
)

// Default configuration values
const (
	DefaultEventChanBuffer  = 100
	DefaultLRUCacheCapacity = 10000
	DefaultTailLookback     = 5 * time.Second
	DefaultTailPollInterval = 2 * time.Second
	MaxListStreamsLimit     = 50
)

// Source is the CloudWatch Logs source.
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

	if accountID, err := GetAccountID(profile, region); err == nil {
		s.accountID = accountID
	} else {
		logging.Debug("Could not determine AWS account ID: %v", err)
	}

	return s, nil
}

// NewSourceWithClient creates a source with a custom client (for testing).
func NewSourceWithClient(logGroup string, client LogsClient) *Source {
	return &Source{
		logGroup: logGroup,
		client:   client,
	}
}

// Query returns log entries matching the given parameters.
func (s *Source) Query(ctx context.Context, params QueryInput) ([]Entry, error) {
	query := params.Query
	if query == "" {
		filterStr := ""
		if params.Filter != nil {
			filterStr = params.Filter.String()
		}
		query = buildInsightsQuery(filterStr, params.Stream, params.Limit, params.SortAsc)
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

	return s.convertResults(results), nil
}

// Tail streams log events in real-time.
func (s *Source) Tail(ctx context.Context, params TailInput) (<-chan Event, error) {
	eventChan := make(chan Event, DefaultEventChanBuffer)

	go func() {
		defer close(eventChan)

		lastTime := time.Now().Add(-DefaultTailLookback)
		seenEvents := lru.New(DefaultLRUCacheCapacity)

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
					logging.Debug("CloudWatch tail transient error: %v", err)
					continue
				}

				for _, e := range events {
					key := fmt.Sprintf("%s:%s", e.Timestamp.Format(time.RFC3339Nano), e.Message)
					if !seenEvents.Add(key) {
						continue
					}

					if params.Filter != nil && !params.Filter.MatchString(e.Message) {
						continue
					}

					// Apply stream filter
					if params.Stream != "" && !containsSubstring(e.LogStream, params.Stream) {
						continue
					}

					select {
					case eventChan <- Event{
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

// ListStreams returns available log streams.
func (s *Source) ListStreams(ctx context.Context) ([]StreamInfo, error) {
	return s.client.ListStreams(ctx, s.logGroup, "", MaxListStreamsLimit, "LastEventTime")
}

// LogGroup returns the log group name.
func (s *Source) LogGroup() string { return s.logGroup }

// Profile returns the AWS profile.
func (s *Source) Profile() string { return s.profile }

// Region returns the AWS region.
func (s *Source) Region() string { return s.region }

// AccountID returns the AWS account ID.
func (s *Source) AccountID() string { return s.accountID }

// Client returns the underlying CloudWatch client.
func (s *Source) Client() LogsClient { return s.client }

// convertResults converts CloudWatch LogResults to Entry slice.
func (s *Source) convertResults(results []LogResult) []Entry {
	var entries []Entry

	for _, r := range results {
		entry := Entry{
			Message: r.Message,
			Stream:  r.LogStream,
			Source:  s.logGroup,
			Fields:  r.Fields,
			Ptr:     r.Fields["@ptr"],
		}

		if r.Timestamp != "" {
			if ts, err := parseLogTimestamp(r.Timestamp); err == nil {
				entry.Timestamp = ts
			}
		}

		if len(r.Context) > 0 {
			for _, c := range r.Context {
				entry.Context.Before = append(entry.Context.Before, Event{
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

// buildInsightsQuery creates a CloudWatch Insights query with optional stream and message filters.
func buildInsightsQuery(filter, stream string, limit int, sortAsc bool) string {
	if limit <= 0 {
		limit = 100
	}

	query := "fields @timestamp, @message, @logStream, @ptr"

	if stream != "" {
		query += fmt.Sprintf("\n| filter @logStream like /%s/", stream)
	}

	if filter != "" {
		query += fmt.Sprintf("\n| filter @message like /(?i)(%s)/", filter)
	}

	if sortAsc {
		query += "\n| sort @timestamp asc"
	} else {
		query += "\n| sort @timestamp desc"
	}
	query += fmt.Sprintf("\n| limit %d", limit)

	return query
}

// containsSubstring is a simple case-sensitive substring check.
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
