# clew

> **clew** *(n.)* - a ball of thread; from Greek mythology, the thread Ariadne gave Theseus to escape the Minotaur's labyrinth. Follow the clew through your logs.

[![CI](https://github.com/jmurray2011/clew/actions/workflows/ci.yml/badge.svg)](https://github.com/jmurray2011/clew/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/jmurray2011/clew)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A fast, focused CLI for CloudWatch Logs.

## Install

**Go install** (requires Go 1.23+):
```bash
go install github.com/jmurray2011/clew@latest
```

**Binary download** (no Go required):
```bash
# Linux (amd64)
curl -fsSL https://github.com/jmurray2011/clew/releases/latest/download/clew-$(curl -s https://api.github.com/repos/jmurray2011/clew/releases/latest | grep tag_name | cut -d'"' -f4)-linux-amd64 -o clew
chmod +x clew && sudo mv clew /usr/local/bin/

# macOS (Apple Silicon)
curl -fsSL https://github.com/jmurray2011/clew/releases/latest/download/clew-$(curl -s https://api.github.com/repos/jmurray2011/clew/releases/latest | grep tag_name | cut -d'"' -f4)-darwin-arm64 -o clew
chmod +x clew && sudo mv clew /usr/local/bin/
```

Or grab a binary from the [releases page](https://github.com/jmurray2011/clew/releases).

## How It Works

clew discovers log groups live from AWS. No aliases, no source mapping, no YAML that rots.

**Three layers:**

1. **Config** (optional) maps short names to AWS profiles — the only thing you configure
2. **Discovery** finds log groups by exact name, search pattern, or interactive picker
3. **Frecency** remembers what you query and suggests it next time

The first argument to any command is a log group specifier:

```bash
# Exact log group name
clew query -e prod /aws/lambda/my-func -s 1h -f "error"

# Search pattern — glob or regex, auto-selects if one match, prompts if many
clew query -e test "*syslog" -s 1h -f "error"
clew query -e test "api.*error" -s 1h -f "error"

# Interactive picker
clew query -e test ? -s 1h -f "error"

# Frecency — omit the log group, get prompted with your recent groups
clew query -e test -s 1h -f "error"
```

## Quick Start

```bash
# 1. Create config with your AWS profiles
clew init

# 2. Edit ~/.clew/config.yaml
#    profiles:
#      test: my_test_profile
#      prod: my_prod_profile
#    default: test

# 3. Discover what's there
clew groups -e test

# 4. Query
clew query -e test "*api*" -s 2h -f "error"
```

## Configuration

`~/.clew/config.yaml` — just profile shortcuts:

```yaml
profiles:
  test: my_test_profile    # short name -> AWS profile
  prod: my_prod_profile
  staging: my_staging_profile

default: test              # used when -e is omitted
region: us-east-1          # default region (override with -r)
```

That's the entire config. No sources, no aliases, no log group mapping.

Share across your team: `clew init --from https://github.com/myteam/dotfiles/raw/main/clew/config.yaml`

## Commands

| Command | Purpose | Example |
|---------|---------|---------|
| `query` | Search logs | `clew query -e test "*api*" -s 2h -f error` |
| `tail` | Follow logs in real-time | `clew tail -e prod /aws/lambda/my-func -f error` |
| `around` | Logs around a timestamp | `clew around -e prod /app/api -t "14:30" --utc -C 5` |
| `groups` | List log groups | `clew groups -e prod` |
| `streams` | List streams / active instances | `clew streams -e prod /app/api --stale 5m` |
| `init` | Create default config | `clew init` |

## Workflows

### Incident Response

```bash
# What just happened?
clew query -e prod "*api*" -s 15m -f "error|exception"

# Output shows UTC timestamps:
# 2025-03-15 11:17:23.000 | i-abc123
#   ERROR: connection pool exhausted

# Copy-paste that timestamp with --utc to see surrounding context
clew around -e prod /app/api -t "2025-03-15 11:17:23" --utc --stream i-abc123

# Need more context above a stack trace?
clew around -e prod /app/api -t "2025-03-15 11:17:23" --utc -C 10

# Or use local time if you're going by the clock on your wall
clew around -e prod /app/api -t "14:31" --stream i-abc123
```

### ASG Debugging

```bash
# Which instances are active?
clew streams -e prod /app/api -s 10m

# Which ones went quiet?
clew streams -e prod /app/api --stale 5m

# What was that instance doing?
clew query -e prod /app/api -s 1h --stream i-ghi789 -l 10
```

### Post-Deploy Verification

```bash
# Before/after deploy — are errors spiking?
clew query -e prod /app/api -s 30m -f "error" --stats
```

### Cross-Account Check

```bash
# Same log group, different environments
clew query -e test /app/api -s 30m -f "database timeout"
clew query -e prod /app/api -s 30m -f "database timeout"
```

### Piping to Other Tools

```bash
clew query -e test "*error*" -s 1h -f error -o json | jq '.[].message'
clew query -e test "*error*" -s 1d -f error -o csv > errors.csv
```

Status messages and color are automatically suppressed when piped.

## Query Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--since` | `-s` | Start time: relative (`2h`, `30m`) or absolute (`2025-01-15 14:30`) |
| `--until` | `-u` | End time (default: now) |
| `--filter` | `-f` | Case-insensitive regex filter against log messages |
| `--query` | `-q` | Raw CloudWatch Logs Insights query (overrides `-f`) |
| `--stream` | | Substring match on `@logStream` (e.g., `i-abc123`) |
| `--limit` | `-l` | Max results (default: 500) |
| `--context` | `-C` | Show N lines before each match (`query` and `around`) |
| `--output` | `-o` | Format: `text`, `json`, `csv` |
| `--export` | | Write results to a file |
| `--stats` | | Show match counts by 5-minute time bucket |

## Timestamps

Bare timestamps (no `Z` or `+/-` offset) are interpreted in **local time** by default.

| Input | Interpretation |
|-------|---------------|
| `2h`, `30m`, `7d` | Relative to now |
| `14:30` | Today, local time (`around` only) |
| `2025-01-15 14:30` | Local time |
| `2025-01-15T14:30:00Z` | Explicit UTC |
| `2025-01-15T14:30:00+01:00` | Explicit offset |

### The `--utc` flag (around)

Query output timestamps are always UTC. When you copy a timestamp from query output and paste it into `around -t`, use `--utc` so clew doesn't reinterpret it as local time:

```bash
# Query shows: 2025-03-15 11:17:23.000 (UTC)
# Without --utc, "11:17:23" is treated as 11:17 local time (wrong)
# With --utc, it's treated as 11:17 UTC (correct)
clew around -e prod /app/api -t "2025-03-15 11:17:23" --utc
```

Timestamps with explicit timezone indicators (`Z`, `+05:00`) are always respected regardless of `--utc`.

## IAM Permissions

```json
{
  "Effect": "Allow",
  "Action": [
    "logs:DescribeLogGroups",
    "logs:DescribeLogStreams",
    "logs:StartQuery",
    "logs:GetQueryResults",
    "logs:FilterLogEvents",
    "logs:GetLogEvents",
    "logs:GetLogRecord"
  ],
  "Resource": "arn:aws:logs:*:*:log-group:*"
}
```

## License

MIT
