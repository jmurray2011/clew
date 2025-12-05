# clew Examples

## Source URIs

clew uses URIs to identify log sources. You can query CloudWatch Logs, local files, and more:

```bash
# CloudWatch Logs (with profile and region)
clew query 'cloudwatch:///app/api/logs?profile=prod&region=us-east-1' -s 2h -f "error"

# Local file (explicit URI)
clew query file:///var/log/app.log -s 2h -f "error"

# Local file (shorthand - just the path)
clew query /var/log/app.log -s 2h -f "error"

# Source alias (defined in config)
clew query @prod-api -s 1h -f "timeout"
```

## Local File Queries

Query local log files with automatic format detection or specify the format explicitly:

```bash
# Auto-detect format
clew query /var/log/app.log -s 2h -f "error"

# Specify format for Java logs with stack traces
clew query /var/log/tomcat/catalina.out --format java -s 2h -f "Exception"

# JSON-formatted logs
clew query /var/log/app.json --format json -s 1h -f "level.*error"

# Syslog format
clew query /var/log/syslog --format syslog -s 1h -f "failed"
```

Supported formats: `auto`, `plain`, `json`, `syslog`, `java`

## Filtering

The `-f` flag accepts a case-insensitive regex pattern:

```bash
# Simple text match
clew query @tomcat -s 1h -f "error"

# OR matching - find any of these terms
clew query @tomcat -s 1h -f "error|exception|timeout"

# Match patterns with regex
clew query @tomcat -s 1h -f "user-[0-9]+"        # user-123, user-456
clew query @api -s 1h -f "status=[45][0-9]{2}"   # status=400, status=500

# Escape special regex characters with backslash
clew query @api -s 1h -f "\\[ERROR\\]"           # literal [ERROR]
```

The filter matches against the log message. For CloudWatch sources with custom fields, use `-q` for Insights queries.

## Basic Queries

```bash
# CloudWatch: Search for errors in the last 2 hours
clew query 'cloudwatch:///app/logs?profile=prod' -s 2h -f "error|exception"

# Local file: Search with time range
clew query /var/log/app.log -s "2025-01-15T00:00:00Z" -e "2025-01-15T12:00:00Z" -f "timeout"

# Limit results
clew query @api -s 1d -f "OutOfMemory" -l 100

# Show context lines around each match
clew query @api -s 2h -f "exception" -C 5
```

## Configuring Source Aliases

Configure source aliases in `~/.clew/config.yaml`:

```yaml
sources:
  prod-api:
    uri: cloudwatch:///app/api/prod?profile=prod&region=us-east-1
  staging:
    uri: cloudwatch:///app/api/staging?profile=staging
  tomcat:
    uri: cloudwatch:///app/tomcat/logs?profile=prod
  waf:
    uri: cloudwatch:///aws-waf-logs-MyALB?profile=prod
  local:
    uri: file:///var/log/app.log
    format: java

default_source: prod-api
```

Then use `@alias-name` to reference sources:

```bash
# Use a configured alias
clew query @tomcat -s 2h -f "error"

# Query local file via alias
clew query @local -s 1h -f "exception"

# List configured sources
clew sources
```

## Custom Insights Queries (CloudWatch)

For CloudWatch sources, use `-q` to write full Insights queries:

```bash
# Stats by time bucket
clew query @api -s 1d -f "error" --stats

# Full custom query - alias aggregations to sort them
clew query @waf -s 1d -q "stats count() as total by httpRequest.clientIp | filter total > 0 | sort total desc | limit 20"

# WAF blocked requests by country
clew query @waf -s 7d -q "stats count() as total by httpRequest.country | sort total desc"

# Top blocked IPs with action filter
clew query @waf -s 7d -q "filter action='BLOCK' | stats count() as blocks by httpRequest.clientIp | sort blocks desc | limit 20"
```

**Note:** CloudWatch Logs Insights requires an alias to sort aggregation results. Use `stats count() as myname` then `sort myname desc`.

## Saved Queries

```bash
# Save a query for later
clew query @tomcat -s 2h -f "exception|error" --save errors

# Run a saved query
clew query --run errors

# Override saved query parameters
clew query --run errors -s 6h
```

## Query History

```bash
# View recent queries
clew history

# Re-run query #3
clew history --run 3

# Clear history
clew history --clear
```

## Cost Estimation (CloudWatch)

```bash
# Estimate query cost before running
clew query 'cloudwatch:///app/logs?profile=prod' -s 7d -f "error" --dry-run

# Check cost for a specific log group
clew query @tomcat -s 30d --dry-run
```

**Note:** Cost estimates are only available for CloudWatch sources. Estimates assume uniform log distribution over time. Actual costs may vary based on log patterns and retention settings.

## AWS Console URL (CloudWatch)

Generate a clickable URL to open the query in AWS Console:

```bash
# Get a console URL for your query
clew query @tomcat -s 2h -f "error" --url

# Combine with other options
clew query @waf -s 1d -q "stats count() as total by httpRequest.clientIp | sort total desc" --url
```

The URL opens CloudWatch Logs Insights with the same log group, time range, and query pre-filled. Only available for CloudWatch sources.

## Field Discovery (CloudWatch)

```bash
# Discover fields in WAF logs
clew fields -g aws-waf-logs-MyALB --profile prod

# Sample more records for better coverage
clew fields -g aws-waf-logs-MyALB --profile prod --sample 100

# Query historical data
clew fields -g /app/logs --profile prod -s 2023-11-01T00:00:00Z -e 2023-11-30T23:59:59Z
```

## Log Groups and Streams (CloudWatch)

```bash
# List all log groups
clew groups --profile prod

# Filter by prefix
clew groups --prefix "/app/" --profile prod

# List streams in a group
clew streams -g /app/tomcat/logs --profile prod

# Filter streams by prefix
clew streams -g /app/tomcat/logs --profile prod --prefix "i-"
```

## Real-time Tailing (CloudWatch)

```bash
# Tail logs
clew tail -g /app/logs --profile prod

# Tail with filter (highlights matches)
clew tail -g /app/logs --profile prod -f "error"

# Tail specific streams
clew tail -g /app/logs --profile prod --stream "i-abc123"
```

## Around Mode

Query logs around a specific timestamp - useful when investigating an issue:

```bash
# Show logs 5 minutes before/after a timestamp (CloudWatch)
clew around -g /app/tomcat/logs --profile prod -t "2025-12-04T10:30:00Z"

# Specify a different window size
clew around -g /app/api/logs --profile prod -t "2025-12-04T10:30:00Z" --window 10m

# Use shorter format for timestamp
clew around -g /app/tomcat/logs --profile prod -t "2025-12-04 10:30:00" --window 2m
```

## Watch Mode

Run a query repeatedly to monitor for new matches:

```bash
# Refresh query every 30 seconds
clew query @tomcat -s 5m -f "error" --watch 30

# Monitor stats over time (CloudWatch)
clew query @api -s 1h -f "timeout" --stats --watch 60
```

Press Ctrl+C to stop watch mode.

## Fetching Specific Events

```bash
# Get full event by pointer (from query results)
clew get "CpMBCmQKJjkz..."
```

## Retention (CloudWatch)

```bash
# View retention for all groups
clew retention --profile prod

# View retention for specific group
clew retention -g /app/tomcat/logs --profile prod
```

## Output Formats

```bash
# Default text output
clew query @tomcat -s 1h -f "error"

# JSON output
clew query @tomcat -s 1h -f "error" -o json

# CSV output
clew query @tomcat -s 1h -f "error" -o csv

# Export to file
clew query @tomcat -s 1d -f "error" --export errors.json -o json

# Works with local files too
clew query /var/log/app.log -s 1h -f "error" -o json
```

## Multiple Log Groups (CloudWatch Legacy)

For querying multiple CloudWatch log groups, use the legacy `-g` flag:

```bash
# Query multiple groups (comma-separated)
clew query -g /app/tomcat/logs,/app/api/logs --profile prod -s 2h -f "error"
```

**Note:** For multi-source queries, run separate queries for each source.

## AWS Profile and Region

Specify profile and region in the URI or as flags:

```bash
# In the URI (recommended)
clew query 'cloudwatch:///app/logs?profile=prod&region=eu-west-1' -s 1h -f "error"

# As global flags
clew query 'cloudwatch:///app/logs' -s 1h -f "error" --profile prod --region eu-west-1

# Mix: profile in URI, region as flag
clew query 'cloudwatch:///app/logs?profile=prod' --region eu-west-1 -s 1h -f "error"

# Source alias handles credentials automatically
clew query @prod-api -s 1h -f "error"
```

## CloudWatch Metrics

Query CloudWatch metrics to identify spikes and anomalies, then pivot to logs:

```bash
# Query ALB request count over 24 hours
clew metrics -n AWS/ApplicationELB -m RequestCount -s 24h -d "LoadBalancer=app/my-alb/abc123"

# Query 5XX errors with hourly period
clew metrics -n AWS/ApplicationELB -m HTTPCode_ELB_5XX_Count -s 24h --period 1h

# Lambda errors over 7 days
clew metrics -n AWS/Lambda -m Errors --stat Sum -s 7d --period 1h -d "FunctionName=my-function"

# List available metrics in a namespace
clew metrics --list -n AWS/ApplicationELB

# Different statistics
clew metrics -n AWS/EC2 -m CPUUtilization --stat Average -s 6h -d "InstanceId=i-abc123"
```

### Workflow: Find Spike, Then Search Logs

```bash
# 1. Find when 5XX errors spiked
clew metrics -n AWS/ApplicationELB -m HTTPCode_ELB_5XX_Count -s 24h --period 5m --profile prod

# Output shows spike at 2025-12-04 14:30 with bar chart visualization
# 2025-12-04 14:25        5  ███
# 2025-12-04 14:30       47  ████████████████████████████████████████
# 2025-12-04 14:35        3  ██

# 2. Search application logs around that time
clew around -g /app/logs --profile prod -t "2025-12-04T14:30:00Z" --window 5m -f "error|exception"
```

### Output Formats

```bash
# Text output with bar chart (default)
clew metrics -n AWS/Lambda -m Invocations -s 6h

# JSON output
clew metrics -n AWS/Lambda -m Invocations -s 6h -o json

# CSV output
clew metrics -n AWS/Lambda -m Invocations -s 6h -o csv
```

### Common Namespaces

- `AWS/ApplicationELB` - Application Load Balancer
- `AWS/Lambda` - Lambda functions
- `AWS/EC2` - EC2 instances
- `AWS/RDS` - RDS databases
- `AWS/ECS` - ECS services
- `AWS/ApiGateway` - API Gateway
- `AWS/DynamoDB` - DynamoDB tables
- `AWS/S3` - S3 buckets

## Case Management

Track investigations with cases, collect evidence, and generate reports.

### Starting an Investigation

```bash
# Create a new case
clew case new "API outage 2025-01-15"
# ✓ Created case: api-outage-2025-01-15
# ✓ Set as active case

# Check case status
clew case status
```

### Running Queries (Auto-Captured)

When a case is active, queries are automatically logged to the timeline:

```bash
# Query results show [N] index and @ptr suffix for easy reference
clew query @api -s 2h -f "error|timeout"
# [1] 2025-01-15 10:30:00 | i-abc123  @AAAAAAAAp11
#   ERROR: connection pool exhausted, 0/100 available
#
# [2] 2025-01-15 10:31:00 | i-abc123  @AAAAAAAAp22
#   ERROR: timeout waiting for connection
#
# [3] 2025-01-15 10:32:00 | i-abc123  @AAAAAAAAp33
#   ERROR: connection pool exhausted, 0/100 available

# Mark a significant query
clew case mark
```

### Collecting Evidence

Save specific log entries as evidence using the index or @ptr suffix:

```bash
# By index (easiest)
clew case keep 1

# By @ptr suffix (unique ending characters)
clew case keep p22

# With annotation
clew case keep 3 -a "Third occurrence - pattern confirmed"

# View collected evidence
clew case evidence
```

### Adding Notes

Document findings during the investigation:

```bash
# Inline note
clew case note "Pool exhaustion correlates with 10:28 deploy"

# From file
clew case note -f findings.md

# Open editor
clew case note -e
```

### Setting Summary

```bash
clew case summary "Root cause: connection pool misconfiguration in v2.3.1 deploy"
```

### Viewing Timeline

```bash
# Full timeline
clew case timeline

# Only marked (significant) entries
clew case timeline --marked

# Filter by type
clew case timeline --type query
clew case timeline --type note
```

### Generating Reports

```bash
# Markdown to stdout
clew case report

# Save to file (format from extension)
clew case report -o report.md
clew case report -o report.json
clew case report -o report.pdf   # requires Typst

# Include all queries (not just marked)
clew case report --full
```

### Exporting for Audit

Create a zip archive with all case data:

```bash
clew case export
# ✓ Exported case to api-outage-2025-01-15.zip
# Archive contains:
#   - case.yaml (full case data)
#   - evidence/ (3 log entries)
#   - report.md
#   - report.pdf
```

### Closing and Managing Cases

```bash
# Close the active case
clew case close

# List all cases
clew case list
clew case list --status active
clew case list --status closed

# Reopen a case
clew case open api-outage-2025-01-15

# Delete a case
clew case delete api-outage-2025-01-15
clew case delete api-outage-2025-01-15 --force
```

### Full Investigation Workflow

```bash
# 1. Alert comes in - start investigation
clew case new "API latency spike 2025-01-15"

# 2. Check metrics to find the spike
clew metrics -n AWS/ApplicationELB -m TargetResponseTime -s 6h --period 5m --profile prod
# Spike at 14:30

# 3. Search logs around that time
clew around -g /app/api/logs --profile prod -t "2025-01-15T14:30:00Z" --window 5m -f "error|slow"
clew case mark  # Mark this query as significant

# 4. Collect key evidence
clew case keep 1 -a "First error in spike window"
clew case keep 3 -a "Stack trace shows connection timeout"

# 5. Search for related patterns
clew query @api -s 1h -f "connection pool"
clew case keep 2 -a "Pool exhaustion confirmed"

# 6. Add notes
clew case note "Correlates with deploy at 14:28 - checking release notes"
clew case note "v2.3.1 changed pool size from 100 to 10 (typo?)"

# 7. Set summary
clew case summary "Connection pool sized incorrectly in v2.3.1. Changed from 100 to 10 connections due to typo in config."

# 8. Generate report and export
clew case report -o report.pdf
clew case export

# 9. Close case
clew case close
```
