package main

import (
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerTools(s *server.MCPServer, pool *pgxpool.Pool) {
	s.AddTool(mcp.NewTool("run_query",
		mcp.WithDescription("Execute a read-only SQL SELECT query against the PostgreSQL database and return results as JSON"),
		mcp.WithString("sql",
			mcp.Required(),
			mcp.Description("The SQL SELECT query to execute (only SELECT and WITH/CTE queries are permitted)"),
		),
	), newQueryHandler(pool))

	s.AddTool(mcp.NewTool("inspect_schema",
		mcp.WithDescription("Inspect the PostgreSQL database schema. Omit table to list all tables; provide a table name (optionally schema-qualified as schema.table) for full column, constraint, and index details"),
		mcp.WithString("table",
			mcp.Description("Optional table name to inspect (e.g. 'users' or 'public.users')"),
		),
	), newSchemaHandler(pool))

	s.AddTool(mcp.NewTool("explain_query",
		mcp.WithDescription("Show the query execution plan for a SQL query using EXPLAIN ANALYZE"),
		mcp.WithString("sql",
			mcp.Required(),
			mcp.Description("The SQL query to explain"),
		),
		mcp.WithBoolean("analyze",
			mcp.Description("Run EXPLAIN ANALYZE to include actual timing and row counts (default: true). The query is wrapped in a rolled-back transaction so no data is modified."),
		),
	), newExplainHandler(pool))

	rulesFile := os.Getenv("COMPLIANCE_RULES_FILE")
	s.AddTool(mcp.NewTool("check_compliance",
		mcp.WithDescription("Validate database data against regulatory compliance rules defined in a JSON rules file. Set COMPLIANCE_RULES_FILE env var to the path of your rules file."),
		mcp.WithString("table",
			mcp.Description("Optional: restrict checks to rules for this specific table"),
		),
	), newComplianceHandler(pool, rulesFile))
}
