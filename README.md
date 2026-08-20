# graytail

Command-line tools for tailing and querying [Graylog](https://www.graylog.org/) over its HTTP search API — no browser required.

- **`graytail`** — continuously polls Graylog and streams new messages to your terminal, like `tail -f`.
- **`grayquery`** — runs a one-shot, bounded search over a time window and prints matching messages, sorted by time.

Both are stdlib-only Go binaries that share configuration, an HTTP client, and output formatting.

## Installation

Requires Go 1.23.1+.

```bash
git clone git@github.com:bytetwiddler/graytail.git
cd graytail
make build
```

This builds `bin/graytail` and `bin/grayquery` for your host platform. To cross-compile for other platforms:

```bash
make build-all   # linux/darwin/windows x amd64/arm64 -> bin/<os>/<arch>/
```

## Configuration

Both tools read the same connection settings, resolved in this order of precedence: **command-line flag > environment variable > `.env` file > default**.

Copy `.env.example` to `.env` and fill in your values, or export the environment variables directly:

```bash
cp .env.example .env
```

| Variable | Flag | Default | Description |
|---|---|---|---|
| `GRAYLOG_URL` | `-url` | *(required)* | Graylog base URL, e.g. `https://graylog.example.com:9000` |
| `GRAYLOG_API_TOKEN` | `-token` | *(required)* | Graylog API token |
| `GRAYLOG_STREAM_ID` | `-stream` | all messages | Stream ID to filter on |
| `TIMEOUT` | `-timeout` | `60s` | Per-request timeout |
| `INSECURE` | `-insecure` | `false` | Skip TLS certificate verification |
| `LIMIT` | `-limit` | `100` | Max messages fetched per request |

`.env` is loaded on startup if present and never overrides variables already set in the environment. It's git-ignored — never commit real credentials.

## Usage

### graytail — live tail

```bash
graytail
```

Polls Graylog and prints new messages as `[timestamp] source: message`. Stop with Ctrl+C.

Additional flags:

| Variable | Flag | Default | Description |
|---|---|---|---|
| `POLL` | `-poll` | `10s` | Polling interval between requests |
| `LOOKBACK` | `-lookback` | `30` | Initial lookback window, in seconds |
| `DUP_RETENTION` | `-dup-retention` | `1h` | How long to remember seen messages to avoid duplicates |

### grayquery — bounded search

```bash
grayquery -b "2024-01-02 15:04:05" -e "2024-01-02 16:04:05" -o line "source:web AND level:3"
```

Runs a single bounded search between `--begin`/`-b` and `--end`/`-e` and prints matches in ascending timestamp order, paginating automatically until all results are fetched.

The begin/end bounds are Graylog "keyword" time expressions — the same natural-language parser Graylog's own search bar uses — so absolute (`"2024-01-02 15:04:05"`) and relative (`"1 hour ago"`, `"yesterday"`) forms both work:

```bash
grayquery -b "1 hour ago" "*"
grayquery -b "2 hours ago" -e "1 hour ago" "source:web"
```

At least one of `--begin`/`--end` is required. The query itself may be given with `--query`/`-q` or positionally (as in the examples above); it defaults to `*`.

| Variable | Flag | Default | Description |
|---|---|---|---|
| — | `--begin`, `-b` | — | Start of the time window |
| — | `--end`, `-e` | — | End of the time window |
| — | `--query`, `-q` | `*` | Graylog search query string (may also be positional) |
| — | `--format`, `-o` | `line` | Output format: `line`, `json`, or `fields` |
| — | `--fields` | — | Comma-separated field list, required when `--format fields` |
| `TIMEZONE` | `--timezone`, `-tz` | `UTC` | Timezone used to resolve `--begin`/`--end` |

Output formats:

- `line` — `[timestamp] source: message`
- `json` — the raw message as compact JSON, one per line
- `fields` — tab-separated values for the fields listed in `--fields`, e.g. `--format fields --fields source,message`

## Development

```bash
go build ./...      # compile everything
go test ./...        # run all tests
go test ./internal/graylog/... -run TestSearchKeyword   # run a single test
```

## Project layout

```
cmd/
  graytail/   # live-tail entry point
  grayquery/  # bounded-search entry point
internal/
  graylog/    # shared config, HTTP client, and output formatting
```

## License

[MIT](LICENSE)
