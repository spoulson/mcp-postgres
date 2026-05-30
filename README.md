# MCP Server for PostgresSQL
MCP Server for Claude to communicate with PostgreSQL databases.

## Build

### Native executable

#### Build prerequisites
- Golang 1.25+

```sh
$ go build -o mcp-postgres .
```

### Docker build
```sh
$ docker build -t mcp-postgres .
```

Or build multiplatform for both amd64 and arm64:
```sh
$ ./build.sh
```

This builds two image tags: `mcp-postgres:latest` amd `mcp-postgres:latest-arm64`.

## Claude Desktop configuration

Claude has an edit button at Settings -> Developer -> Edit Config button.  Add/edit the `mcpServers` stanza to add the MCP server.  Then restart Claude.

### Run native
Build executable and run natively:
```json
{
  "mcpServers": {
    "postgres": {
      "command": "/home/spoulson/src/mcp-postgres/mcp-postgres",
      "env": {
        "POSTGRES_DSN": "postgres://user:pass@localhost:5432/mydb",
        "COMPLIANCE_RULES_FILE": "/home/spoulson/src/mcp-postgres/compliance_rules.json"
      }
    }
  }
}
```

### Run in Docker
Build Docker image and run in container:
```json
{
  "mcpServers": {
    "postgres": {
      "command": "docker",
      "args": [
        "run",
        "--rm",
        "-i",
        "-e",
        "POSTGRES_DSN",
        "-e",
        "COMPLIANCE_RULES_FILE",
        "mcp-postgres:latest"
      ],
      "env": {
        "POSTGRES_DSN": "postgres://user:pass@localhost:5432/mydb",
        "COMPLIANCE_RULES_FILE": "/home/spoulson/src/mcp-postgres/compliance_rules.json"
      }
    }
  }
}
```

## Notes from Claude
    ● Done. Here's a summary of what was built and how to wire it up in Claude Desktop:

    ---
    MCP Postgres Server

    Four tools exposed to Claude:

    ┌──────────────────┬───────────────────────────────────────────────────────────────────────────────────┐
    │       Tool       │                                   What it does                                    │
    ├──────────────────┼───────────────────────────────────────────────────────────────────────────────────┤
    │ run_query        │ Executes read-only SELECT/WITH queries, returns JSON rows                         │
    ├──────────────────┼───────────────────────────────────────────────────────────────────────────────────┤
    │ inspect_schema   │ Lists all tables, or full column/constraint/index detail for one table            │
    ├──────────────────┼───────────────────────────────────────────────────────────────────────────────────┤
    │ explain_query    │ Returns EXPLAIN (ANALYZE, BUFFERS) output (query runs in a rolled-back tx)        │
    ├──────────────────┼───────────────────────────────────────────────────────────────────────────────────┤
    │ check_compliance │ Runs rules from your JSON rules file and reports pass/fail with violation samples │
    └──────────────────┴───────────────────────────────────────────────────────────────────────────────────┘

    Configure via environment variables:
    POSTGRES_DSN=postgres://user:pass@host:5432/dbname
    # OR individual vars:
    POSTGRES_HOST, POSTGRES_PORT, POSTGRES_USER, POSTGRES_PASSWORD, POSTGRES_DB, POSTGRES_SSLMODE

    COMPLIANCE_RULES_FILE=/path/to/your/rules.json

    Wire into Claude Desktop — add to ~/Library/Application Support/Claude/claude_desktop_config.json (macOS):
    {
      "mcpServers": {
      "postgres": {
        "command": "/home/spoulson/src/mcp-postgres/mcp-postgres",
          "env": {
            "POSTGRES_DSN": "postgres://user:pass@localhost:5432/mydb",
            "COMPLIANCE_RULES_FILE": "/home/spoulson/src/mcp-postgres/compliance_rules.json"
          }
        }
      }
    }

    Compliance rules — copy compliance_rules.example.json as a starting point. Supported rule types: not_null, regex, range, and sql (custom SQL returning violation rows).
