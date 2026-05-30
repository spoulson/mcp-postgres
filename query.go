package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mark3labs/mcp-go/mcp"
)

func newQueryHandler(pool *pgxpool.Pool) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		sql, err := req.RequireString("sql")
		if err != nil || strings.TrimSpace(sql) == "" {
			return mcp.NewToolResultError("sql parameter is required"), nil
		}

		upper := strings.ToUpper(strings.TrimSpace(sql))
		if !strings.HasPrefix(upper, "SELECT") && !strings.HasPrefix(upper, "WITH") {
			return mcp.NewToolResultError("only SELECT and WITH (CTE) queries are allowed"), nil
		}

		tx, err := pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
		if err != nil {
			return nil, fmt.Errorf("begin transaction: %w", err)
		}
		defer tx.Rollback(ctx)

		rows, err := tx.Query(ctx, sql)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("query error: %v", err)), nil
		}
		defer rows.Close()

		fieldDescs := rows.FieldDescriptions()
		var results []map[string]any

		for rows.Next() {
			values, err := rows.Values()
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("row scan error: %v", err)), nil
			}
			row := make(map[string]any, len(fieldDescs))
			for i, fd := range fieldDescs {
				row[fd.Name] = values[i]
			}
			results = append(results, row)
		}
		if err := rows.Err(); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("rows error: %v", err)), nil
		}

		if results == nil {
			results = []map[string]any{}
		}

		out, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("marshal results: %w", err)
		}
		return mcp.NewToolResultText(string(out)), nil
	}
}
