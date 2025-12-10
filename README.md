# clew

> **clew** *(n.)* - a ball of thread; from Greek mythology, the thread Ariadne gave Theseus to escape the Minotaur's labyrinth. Follow the clew through your logs.

[![CI](https://github.com/jmurray2011/clew/actions/workflows/ci.yml/badge.svg)](https://github.com/jmurray2011/clew/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/jmurray2011/clew)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A command-line tool for querying logs from multiple sources including AWS CloudWatch Logs and local files.

## Installation

```bash
go build -o clew .
```

## Quick Start

```bash
# Query CloudWatch logs (use -p for profile, -r for region)
clew query cloudwatch:///app/logs -p prod -r us-east-1 -s 2h -f "error|exception"

# Query local log files
clew query /var/log/app.log -s 2h -f "error"

# Use source aliases (after configuration)
clew query @prod-api -s 1h -f "timeout"

# List available CloudWatch log groups
clew groups -p prod -r us-east-1

# Tail logs in real-time
clew tail cloudwatch:///app/logs -p prod -f "error"

# Discover fields in structured logs (WAF, JSON)
clew fields cloudwatch:///aws-waf-logs-MyALB -p prod
```

## Source URIs

clew uses URIs to identify log sources:

| Format | Description |
|--------|-------------|
| `cloudwatch:///log-group` | AWS CloudWatch Logs (use `-p`/`-r` flags for profile/region) |
| `file:///path/to/file.log` | Local file (explicit) |
| `/var/log/app.log` | Local file (shorthand) |
| `@alias-name` | Configured source alias |

## Commands

| Command | Description |
|---------|-------------|
| `init` | Create default config and history files |
| `query` | Query logs from any source (CloudWatch, local files) |
| `around` | Query logs around a specific timestamp |
| `sources` | List configured source aliases |
| `groups` | List available CloudWatch log groups |
| `streams` | List log streams in a group |
| `tail` | Follow CloudWatch logs in real-time |
| `get` | Fetch a specific log event by pointer |
| `metrics` | Query CloudWatch Metrics (identify spikes) |
| `retention` | View log group retention settings |
| `history` | View and re-run past queries |
| `fields` | Discover available fields in a log group |
| `case` | Manage investigation cases (evidence, timeline, reports) |
| `completion` | Generate shell completions (bash/zsh/fish/powershell) |

## Configuration

Create `~/.clew/config.yaml`:

```yaml
# Default AWS settings
profile: my-aws-profile
region: us-east-1

# Output settings
output:
  format: text      # text, json, csv
  timestamps: local # local, utc

# History settings
history_max: 50                          # Max entries to keep (default: 50)
history_file: ~/.clew_history.json       # Custom location (optional)

# Source aliases - use with @alias-name (use -p/-r flags for profile/region)
sources:
  prod-api:
    uri: cloudwatch:///app/api/prod
  staging:
    uri: cloudwatch:///app/api/staging
  tomcat:
    uri: cloudwatch:///app/tomcat/logs
  waf:
    uri: cloudwatch:///aws-waf-logs-MyALB
  local:
    uri: file:///var/log/app.log
    format: java    # plain, json, syslog, java

# Default source when none specified
default_source: prod-api

# Saved queries (legacy format, still supported)
queries:
  errors:
    log_group: tomcat
    filter: "exception|error"
    start: 2h
```

## Using Source Aliases

Once configured, use `@alias-name` to reference sources:

```bash
# Instead of the full URI with profile flag:
clew query cloudwatch:///app/tomcat/logs -p prod -s 1h -f "error"

# Use the source alias (set profile/region in config or use -p/-r flags):
clew query @tomcat -p prod -s 1h -f "error"

# Query local files
clew query @local -s 1h -f "exception"

# List configured aliases
clew sources
```

Source aliases are resolved automatically and support both CloudWatch and local file sources.

## Filtering

The `-f` flag accepts a regex pattern that matches against log messages (case-insensitive):

```bash
# Simple text match
clew query -g tomcat -s 1h -f "error"

# OR matching with pipe
clew query -g tomcat -s 1h -f "error|exception|timeout"

# Regex patterns
clew query -g tomcat -s 1h -f "user-[0-9]+"
```

Under the hood, `-f "pattern"` generates a CloudWatch Logs Insights query:
```
filter @message like /(?i)(pattern)/
```

For complex queries, use `-q` to write the full Insights query directly (this overrides `-f`).

## Features

- **Multi-source support**: Query CloudWatch Logs, local files, and more
- **Source aliases**: Define shortcuts for frequently used sources
- **Local file parsing**: Auto-detect or specify format (plain, JSON, syslog, Java stack traces)
- **Query history**: View and re-run past queries with `clew history --run N`
- **Case management**: Track investigations, collect evidence, generate reports
- **Cost estimation**: Preview CloudWatch query cost with `--dry-run` (rough estimate)
- **Field discovery**: Find available fields in JSON/structured logs
- **Multiple output formats**: text, json, csv
- **Context lines**: Show surrounding log lines with `-C`
- **Around mode**: Query logs around a specific timestamp with `clew around`
- **Watch mode**: Repeat queries at intervals with `--watch N` for monitoring
- **AWS Console URLs**: Generate clickable console links with `--url`
- **Pattern highlighting**: Filter matches are highlighted in text output
- **CloudWatch Metrics**: Query metrics to identify spikes, then pivot to logs
- **Verbose mode**: Debug with `-v` flag

## IAM Permissions (CloudWatch)

The following IAM permissions are required for CloudWatch Logs sources:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "logs:DescribeLogGroups",
        "logs:DescribeLogStreams",
        "logs:StartQuery",
        "logs:GetQueryResults",
        "logs:StopQuery",
        "logs:FilterLogEvents",
        "logs:GetLogEvents",
        "logs:GetLogRecord"
      ],
      "Resource": "arn:aws:logs:*:*:log-group:*"
    }
  ]
}
```

For CloudWatch Metrics (`clew metrics`), add:

```json
{
  "Effect": "Allow",
  "Action": [
    "cloudwatch:GetMetricStatistics",
    "cloudwatch:ListMetrics"
  ],
  "Resource": "*"
}
```

**Minimum permissions** (read-only queries):
- `logs:DescribeLogGroups`
- `logs:StartQuery`
- `logs:GetQueryResults`

## Documentation

See [EXAMPLES.md](EXAMPLES.md) for detailed usage examples.

## License

MIT
