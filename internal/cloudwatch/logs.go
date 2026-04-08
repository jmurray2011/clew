package cloudwatch

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
)

// Configuration constants for CloudWatch Logs operations
const (
	// QueryTimeout is the maximum time to wait for a Logs Insights query to complete
	QueryTimeout = 60 * time.Second

	// QueryPollInterval is how often to check for query completion
	QueryPollInterval = 500 * time.Millisecond

	// MaxConcurrentContextFetches limits parallel API calls when fetching context
	MaxConcurrentContextFetches = 10

	// ContextLookbackWindow is how far back to search for context lines
	ContextLookbackWindow = 30 * time.Minute
)

// Pre-compiled regexes (avoids repeated compilation)
var (
	pipeRegex       = regexp.MustCompile(`\|`)
	whitespaceRegex = regexp.MustCompile(`\s+`)
	trimSpaceRegex  = regexp.MustCompile(`^\s+|\s+$`)
)

// LogsClient defines the interface for CloudWatch Logs operations.
// This interface enables mocking for testing.
type LogsClient interface {
	// GetLogGroup returns information about a specific log group.
	GetLogGroup(ctx context.Context, name string) (LogGroupInfo, error)

	// ListLogGroups returns available log groups.
	ListLogGroups(ctx context.Context, prefix string, limit int) ([]LogGroupInfo, error)

	// FilterLogEvents returns log events matching a filter pattern (for tailing).
	FilterLogEvents(ctx context.Context, logGroup, filter string, startTime, endTime time.Time) ([]TailEvent, error)

	// RunInsightsQuery executes a Logs Insights query and returns the results.
	RunInsightsQuery(ctx context.Context, params QueryParams) ([]LogResult, error)

	// ListStreams returns log streams for a log group.
	ListStreams(ctx context.Context, logGroup, prefix string, limit int, orderBy string) ([]StreamInfo, error)

	// FetchContext retrieves preceding log events for context around matched results.
	FetchContext(ctx context.Context, logGroup string, results []LogResult, contextLines int) ([]LogResult, error)

	// GetLogRecord retrieves a single log record by its @ptr value.
	GetLogRecord(ctx context.Context, ptr string) (LogResult, error)

	// GetLogEvents fetches log events from a specific stream within a time range.
	GetLogEvents(ctx context.Context, logGroup, logStream string, startTime, endTime time.Time, limit int) ([]LogEvent, error)
}

// Ensure Client implements LogsClient interface
var _ LogsClient = (*Client)(nil)

// Client wraps the CloudWatch Logs client with convenience methods.
type Client struct {
	client *cloudwatchlogs.Client
}

// NewClient creates a new Client wrapper from an SDK client.
func NewClient(client *cloudwatchlogs.Client) *Client {
	return &Client{client: client}
}

// QueryParams holds parameters for running a Logs Insights query.
type QueryParams struct {
	LogGroup  string
	StartTime time.Time
	EndTime   time.Time
	Query     string
	Limit     int
}

// LogResult represents a single log entry from a query.
type LogResult struct {
	Timestamp string
	LogStream string
	Message   string
	Fields    map[string]string
	Context   []LogEvent // Preceding log events for context
	IsContext bool       // True if this entry is context, not a match
}

// LogEvent represents a simple log event (used for context).
type LogEvent struct {
	Timestamp time.Time
	Message   string
}

// StreamInfo represents information about a log stream.
type StreamInfo struct {
	Name           string
	LastEventTime  time.Time
	FirstEventTime time.Time
	StoredBytes    int64
}

// LogGroupInfo represents information about a log group.
type LogGroupInfo struct {
	Name          string
	StoredBytes   int64
	CreationTime  time.Time
	RetentionDays int
}

// TailEvent represents a log event from FilterLogEvents (for tailing).
type TailEvent struct {
	Timestamp time.Time
	LogStream string
	Message   string
}

// GetLogGroup returns information about a specific log group.
func (c *Client) GetLogGroup(ctx context.Context, name string) (LogGroupInfo, error) {
	input := &cloudwatchlogs.DescribeLogGroupsInput{
		LogGroupNamePrefix: &name,
		Limit:              aws.Int32(1),
	}

	result, err := c.client.DescribeLogGroups(ctx, input)
	if err != nil {
		return LogGroupInfo{}, fmt.Errorf("failed to describe log group: %w", err)
	}

	// Find exact match
	for _, g := range result.LogGroups {
		if aws.ToString(g.LogGroupName) == name {
			group := LogGroupInfo{
				Name: aws.ToString(g.LogGroupName),
			}
			if g.StoredBytes != nil {
				group.StoredBytes = *g.StoredBytes
			}
			if g.CreationTime != nil {
				group.CreationTime = time.UnixMilli(*g.CreationTime)
			}
			if g.RetentionInDays != nil {
				group.RetentionDays = int(*g.RetentionInDays)
			}
			return group, nil
		}
	}

	return LogGroupInfo{}, fmt.Errorf("log group not found: %s", name)
}

// ListLogGroups returns available log groups.
func (c *Client) ListLogGroups(ctx context.Context, prefix string, limit int) ([]LogGroupInfo, error) {
	// AWS caps DescribeLogGroups at 50 per page; use that as the page size
	// and let the paginator fetch multiple pages up to the caller's limit.
	pageSize := limit
	if pageSize > 50 {
		pageSize = 50
	}

	input := &cloudwatchlogs.DescribeLogGroupsInput{
		Limit: aws.Int32(int32(pageSize)),
	}

	if prefix != "" {
		input.LogGroupNamePrefix = &prefix
	}

	var groups []LogGroupInfo

	paginator := cloudwatchlogs.NewDescribeLogGroupsPaginator(c.client, input)
	for paginator.HasMorePages() && len(groups) < limit {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to describe log groups: %w", err)
		}

		for _, g := range page.LogGroups {
			if len(groups) >= limit {
				break
			}

			group := LogGroupInfo{
				Name: aws.ToString(g.LogGroupName),
			}

			if g.StoredBytes != nil {
				group.StoredBytes = *g.StoredBytes
			}
			if g.CreationTime != nil {
				group.CreationTime = time.UnixMilli(*g.CreationTime)
			}
			if g.RetentionInDays != nil {
				group.RetentionDays = int(*g.RetentionInDays)
			}

			groups = append(groups, group)
		}
	}

	return groups, nil
}

// FilterLogEvents returns log events matching a filter pattern (for tailing).
func (c *Client) FilterLogEvents(ctx context.Context, logGroup, filter string, startTime, endTime time.Time) ([]TailEvent, error) {
	input := &cloudwatchlogs.FilterLogEventsInput{
		LogGroupName: &logGroup,
		StartTime:    aws.Int64(startTime.UnixMilli()),
		EndTime:      aws.Int64(endTime.UnixMilli()),
		Limit:        aws.Int32(100),
	}

	if filter != "" {
		// Convert simple filter to CloudWatch filter pattern
		// CloudWatch uses space-separated terms with ? for OR
		filterPattern := convertToFilterPattern(filter)
		input.FilterPattern = &filterPattern
	}

	result, err := c.client.FilterLogEvents(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to filter log events: %w", err)
	}

	var events []TailEvent
	for _, e := range result.Events {
		if e.Timestamp == nil || e.Message == nil {
			continue
		}
		events = append(events, TailEvent{
			Timestamp: time.UnixMilli(*e.Timestamp),
			LogStream: aws.ToString(e.LogStreamName),
			Message:   *e.Message,
		})
	}

	return events, nil
}

// convertToFilterPattern converts a simple filter like "error|exception" to CloudWatch filter pattern.
func convertToFilterPattern(filter string) string {
	// CloudWatch filter syntax: ?term1 ?term2 for OR
	// For now, just pass through - user can use CloudWatch syntax directly
	// or we convert pipe-separated to space-separated with ?
	if filter == "" {
		return ""
	}

	// If it contains |, treat as OR (uses pre-compiled regex)
	parts := pipeRegex.Split(filter, -1)
	if len(parts) > 1 {
		var terms []string
		for _, p := range parts {
			p = whitespaceRegex.ReplaceAllString(p, " ")
			p = trimSpaceRegex.ReplaceAllString(p, "")
			if p != "" {
				terms = append(terms, "?\""+p+"\"")
			}
		}
		return strings.Join(terms, " ")
	}

	return filter
}

// RunInsightsQuery executes a Logs Insights query and returns the results.
func (c *Client) RunInsightsQuery(ctx context.Context, params QueryParams) ([]LogResult, error) {
	startQuery, err := c.client.StartQuery(ctx, &cloudwatchlogs.StartQueryInput{
		LogGroupName: &params.LogGroup,
		StartTime:    aws.Int64(params.StartTime.Unix()),
		EndTime:      aws.Int64(params.EndTime.Unix()),
		QueryString:  &params.Query,
		Limit:        aws.Int32(int32(params.Limit)),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start query: %w", err)
	}

	// Poll for results with timeout
	timeout := time.After(QueryTimeout)
	ticker := time.NewTicker(QueryPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return nil, fmt.Errorf("query did not complete within %v", QueryTimeout)
		case <-ticker.C:
			result, err := c.client.GetQueryResults(ctx, &cloudwatchlogs.GetQueryResultsInput{
				QueryId: startQuery.QueryId,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to get query results: %w", err)
			}

			switch result.Status {
			case types.QueryStatusComplete:
				return parseResults(result.Results), nil
			case types.QueryStatusFailed:
				return nil, fmt.Errorf("query failed")
			case types.QueryStatusCancelled:
				return nil, fmt.Errorf("query was cancelled")
			case types.QueryStatusTimeout:
				return nil, fmt.Errorf("query timed out on AWS side")
			}
			// Status is Running or Scheduled, continue polling
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// parseResults converts AWS SDK results to our LogResult type.
func parseResults(results [][]types.ResultField) []LogResult {
	var logResults []LogResult

	for _, row := range results {
		result := LogResult{
			Fields: make(map[string]string),
		}

		for _, field := range row {
			if field.Field == nil || field.Value == nil {
				continue
			}

			fieldName := *field.Field
			fieldValue := *field.Value

			result.Fields[fieldName] = fieldValue

			switch fieldName {
			case "@timestamp":
				result.Timestamp = fieldValue
			case "@logStream":
				result.LogStream = fieldValue
			case "@message":
				result.Message = fieldValue
			}
		}

		logResults = append(logResults, result)
	}

	return logResults
}

// ListStreams returns log streams for a log group.
func (c *Client) ListStreams(ctx context.Context, logGroup, prefix string, limit int, orderBy string) ([]StreamInfo, error) {
	input := &cloudwatchlogs.DescribeLogStreamsInput{
		LogGroupName: &logGroup,
		Limit:        aws.Int32(int32(limit)),
		Descending:   aws.Bool(true),
	}

	if prefix != "" {
		input.LogStreamNamePrefix = &prefix
	}

	switch orderBy {
	case "LogStreamName":
		input.OrderBy = types.OrderByLogStreamName
	default:
		input.OrderBy = types.OrderByLastEventTime
	}

	result, err := c.client.DescribeLogStreams(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to describe log streams: %w", err)
	}

	var streams []StreamInfo
	for _, s := range result.LogStreams {
		stream := StreamInfo{
			Name: aws.ToString(s.LogStreamName),
		}

		if s.LastEventTimestamp != nil {
			stream.LastEventTime = time.UnixMilli(*s.LastEventTimestamp)
		}
		if s.FirstEventTimestamp != nil {
			stream.FirstEventTime = time.UnixMilli(*s.FirstEventTimestamp)
		}
		// Note: StoredBytes is deprecated for log streams, leaving as 0

		streams = append(streams, stream)
	}

	return streams, nil
}

// FetchContext retrieves preceding log events for context around matched results.
// Uses parallel fetching for better performance.
func (c *Client) FetchContext(ctx context.Context, logGroup string, results []LogResult, contextLines int) ([]LogResult, error) {
	if contextLines <= 0 {
		return results, nil
	}

	// Use a semaphore to limit concurrent API calls
	sem := make(chan struct{}, MaxConcurrentContextFetches)

	type contextResult struct {
		index  int
		events []LogEvent
	}

	resultsChan := make(chan contextResult, len(results))
	var wg sync.WaitGroup

	for i := range results {
		if results[i].Timestamp == "" || results[i].LogStream == "" {
			continue
		}

		// Parse the timestamp - CloudWatch Logs uses various formats
		ts, err := parseLogTimestamp(results[i].Timestamp)
		if err != nil {
			continue
		}

		wg.Add(1)
		go func(idx int, timestamp time.Time, logStream string) {
			defer wg.Done()

			// Check for cancellation before acquiring semaphore
			select {
			case <-ctx.Done():
				return
			default:
			}

			// Acquire semaphore
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}

			// Fetch events before this timestamp from the same log stream
			startTime := timestamp.Add(-ContextLookbackWindow)
			endTime := timestamp.Add(-1 * time.Millisecond)

			events, err := c.GetLogEvents(ctx, logGroup, logStream, startTime, endTime, contextLines)
			if err != nil {
				return // Don't fail the whole query if context fetch fails
			}

			resultsChan <- contextResult{index: idx, events: events}
		}(i, ts, results[i].LogStream)
	}

	// Close results channel when all goroutines complete
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Collect results
	for res := range resultsChan {
		results[res.index].Context = res.events
	}

	return results, nil
}

// GetLogEvents fetches log events from a specific stream within a time range.
// Reads backward from endTime to get the N events immediately before the match.
// Returns events in chronological order (oldest first).
func (c *Client) GetLogEvents(ctx context.Context, logGroup, logStream string, startTime, endTime time.Time, limit int) ([]LogEvent, error) {
	// Read backward from endTime (just before the match) so we get the
	// lines immediately preceding the error, not random lines from 30min ago.
	input := &cloudwatchlogs.GetLogEventsInput{
		LogGroupName:  &logGroup,
		LogStreamName: &logStream,
		StartTime:     aws.Int64(startTime.UnixMilli()),
		EndTime:       aws.Int64(endTime.UnixMilli()),
		StartFromHead: aws.Bool(false),
		Limit:         aws.Int32(int32(limit)),
	}

	result, err := c.client.GetLogEvents(ctx, input)
	if err != nil {
		return nil, err
	}

	var allEvents []LogEvent
	for _, e := range result.Events {
		if e.Timestamp == nil || e.Message == nil {
			continue
		}
		allEvents = append(allEvents, LogEvent{
			Timestamp: time.UnixMilli(*e.Timestamp),
			Message:   *e.Message,
		})
	}

	return allEvents, nil
}

// parseLogTimestamp parses timestamps from CloudWatch Logs in various formats.
func parseLogTimestamp(input string) (time.Time, error) {
	// CloudWatch Logs Insights returns timestamps in format: "2025-12-03 19:13:20.000"
	formats := []string{
		"2006-01-02 15:04:05.000",  // CloudWatch Logs Insights format
		"2006-01-02 15:04:05",      // Without milliseconds
		time.RFC3339Nano,           // 2006-01-02T15:04:05.999999999Z07:00
		time.RFC3339,               // 2006-01-02T15:04:05Z07:00
		"2006-01-02T15:04:05.000Z", // ISO with milliseconds
		"2006-01-02T15:04:05.000",  // ISO with milliseconds, no TZ
	}

	for _, format := range formats {
		if t, err := time.Parse(format, input); err == nil {
			return t.UTC(), nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse timestamp: %s", input)
}

// BuildDefaultQuery creates a default Logs Insights query with optional stream and message filters.
func BuildDefaultQuery(filter, stream string, limit int) string {
	query := "fields @timestamp, @message, @logStream"

	if stream != "" {
		query += fmt.Sprintf("\n| filter @logStream like /%s/", stream)
	}

	if filter != "" {
		query += fmt.Sprintf("\n| filter @message like /(?i)(%s)/", filter)
	}

	query += "\n| sort @timestamp desc"
	query += fmt.Sprintf("\n| limit %d", limit)

	return query
}

// GetLogRecord retrieves a single log record by its @ptr value.
func (c *Client) GetLogRecord(ctx context.Context, ptr string) (LogResult, error) {
	result, err := c.client.GetLogRecord(ctx, &cloudwatchlogs.GetLogRecordInput{
		LogRecordPointer: &ptr,
	})
	if err != nil {
		return LogResult{}, fmt.Errorf("failed to get log record: %w", err)
	}

	logResult := LogResult{
		Fields: make(map[string]string),
	}

	for key, value := range result.LogRecord {
		logResult.Fields[key] = value

		switch key {
		case "@timestamp":
			logResult.Timestamp = value
		case "@logStream":
			logResult.LogStream = value
		case "@message":
			logResult.Message = value
		}
	}

	return logResult, nil
}

// BuildStatsQuery creates a Logs Insights query that returns counts by time bucket.
func BuildStatsQuery(filter, stream string, limit int) string {
	query := "fields @timestamp, @message"

	if stream != "" {
		query += fmt.Sprintf("\n| filter @logStream like /%s/", stream)
	}

	if filter != "" {
		query += fmt.Sprintf("\n| filter @message like /(?i)(%s)/", filter)
	}

	query += "\n| stats count() as count by bin(5m) as time_bucket"
	query += "\n| sort time_bucket desc"
	query += fmt.Sprintf("\n| limit %d", limit)

	return query
}
