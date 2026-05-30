package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mark3labs/mcp-go/mcp"
)

func newExplainHandler(pool *pgxpool.Pool) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		sql, err := req.RequireString("sql")
		if err != nil || strings.TrimSpace(sql) == "" {
			return mcp.NewToolResultError("sql parameter is required"), nil
		}

		analyze := req.GetBool("analyze", true)

		tx, err := pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
		if err != nil {
			return nil, fmt.Errorf("begin transaction: %w", err)
		}
		defer tx.Rollback(ctx)

		var explainSQL string
		if analyze {
			explainSQL = "EXPLAIN (FORMAT TEXT, ANALYZE, BUFFERS) " + sql
		} else {
			explainSQL = "EXPLAIN (FORMAT TEXT) " + sql
		}

		rows, err := tx.Query(ctx, explainSQL)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("explain error: %v", err)), nil
		}
		defer rows.Close()

		var lines []string
		for rows.Next() {
			var line string
			if err := rows.Scan(&line); err != nil {
				return nil, fmt.Errorf("scan: %w", err)
			}
			lines = append(lines, line)
		}
		if err := rows.Err(); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("rows error: %v", err)), nil
		}

		return mcp.NewToolResultText(strings.Join(lines, "\n")), nil
	}
}
