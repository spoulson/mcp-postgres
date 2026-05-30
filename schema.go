package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mark3labs/mcp-go/mcp"
)

func newSchemaHandler(pool *pgxpool.Pool) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		table := req.GetString("table", "")

		if strings.TrimSpace(table) == "" {
			return listAllTables(ctx, pool)
		}
		return describeTable(ctx, pool, table)
	}
}

func listAllTables(ctx context.Context, pool *pgxpool.Pool) (*mcp.CallToolResult, error) {
	const q = `
		SELECT table_schema, table_name, table_type
		FROM information_schema.tables
		WHERE table_schema NOT IN ('pg_catalog', 'information_schema')
		ORDER BY table_schema, table_name`

	rows, err := pool.Query(ctx, q)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("schema query error: %v", err)), nil
	}
	defer rows.Close()

	type tableEntry struct {
		Schema string `json:"schema"`
		Name   string `json:"name"`
		Type   string `json:"type"`
	}
	var tables []tableEntry

	for rows.Next() {
		var e tableEntry
		if err := rows.Scan(&e.Schema, &e.Name, &e.Type); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		tables = append(tables, e)
	}
	if err := rows.Err(); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("rows error: %v", err)), nil
	}

	out, _ := json.MarshalIndent(tables, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

func describeTable(ctx context.Context, pool *pgxpool.Pool, tableArg string) (*mcp.CallToolResult, error) {
	schema, table := "public", tableArg
	if parts := strings.SplitN(tableArg, ".", 2); len(parts) == 2 {
		schema, table = parts[0], parts[1]
	}

	columns, err := queryColumns(ctx, pool, schema, table)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("columns error: %v", err)), nil
	}
	constraints, err := queryConstraints(ctx, pool, schema, table)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("constraints error: %v", err)), nil
	}
	indexes, err := queryIndexes(ctx, pool, schema, table)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("indexes error: %v", err)), nil
	}

	result := map[string]any{
		"schema":      schema,
		"table":       table,
		"columns":     columns,
		"constraints": constraints,
		"indexes":     indexes,
	}
	out, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

type columnInfo struct {
	Name       string  `json:"name"`
	Type       string  `json:"type"`
	MaxLength  *int    `json:"max_length,omitempty"`
	Nullable   bool    `json:"nullable"`
	Default    *string `json:"default,omitempty"`
}

func queryColumns(ctx context.Context, pool *pgxpool.Pool, schema, table string) ([]columnInfo, error) {
	const q = `
		SELECT column_name, data_type, character_maximum_length,
		       is_nullable = 'YES', column_default
		FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = $2
		ORDER BY ordinal_position`

	rows, err := pool.Query(ctx, q, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []columnInfo
	for rows.Next() {
		var c columnInfo
		if err := rows.Scan(&c.Name, &c.Type, &c.MaxLength, &c.Nullable, &c.Default); err != nil {
			return nil, err
		}
		cols = append(cols, c)
	}
	return cols, rows.Err()
}

type constraintInfo struct {
	Name          string  `json:"name"`
	Type          string  `json:"type"`
	Column        string  `json:"column"`
	ForeignTable  *string `json:"foreign_table,omitempty"`
	ForeignColumn *string `json:"foreign_column,omitempty"`
}

func queryConstraints(ctx context.Context, pool *pgxpool.Pool, schema, table string) ([]constraintInfo, error) {
	const q = `
		SELECT tc.constraint_name, tc.constraint_type, kcu.column_name,
		       ccu.table_name, ccu.column_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
		    ON tc.constraint_name = kcu.constraint_name
		    AND tc.table_schema = kcu.table_schema
		    AND tc.table_name = kcu.table_name
		LEFT JOIN information_schema.referential_constraints rc
		    ON tc.constraint_name = rc.constraint_name
		    AND tc.table_schema = rc.constraint_schema
		LEFT JOIN information_schema.constraint_column_usage ccu
		    ON rc.unique_constraint_name = ccu.constraint_name
		    AND rc.unique_constraint_schema = ccu.table_schema
		WHERE tc.table_schema = $1 AND tc.table_name = $2
		ORDER BY tc.constraint_type, tc.constraint_name`

	rows, err := pool.Query(ctx, q, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cons []constraintInfo
	for rows.Next() {
		var c constraintInfo
		if err := rows.Scan(&c.Name, &c.Type, &c.Column, &c.ForeignTable, &c.ForeignColumn); err != nil {
			return nil, err
		}
		cons = append(cons, c)
	}
	return cons, rows.Err()
}

type indexInfo struct {
	Name       string `json:"name"`
	Definition string `json:"definition"`
}

func queryIndexes(ctx context.Context, pool *pgxpool.Pool, schema, table string) ([]indexInfo, error) {
	const q = `
		SELECT indexname, indexdef
		FROM pg_indexes
		WHERE schemaname = $1 AND tablename = $2
		ORDER BY indexname`

	rows, err := pool.Query(ctx, q, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var idxs []indexInfo
	for rows.Next() {
		var idx indexInfo
		if err := rows.Scan(&idx.Name, &idx.Definition); err != nil {
			return nil, err
		}
		idxs = append(idxs, idx)
	}
	return idxs, rows.Err()
}
