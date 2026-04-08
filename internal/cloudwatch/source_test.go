package cloudwatch

import (
	"context"
	"regexp"
	"testing"
	"time"
)

// mockLogsClient implements LogsClient for testing.
type mockLogsClient struct {
	logGroups       []LogGroupInfo
	streams         []StreamInfo
	queryResults    []LogResult
	tailEvents      []TailEvent
	logEvents       []LogEvent
	logRecord       LogResult
	err             error
	filterCallCount int
}

func (m *mockLogsClient) GetLogGroup(ctx context.Context, name string) (LogGroupInfo, error) {
	if m.err != nil {
		return LogGroupInfo{}, m.err
	}
	for _, g := range m.logGroups {
		if g.Name == name {
			return g, nil
		}
	}
	return LogGroupInfo{Name: name}, nil
}

func (m *mockLogsClient) ListLogGroups(ctx context.Context, prefix string, limit int) ([]LogGroupInfo, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.logGroups, nil
}

func (m *mockLogsClient) FilterLogEvents(ctx context.Context, logGroup, filter string, startTime, endTime time.Time) ([]TailEvent, error) {
	m.filterCallCount++
	if m.err != nil {
		return nil, m.err
	}
	return m.tailEvents, nil
}

func (m *mockLogsClient) RunInsightsQuery(ctx context.Context, params QueryParams) ([]LogResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.queryResults, nil
}

func (m *mockLogsClient) ListStreams(ctx context.Context, logGroup, prefix string, limit int, orderBy string) ([]StreamInfo, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.streams, nil
}

func (m *mockLogsClient) FetchContext(ctx context.Context, logGroup string, results []LogResult, contextLines int) ([]LogResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return results, nil
}

func (m *mockLogsClient) GetLogRecord(ctx context.Context, ptr string) (LogResult, error) {
	if m.err != nil {
		return LogResult{}, m.err
	}
	return m.logRecord, nil
}

func (m *mockLogsClient) GetLogEvents(ctx context.Context, logGroup, logStream string, startTime, endTime time.Time, limit int) ([]LogEvent, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.logEvents, nil
}

func TestSource_Query(t *testing.T) {
	now := time.Now()
	mock := &mockLogsClient{
		queryResults: []LogResult{
			{
				Timestamp: now.Format("2006-01-02 15:04:05.000"),
				LogStream: "stream-1",
				Message:   "test error message",
				Fields: map[string]string{
					"@ptr":       "ptr123",
					"@timestamp": now.Format("2006-01-02 15:04:05.000"),
					"@message":   "test error message",
				},
			},
			{
				Timestamp: now.Add(-time.Minute).Format("2006-01-02 15:04:05.000"),
				LogStream: "stream-2",
				Message:   "another log entry",
				Fields: map[string]string{
					"@ptr": "ptr456",
				},
			},
		},
	}

	src := NewSourceWithClient("/app/logs", mock)

	params := QueryInput{
		StartTime: now.Add(-time.Hour),
		EndTime:   now,
		Limit:     100,
	}

	entries, err := src.Query(context.Background(), params)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	if entries[0].Message != "test error message" {
		t.Errorf("expected message 'test error message', got %q", entries[0].Message)
	}
	if entries[0].Stream != "stream-1" {
		t.Errorf("expected stream 'stream-1', got %q", entries[0].Stream)
	}
	if entries[0].Ptr != "ptr123" {
		t.Errorf("expected ptr 'ptr123', got %q", entries[0].Ptr)
	}
}

func TestSource_Query_WithFilter(t *testing.T) {
	mock := &mockLogsClient{
		queryResults: []LogResult{
			{
				Timestamp: "2025-01-15 10:00:00.000",
				LogStream: "stream-1",
				Message:   "error: something went wrong",
				Fields:    map[string]string{"@ptr": "ptr1"},
			},
		},
	}

	src := NewSourceWithClient("/app/logs", mock)

	filter := regexp.MustCompile("error")
	params := QueryInput{
		StartTime: time.Now().Add(-time.Hour),
		EndTime:   time.Now(),
		Filter:    filter,
		Limit:     50,
	}

	entries, err := src.Query(context.Background(), params)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
}

func TestSource_Query_WithStream(t *testing.T) {
	mock := &mockLogsClient{
		queryResults: []LogResult{
			{
				Timestamp: "2025-01-15 10:00:00.000",
				LogStream: "i-abc123",
				Message:   "error on this instance",
				Fields:    map[string]string{"@ptr": "ptr1"},
			},
		},
	}

	src := NewSourceWithClient("/app/logs", mock)

	params := QueryInput{
		StartTime: time.Now().Add(-time.Hour),
		EndTime:   time.Now(),
		Stream:    "i-abc123",
		Limit:     50,
	}

	entries, err := src.Query(context.Background(), params)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
}

func TestSource_Tail(t *testing.T) {
	now := time.Now()
	mock := &mockLogsClient{
		tailEvents: []TailEvent{
			{
				Timestamp: now,
				LogStream: "stream-1",
				Message:   "new log message",
			},
		},
	}

	src := NewSourceWithClient("/app/logs", mock)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	eventChan, err := src.Tail(ctx, TailInput{})
	if err != nil {
		t.Fatalf("Tail failed: %v", err)
	}

	var events []Event
	timeout := time.After(3 * time.Second)

loop:
	for {
		select {
		case event, ok := <-eventChan:
			if !ok {
				break loop
			}
			events = append(events, event)
			if len(events) >= 1 {
				cancel()
			}
		case <-timeout:
			break loop
		}
	}

	if len(events) == 0 {
		t.Error("expected at least 1 event from tail")
	}
	if len(events) > 0 && events[0].Message != "new log message" {
		t.Errorf("expected message 'new log message', got %q", events[0].Message)
	}
}

func TestSource_Tail_WithFilter(t *testing.T) {
	now := time.Now()
	mock := &mockLogsClient{
		tailEvents: []TailEvent{
			{Timestamp: now, LogStream: "s1", Message: "error: problem"},
			{Timestamp: now, LogStream: "s1", Message: "info: all good"},
			{Timestamp: now, LogStream: "s1", Message: "error: another issue"},
		},
	}

	src := NewSourceWithClient("/app/logs", mock)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	filter := regexp.MustCompile("error")
	eventChan, err := src.Tail(ctx, TailInput{Filter: filter})
	if err != nil {
		t.Fatalf("Tail failed: %v", err)
	}

	var events []Event
	timeout := time.After(3 * time.Second)

loop:
	for {
		select {
		case event, ok := <-eventChan:
			if !ok {
				break loop
			}
			events = append(events, event)
			if len(events) >= 2 {
				cancel()
			}
		case <-timeout:
			break loop
		}
	}

	for _, e := range events {
		if !filter.MatchString(e.Message) {
			t.Errorf("got unfiltered message: %q", e.Message)
		}
	}
}

func TestSource_ListStreams(t *testing.T) {
	now := time.Now()
	mock := &mockLogsClient{
		streams: []StreamInfo{
			{Name: "stream-1", LastEventTime: now, StoredBytes: 1024},
			{Name: "stream-2", LastEventTime: now.Add(-time.Hour), StoredBytes: 2048},
		},
	}

	src := NewSourceWithClient("/app/logs", mock)

	streams, err := src.ListStreams(context.Background())
	if err != nil {
		t.Fatalf("ListStreams failed: %v", err)
	}

	if len(streams) != 2 {
		t.Fatalf("expected 2 streams, got %d", len(streams))
	}

	if streams[0].Name != "stream-1" {
		t.Errorf("expected stream name 'stream-1', got %q", streams[0].Name)
	}
}

func TestSource_Getters(t *testing.T) {
	src := &Source{
		logGroup:  "/app/logs",
		profile:   "prod",
		region:    "us-east-1",
		accountID: "123456789012",
	}

	if src.LogGroup() != "/app/logs" {
		t.Errorf("LogGroup() = %q, want '/app/logs'", src.LogGroup())
	}
	if src.Profile() != "prod" {
		t.Errorf("Profile() = %q, want 'prod'", src.Profile())
	}
	if src.Region() != "us-east-1" {
		t.Errorf("Region() = %q, want 'us-east-1'", src.Region())
	}
	if src.AccountID() != "123456789012" {
		t.Errorf("AccountID() = %q, want '123456789012'", src.AccountID())
	}
}

func TestBuildInsightsQuery_Source(t *testing.T) {
	tests := []struct {
		name   string
		filter string
		stream string
		limit  int
		check  func(string) bool
	}{
		{
			name:   "no filter",
			filter: "",
			limit:  100,
			check: func(q string) bool {
				return !stringContains(q, "filter") && stringContains(q, "limit 100")
			},
		},
		{
			name:   "with filter",
			filter: "error",
			limit:  50,
			check: func(q string) bool {
				return stringContains(q, "filter @message like") && stringContains(q, "error") && stringContains(q, "limit 50")
			},
		},
		{
			name:   "with stream",
			stream: "i-abc123",
			limit:  100,
			check: func(q string) bool {
				return stringContains(q, "filter @logStream like /i-abc123/")
			},
		},
		{
			name:   "with stream and filter",
			filter: "error",
			stream: "i-abc123",
			limit:  50,
			check: func(q string) bool {
				return stringContains(q, "filter @logStream like /i-abc123/") &&
					stringContains(q, "filter @message like") &&
					stringContains(q, "limit 50")
			},
		},
		{
			name:   "default limit",
			filter: "",
			limit:  0,
			check: func(q string) bool {
				return stringContains(q, "limit 100")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := buildInsightsQuery(tt.filter, tt.stream, tt.limit, false)
			if !tt.check(query) {
				t.Errorf("buildInsightsQuery(%q, %q, %d) = %q, check failed", tt.filter, tt.stream, tt.limit, query)
			}
		})
	}
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestSource_Query_WithContext(t *testing.T) {
	now := time.Now()
	mock := &mockLogsClient{
		queryResults: []LogResult{
			{
				Timestamp: now.Format("2006-01-02 15:04:05.000"),
				LogStream: "stream-1",
				Message:   "error message",
				Fields:    map[string]string{"@ptr": "ptr123"},
				Context: []LogEvent{
					{Timestamp: now.Add(-time.Second), Message: "context line 1"},
					{Timestamp: now.Add(-2 * time.Second), Message: "context line 2"},
				},
			},
		},
	}

	src := NewSourceWithClient("/app/logs", mock)

	params := QueryInput{
		StartTime: now.Add(-time.Hour),
		EndTime:   now,
		Context:   5,
		Limit:     100,
	}

	entries, err := src.Query(context.Background(), params)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	if len(entries[0].Context.Before) != 2 {
		t.Errorf("expected 2 context lines, got %d", len(entries[0].Context.Before))
	}
}

func TestSource_Query_Error(t *testing.T) {
	mock := &mockLogsClient{
		err: context.DeadlineExceeded,
	}

	src := NewSourceWithClient("/app/logs", mock)

	params := QueryInput{
		StartTime: time.Now().Add(-time.Hour),
		EndTime:   time.Now(),
		Limit:     100,
	}

	_, err := src.Query(context.Background(), params)
	if err == nil {
		t.Error("expected error from Query")
	}
}

func TestSource_Query_WithCustomQuery(t *testing.T) {
	mock := &mockLogsClient{
		queryResults: []LogResult{
			{
				Timestamp: "2025-01-15 10:00:00.000",
				LogStream: "stream-1",
				Message:   "custom query result",
				Fields:    map[string]string{"@ptr": "ptr1"},
			},
		},
	}

	src := NewSourceWithClient("/app/logs", mock)

	params := QueryInput{
		StartTime: time.Now().Add(-time.Hour),
		EndTime:   time.Now(),
		Query:     "fields @message | filter @message like /error/ | limit 10",
		Limit:     100,
	}

	entries, err := src.Query(context.Background(), params)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
}

func TestSource_Tail_Deduplication(t *testing.T) {
	now := time.Now()
	mock := &mockLogsClient{
		tailEvents: []TailEvent{
			{Timestamp: now, LogStream: "s1", Message: "duplicate message"},
			{Timestamp: now, LogStream: "s1", Message: "duplicate message"},
			{Timestamp: now.Add(time.Millisecond), LogStream: "s1", Message: "unique message"},
		},
	}

	src := NewSourceWithClient("/app/logs", mock)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	eventChan, err := src.Tail(ctx, TailInput{})
	if err != nil {
		t.Fatalf("Tail failed: %v", err)
	}

	var events []Event
	timeout := time.After(3 * time.Second)

loop:
	for {
		select {
		case event, ok := <-eventChan:
			if !ok {
				break loop
			}
			events = append(events, event)
			if len(events) >= 2 {
				cancel()
			}
		case <-timeout:
			break loop
		}
	}

	duplicateCount := 0
	for _, e := range events {
		if e.Message == "duplicate message" {
			duplicateCount++
		}
	}
	if duplicateCount > 1 {
		t.Errorf("expected deduplication to filter duplicates, got %d copies", duplicateCount)
	}
}

func TestSource_Tail_ContextCancellation(t *testing.T) {
	mock := &mockLogsClient{
		tailEvents: []TailEvent{},
	}

	src := NewSourceWithClient("/app/logs", mock)

	ctx, cancel := context.WithCancel(context.Background())

	eventChan, err := src.Tail(ctx, TailInput{})
	if err != nil {
		t.Fatalf("Tail failed: %v", err)
	}

	cancel()

	timeout := time.After(time.Second)
	select {
	case _, ok := <-eventChan:
		if ok {
			select {
			case <-eventChan:
			case <-timeout:
				t.Error("channel did not close after context cancellation")
			}
		}
	case <-timeout:
		t.Error("channel did not close after context cancellation")
	}
}

func TestSource_ListStreams_Error(t *testing.T) {
	mock := &mockLogsClient{
		err: context.DeadlineExceeded,
	}

	src := NewSourceWithClient("/app/logs", mock)

	_, err := src.ListStreams(context.Background())
	if err == nil {
		t.Error("expected error from ListStreams")
	}
}

func TestConvertResults_EmptyTimestamp(t *testing.T) {
	src := NewSourceWithClient("/app/logs", &mockLogsClient{})

	results := []LogResult{
		{
			Message:   "no timestamp",
			LogStream: "stream-1",
			Fields:    map[string]string{"@ptr": "ptr1"},
		},
	}

	entries := src.convertResults(results)

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Message != "no timestamp" {
		t.Errorf("expected message 'no timestamp', got %q", entries[0].Message)
	}
	if !entries[0].Timestamp.IsZero() {
		t.Error("expected zero timestamp for empty timestamp string")
	}
}

func TestConvertResults_InvalidTimestamp(t *testing.T) {
	src := NewSourceWithClient("/app/logs", &mockLogsClient{})

	results := []LogResult{
		{
			Timestamp: "invalid-timestamp",
			Message:   "bad timestamp",
			LogStream: "stream-1",
			Fields:    map[string]string{"@ptr": "ptr1"},
		},
	}

	entries := src.convertResults(results)

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Message != "bad timestamp" {
		t.Errorf("expected message 'bad timestamp', got %q", entries[0].Message)
	}
}
