# Usage

Complete reference for every flag and environment variable accepted by
`graytail` and `grayquery`, plus worked examples. Every example command below
has been run against a live Graylog instance to confirm it behaves as
documented; example *output* shown uses placeholder data (host names,
messages) rather than real captured log content.

For installation, see [INSTALL.md](INSTALL.md). For a quick overview, see the
[README](README.md).

## Configuration precedence

Every setting below (shared or binary-specific) resolves in this order,
highest priority first:

1. **command-line flag**
2. **environment variable** (real, exported)
3. **`.env` file** — loaded once at startup from the *current working
   directory* (not the binary's location); only fills in variables not
   already set in step 2
4. **built-in default**

## Shared settings

These apply to both `graytail` and `grayquery`.

| Env var | Flag | Default | Notes |
|---|---|---|---|
| `GRAYLOG_URL` | `-url` | *(required)* | Graylog base URL, e.g. `https://graylog.example.com:9000`. A trailing `/` is stripped automatically. |
| `GRAYLOG_API_TOKEN` | `-token` | *(required)* | Graylog API token. Sent as HTTP Basic Auth with the token as the username and the literal string `token` as the password — this is Graylog's documented token-auth convention, not a real password. |
| `GRAYLOG_STREAM_ID` | `-stream` | `000000000000000000000001` | Stream to filter on. If left empty, it's filled in with Graylog's built-in "All Messages" stream ID before the first request, so every query is stream-scoped even if you never set this. Sent as `filter=streams:<id>`. |
| `TIMEOUT` | `-timeout` | `60s` | Per-request timeout (Go duration syntax, e.g. `30s`, `2m`). Applies to each individual HTTP request, not the whole process lifetime — `graytail` makes one request per poll cycle, `grayquery` makes one per page. |
| `INSECURE` | `-insecure` | `false` | Skips TLS certificate verification. Only use this for self-signed certs on a trusted network — it disables protection against man-in-the-middle attacks. |
| `LIMIT` | `-limit` | `100` | Max messages requested per HTTP call. For `graytail` this bounds each poll; for `grayquery` it's the page size used while paginating. |

## graytail

Continuously polls Graylog's relative search endpoint and prints newly
arrived messages, like `tail -f`. Runs until interrupted (Ctrl+C / SIGTERM).

### graytail-specific settings

| Env var | Flag | Default | Notes |
|---|---|---|---|
| `POLL` | `-poll` | `10s` | Delay between the end of one poll cycle and the start of the next. |
| `LOOKBACK` | `-lookback` | `30` | Initial lookback window, in seconds, used for the very first query (before any message has been seen). Also used as a fallback range if the last-seen timestamp can't be parsed. Passed straight through to Graylog's relative-search `range` parameter, hence it's a string, not a number. |
| `DUP_RETENTION` | `-dup-retention` | `1h` | How long a message's dedup key is remembered. The in-memory dedup map is pruned every 5 minutes, discarding any key older than this. |

### How the poll loop works

- **Query window**: the first request uses `range=<lookback>` seconds. After
  that, `graytail` tracks the latest message timestamp it has seen and
  recomputes `range` on each subsequent poll as *(seconds since that
  timestamp) + 5s buffer*, clamped between 5s and 3600s (1h) — this narrows
  the query window over time instead of re-fetching a fixed range forever. If
  the last-seen timestamp ever fails to parse as RFC3339 (shouldn't normally
  happen), it falls back to the fixed `-lookback` range and adds
  `timestamp:><lastseen>` to the query as a safety net.
- **Deduplication**: each message is keyed on `timestamp|source|message`.
  Only messages not already in the in-memory `seen` map are printed; already-
  seen keys are silently skipped on later polls (Graylog's relative-range
  windows can overlap between polls by design, which is what makes this
  necessary).
- **Error handling**, per response:
  - **Auth error** (401/403) — fatal; the process logs the error and exits
    with status `1`.
  - **Server error** (5xx) — logged, then retried with exponential backoff
    starting at 1s and doubling up to a 32s cap; backoff resets to 1s after
    any successful request.
  - **Other API error / JSON decode error** — logged, then the loop just
    waits one normal `-poll` interval and retries (no backoff escalation).
  - **Network error** (e.g. DNS failure, connection refused) — same
    exponential backoff as a server error, unless shutdown was already
    requested, in which case it exits immediately instead of retrying.
- **Shutdown**: SIGINT or SIGTERM is caught; the loop finishes its current
  step, logs `shutting down`, and exits `0`. In-flight requests are also
  bounded by the shutdown context, so a slow request doesn't block exit.
- **Output**: one line per new message, format `[timestamp] source: message`
  — any field Graylog's response doesn't include renders as `<nil>`.

### graytail examples (verified)

```bash
# Basic tail with defaults (10s poll, 30s initial lookback)
graytail
```

```bash
# Poll every 5 seconds, look back 5 minutes on startup
graytail -poll 5s -lookback 300
```

Sample output (placeholder data):

```
📋 Tailing Graylog logs... Press Ctrl+C to stop.
[2026-08-20T13:26:01.123Z] app-server-01: request completed in 42ms
[2026-08-20T13:26:03.870Z] worker-02: job finished successfully
```

Stop with Ctrl+C (or send SIGTERM); confirmed it logs `shutting down` and
exits cleanly rather than hanging or exiting abruptly.

```bash
# Skip TLS verification for a self-signed Graylog instance
graytail -insecure
```

```bash
# Bad credentials — confirmed behavior: logs the auth error and exits status 1
graytail -token invalid-token
# 2026/08/20 13:26:12 authentication error: 401 Unauthorized - ...
```

## grayquery

Runs a single bounded search over a `--begin`/`--end` time window against
Graylog's keyword search endpoint, paginating until all matches are fetched
(or a safety cap is hit), then prints them sorted ascending by timestamp.

### grayquery-specific settings

| Env var | Flag | Default | Notes |
|---|---|---|---|
| — | `--begin`, `-b` | — | Start of the time window. At least one of `--begin`/`--end` is required. |
| — | `--end`, `-e` | — | End of the time window. |
| — | `--query`, `-q` | `*` | Search query string. May also be given positionally (see below); defaults to `*` (match everything) if left empty. |
| — | `--format`, `-o` | `line` | Output format: `line`, `json`, or `fields`. |
| — | `--fields` | — | Comma-separated field list; required (non-empty) only when `--format fields`. Entries are trimmed of whitespace; blank entries are dropped. |
| `TIMEZONE` | `--timezone`, `-tz` | `UTC` | Timezone Graylog uses to resolve `--begin`/`--end`. Graylog requires a non-empty value here, so leaving this unset always falls back to `UTC` rather than an empty string. |

### How the search works

- **Time window**: `--begin` and `--end` are combined into a single Graylog
  "keyword" time expression — the same free-text parser Graylog's own search
  bar uses — as `"<begin> to <end>"` if both are given, or just `<begin>` /
  `<end>` alone if only one is. Both absolute (`"2024-01-02 15:04:05"`) and
  relative (`"2 hours ago"`, `"yesterday"`) forms are accepted; both were
  exercised against a live server while writing this doc and returned
  consistent results.
- **Query source**: if `--query`/`-q` is empty, any positional arguments
  after the flags are joined with spaces and used as the query instead (so
  `grayquery -b "1 hour ago" source:web AND level:3` works without needing
  `-q`). If `--query` is explicitly set, positional arguments are ignored.
- **Pagination**: fetches pages of `-limit` (default 100) messages via
  `offset`/`limit`, stopping when a page comes back shorter than `-limit`
  (i.e. the last page) or when the total reaches a hard safety cap of
  **100,000 messages** — whichever comes first. This cap exists to keep an
  unbounded query from exhausting memory; it does not adjust or warn, it just
  stops fetching.
- **Sorting**: once all pages are collected, results are sorted ascending by
  `timestamp` (RFC3339, with a plain string-comparison fallback if a
  timestamp fails to parse) — independent of what order Graylog returned
  pages in.
- **Exit codes**: `0` on success (a query matching **zero** messages is still
  a success — confirmed it prints nothing and exits `0`, not an error);
  general/validation errors (bad `--format`, missing `--begin`/`--end`, etc.)
  exit `1`; an auth error (401/403) specifically exits `2`.

### Output formats

- **`line`** (default) — `[timestamp] source: message`, same as `graytail`.
- **`json`** — the complete raw message object as compact single-line JSON,
  one per line. This includes *every* field Graylog returned for that
  message, not just timestamp/source/message.
- **`fields`** — tab-separated values, in the exact order given to
  `--fields`. A field missing from a given message renders as an empty
  column rather than erroring.

### grayquery examples (verified)

```bash
# Absolute time window, default line format, all messages
grayquery -b "2024-01-02 15:04:05" -e "2024-01-02 16:04:05" "*"
```

```bash
# Relative time, default query (defaults to "*")
grayquery -b "2 hours ago"
```

Sample output (placeholder data):

```
[2026-08-20T11:30:02.001Z] app-server-01: startup complete
[2026-08-20T12:15:47.512Z] worker-02: job finished successfully
```

```bash
# JSON output, one compact object per line
grayquery -b "2 hours ago" -o json
```

```json
{"message":"job finished successfully","source":"worker-02","timestamp":"2026-08-20T12:15:47.512Z"}
```

```bash
# Fields output — tab-separated, in the order requested
grayquery -b "2 hours ago" -o fields --fields timestamp,source
```

```
2026-08-20T12:15:47.512Z	worker-02
```

```bash
# Positional query instead of --query/-q
grayquery -b "2 hours ago" source:web
```

```bash
# Explicit stream filter and a longer per-request timeout
grayquery -b "1 hour ago" -stream 000000000000000000000001 -timeout 2m "*"
```

```bash
# Missing --begin and --end — confirmed error, exit status 1
grayquery -o line "*"
# 2026/08/20 13:25:45 at least one of --begin/-b or --end/-e must be provided
```

```bash
# --format fields without --fields — confirmed error, exit status 1
grayquery -b "2 hours ago" -o fields
# 2026/08/20 13:25:45 --format fields requires a non-empty --fields list
```

```bash
# Bad credentials — confirmed behavior: logs the auth error and exits status 2
grayquery -token invalid-token -b "1 hour ago"
# 2026/08/20 13:26:07 authentication error: 401 Unauthorized - ...
```

## Using a `.env` file instead of flags

Both binaries load `.env` from the current working directory at startup (see
[INSTALL.md](INSTALL.md#environment-variables-macoslinux-shells) for the
CWD caveat and the exported-environment-variable alternative). With a `.env`
in place, the commands above shrink to just the parts that change per
invocation:

```bash
# .env already provides GRAYLOG_URL / GRAYLOG_API_TOKEN / etc.
grayquery -b "2 hours ago" -o json
graytail -poll 5s
```
