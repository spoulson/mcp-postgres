# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build

```sh
go build -o mcp-postgres .
```

Docker single-platform:
```sh
docker build -t mcp-postgres .
```

Docker multi-platform (amd64 + arm64):
```sh
./build.sh
```

There are no tests in this project.

## Architecture

This is a Go MCP (Model Context Protocol) server that exposes PostgreSQL database introspection and querying capabilities to Claude. It communicates over stdio using the `mark3labs/mcp-go` library.

**Startup flow:** `main.go` connects to Postgres via `pgxpool`, pings it, creates the MCP server, calls `registerTools()`, then starts the stdio transport loop.

**Tool registration** (`tools.go`): All four tools are registered in `registerTools()`, which wires each tool definition to a handler factory. Each handler is a closure over the shared `*pgxpool.Pool`.

**Tool handlers** (one file per tool):
- `query.go` — `run_query`: enforces SELECT/WITH-only queries by prefix check, runs in a read-only transaction
- `schema.go` — `inspect_schema`: no table arg → lists all non-system tables; with table arg → returns columns, constraints, and indexes using `information_schema` and `pg_indexes`
- `explain.go` — `explain_query`: wraps query in `EXPLAIN (FORMAT TEXT, ANALYZE, BUFFERS)` inside a rolled-back read-only transaction so no data is modified; `analyze` param defaults to `true`
- `compliance.go` — `check_compliance`: loads rules from a JSON file at startup path, evaluates each rule by generating SQL, returns a report with pass/fail counts and up to 5 sample violation rows

**Compliance rules** (`compliance_rules.example.json`): The rules file is a JSON array under `"rules"`. Supported rule types: `not_null`, `regex`, `range`, `sql`. The `sql` type runs arbitrary SQL where any returned rows are treated as violations. `pgQuoteIdent`/`pgQuoteLiteral` in `compliance.go` handle safe SQL construction for the non-`sql` rule types.

## Environment variables

| Variable | Purpose |
|---|---|
| `POSTGRES_DSN` | Full connection string (overrides individual vars) |
| `POSTGRES_HOST/PORT/USER/PASSWORD/DB/SSLMODE` | Individual connection params |
| `COMPLIANCE_RULES_FILE` | Path to the compliance rules JSON file |
