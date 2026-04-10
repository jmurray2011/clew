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

## AWS Authentication

clew uses the standard AWS SDK credential chain — whatever works for `aws` CLI works for clew. It does not handle authentication itself.

**Supported:**
- **SSO profiles** — run `aws sso login --profile <name>` first, then use clew
- **IAM access keys** in `~/.aws/credentials`
- **Instance/task roles** (EC2, ECS, Lambda) via IMDS
- **Environment variables** (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_SESSION_TOKEN`)
- **Assume-role profiles** (via `source_profile` / `role_arn` in `~/.aws/config`)

**For SSO users** (most common):

```bash
# Log in once per session
aws sso login --profile my_prod_profile

# Then use clew with the environment shortcut
clew query -e prod /app/api -s 1h -f "error"
```

If you see a credentials error, the usual fix is `aws sso login --profile <name>`.

## The `-e` Flag

`-e` (short for *environment*) selects which AWS profile to use. It's a thin wrapper over `--profile` with one extra trick: if your config has a `profiles` map, `-e` resolves the short name to the full AWS profile.

**Without config** — `-e` passes straight through to AWS as a profile name:

```bash
clew query -e my_prod_profile /app/api -s 1h
# equivalent to: AWS_PROFILE=my_prod_profile clew query ...
```

**With config** — `-e` becomes a short alias:

```yaml
# ~/.clew/config.yaml
profiles:
  prod: my_prod_profile     # -e prod   → AWS_PROFILE=my_prod_profile
  test: my_test_profile     # -e test   → AWS_PROFILE=my_test_profile
  staging: assume-role-abc  # -e staging → AWS_PROFILE=assume-role-abc
default: test
```

```bash
clew query -e prod /app/api -s 1h
clew query /app/api -s 1h           # uses default: test
```

**Override the region** with `-r`:

```bash
clew query -e prod -r eu-west-1 /app/api -s 1h
```

## Quick Start

```bash
# 1. Log in (if using SSO)
aws sso login --profile my_prod_profile

# 2. Create config with your AWS profile shortcuts
clew init

# 3. Edit ~/.clew/config.yaml:
#    profiles:
#      test: my_test_profile
#      prod: my_prod_profile
#    default: test

# 4. Discover what's there
clew groups -e test

# 5. Query
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

Run `clew <command> --help` for the full flag list on each command.

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

Every query result shows the log stream name — typically the instance ID for an EC2/ASG fleet. That makes it easy to narrow down to one box:

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
# Before/after deploy — are errors spiking? (counts per 5-min bucket)
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

Status messages and color are automatically suppressed when stdout is piped.

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

### Filters (`-f`)

`-f` is a **case-insensitive Go regex** matched against `@message`. Under the hood clew translates it to a CloudWatch Logs Insights `filter @message like /(?i)(your-pattern)/` clause. For most day-to-day searches this is all you need.

**Simple matches:**

```bash
-f "error"                    # any line containing "error" (case-insensitive)
-f "OutOfMemory"              # literal string
```

**Alternation (OR):**

```bash
-f "error|exception|timeout"  # any of these three
-f "5[0-9]{2}"                # HTTP 5xx codes: 500, 503, 599, etc.
```

**Patterns:**

```bash
-f "user.*login"              # "user" followed by "login" (with anything between)
-f "user-[0-9]+"              # user-123, user-4567, etc.
-f "status=[45][0-9]{2}"      # status=400 through status=599
-f "took [0-9]{4,}ms"         # requests taking 1000ms+ (4+ digits)
```

**Escaping special characters** — because `-f` is a regex, you need to escape regex metacharacters (`.`, `*`, `+`, `?`, `(`, `)`, `[`, `]`, `{`, `}`, `|`, `\`, `^`, `$`) if you want a literal match:

```bash
-f "\[ERROR\]"                # literal "[ERROR]"
-f "host=api\.example\.com"   # literal dots
-f "\\$\\{var\\}"             # literal "${var}"
```

**Combining with `--stream`** — filter by instance and message at the same time:

```bash
clew query -e prod /app/api -s 1h --stream i-abc123 -f "error|timeout"
```

### Raw queries (`-q`)

When `-f` isn't enough — when you need `stats`, `parse`, sorting, multi-field filtering, or any other Insights feature — use `-q` to pass a full CloudWatch Logs Insights query. `-q` overrides `-f`.

**Top error messages:**

```bash
clew query -e prod /app/api -s 1h \
  -q 'fields @message | filter @message like /ERROR/ | stats count() as c by @message | sort c desc | limit 20'
```

**5xx response codes over time:**

```bash
clew query -e prod /app/api -s 1h \
  -q 'fields @timestamp, @message | parse @message "status=*" as status | filter status >= 500 | stats count() by bin(1m)'
```

**Slowest requests:**

```bash
clew query -e prod /app/api -s 1h \
  -q 'fields @timestamp, @message | parse @message "took *ms" as ms | sort ms desc | limit 20'
```

See the [CloudWatch Logs Insights query syntax reference](https://docs.aws.amazon.com/AmazonCloudWatch/latest/logs/CWL_QuerySyntax.html) for the full language.

### Quick stats with `--stats`

If you just want counts over time (e.g., "are errors spiking?") without writing a raw query, `--stats` turns any `-f` filter into a per-5-minute bucket count:

```bash
clew query -e prod /app/api -s 1h -f "error" --stats
# 2025-03-15 14:25:00    47
# 2025-03-15 14:20:00     3
# 2025-03-15 14:15:00     1
```

### Result limits

CloudWatch Logs Insights caps results at 10,000 per query. `--limit` caps client-side below that. If you need to dump an entire stream (more than 10k lines), use `aws logs get-log-events` directly — clew is tuned for interactive investigation, not bulk exports.

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

Minimum permissions for clew to work against a log group:

```json
{
  "Version": "2012-10-17",
  "Statement": [{
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
  }]
}
```

If you're using SSO, your permission set probably already includes these via `CloudWatchLogsReadOnlyAccess` or `ReadOnlyAccess`.

## Troubleshooting

**`NoCredentialProviders` / `ExpiredToken`**
Your AWS credentials are missing or expired. For SSO: `aws sso login --profile <name>`.

**`source "test" not found`**
You used `-e test` but there's no `test` key in your config's `profiles` map and no AWS profile literally named `test`. Either add it to `~/.clew/config.yaml` or use the real profile name directly.

**`no log groups matching "foo"`**
The pattern didn't match any log groups in that account/region. Run `clew groups -e <env>` to see what exists.

**Multiple matches in a non-interactive context**
When stdout is piped, clew can't prompt you to pick. Narrow the pattern until it matches exactly one log group, or use the exact name.

**Results truncated before covering the full window (around)**
You hit `-l`. Increase the limit (`-l 1000`) or add a filter (`-f "error"`) to cut noise.

## License

MIT
