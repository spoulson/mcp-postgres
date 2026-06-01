package main

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	ctx := context.Background()

	pgc, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("testdb"),
		tcpostgres.WithUsername("testuser"),
		tcpostgres.WithPassword("testpass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		panic("start postgres container: " + err.Error())
	}
	defer func() {
		_ = pgc.Terminate(ctx)
	}()

	dsn, err := pgc.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		panic("get connection string: " + err.Error())
	}

	testPool, err = pgxpool.New(ctx, dsn)
	if err != nil {
		panic("create pool: " + err.Error())
	}
	defer testPool.Close()

	if _, err := testPool.Exec(ctx, testSchema); err != nil {
		panic("seed schema: " + err.Error())
	}

	os.Exit(m.Run())
}

const testSchema = `
CREATE TABLE users (
	id    SERIAL PRIMARY KEY,
	name  TEXT NOT NULL,
	email TEXT,
	score NUMERIC
);
INSERT INTO users (name, email, score) VALUES
	('Alice',   'alice@example.com', 95),
	('Bob',     'bob@example.com',   85),
	('Charlie', NULL,               105),
	('Dave',    'not-an-email',      -5);
`

func toolReq(args map[string]any) mcp.CallToolRequest {
	r := mcp.CallToolRequest{}
	r.Params.Arguments = args
	return r
}

func mustText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", firstText(res))
	}
	return firstText(res)
}

func firstText(res *mcp.CallToolResult) string {
	for _, c := range res.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}

// --- run_query ---

func TestRunQuery_Select(t *testing.T) {
	h := newQueryHandler(testPool)
	res, err := h(context.Background(), toolReq(map[string]any{
		"sql": "SELECT id, name FROM users ORDER BY id",
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := mustText(t, res)

	var rows []map[string]any
	if err := json.Unmarshal([]byte(text), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 {
		t.Errorf("expected 4 rows, got %d", len(rows))
	}
	if rows[0]["name"] != "Alice" {
		t.Errorf("expected first row Alice, got %v", rows[0]["name"])
	}
}

func TestRunQuery_CTE(t *testing.T) {
	h := newQueryHandler(testPool)
	res, err := h(context.Background(), toolReq(map[string]any{
		"sql": "WITH u AS (SELECT name FROM users) SELECT * FROM u ORDER BY name",
	}))
	if err != nil {
		t.Fatal(err)
	}
	mustText(t, res)
}

func TestRunQuery_EmptyResult(t *testing.T) {
	h := newQueryHandler(testPool)
	res, err := h(context.Background(), toolReq(map[string]any{
		"sql": "SELECT id FROM users WHERE id = -1",
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := mustText(t, res)
	if text != "[]" {
		t.Errorf("expected empty JSON array, got: %s", text)
	}
}

func TestRunQuery_RejectsNonSelect(t *testing.T) {
	h := newQueryHandler(testPool)
	res, err := h(context.Background(), toolReq(map[string]any{
		"sql": "DELETE FROM users",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Error("expected error for non-SELECT query")
	}
}

func TestRunQuery_EmptySQL(t *testing.T) {
	h := newQueryHandler(testPool)
	res, err := h(context.Background(), toolReq(map[string]any{"sql": ""}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Error("expected error for empty sql")
	}
}

// --- inspect_schema ---

func TestInspectSchema_ListTables(t *testing.T) {
	h := newSchemaHandler(testPool)
	res, err := h(context.Background(), toolReq(map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	text := mustText(t, res)
	if !strings.Contains(text, "users") {
		t.Errorf("expected users in table list, got: %s", text)
	}
}

func TestInspectSchema_DescribeTable(t *testing.T) {
	h := newSchemaHandler(testPool)
	res, err := h(context.Background(), toolReq(map[string]any{
		"table": "users",
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := mustText(t, res)

	var detail map[string]any
	if err := json.Unmarshal([]byte(text), &detail); err != nil {
		t.Fatalf("unmarshal: %v\nraw: %s", err, text)
	}
	if detail["table"] != "users" {
		t.Errorf("expected table=users, got %v", detail["table"])
	}
	cols, ok := detail["columns"].([]any)
	if !ok || len(cols) == 0 {
		t.Error("expected non-empty columns")
	}
}

func TestInspectSchema_DescribeTable_SchemaQualified(t *testing.T) {
	h := newSchemaHandler(testPool)
	res, err := h(context.Background(), toolReq(map[string]any{
		"table": "public.users",
	}))
	if err != nil {
		t.Fatal(err)
	}
	mustText(t, res)
}

// --- explain_query ---

func TestExplainQuery_WithAnalyze(t *testing.T) {
	h := newExplainHandler(testPool)
	res, err := h(context.Background(), toolReq(map[string]any{
		"sql":     "SELECT * FROM users",
		"analyze": true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := mustText(t, res)
	if !strings.Contains(strings.ToLower(text), "users") {
		t.Errorf("expected plan to mention users table, got: %s", text)
	}
	if !strings.Contains(text, "actual time") {
		t.Errorf("expected 'actual time' in ANALYZE output, got: %s", text)
	}
}

func TestExplainQuery_WithoutAnalyze(t *testing.T) {
	h := newExplainHandler(testPool)
	res, err := h(context.Background(), toolReq(map[string]any{
		"sql":     "SELECT * FROM users",
		"analyze": false,
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := mustText(t, res)
	if strings.Contains(text, "actual time") {
		t.Error("expected no 'actual time' in non-analyze explain")
	}
}

func TestExplainQuery_DefaultsToAnalyze(t *testing.T) {
	h := newExplainHandler(testPool)
	res, err := h(context.Background(), toolReq(map[string]any{
		"sql": "SELECT * FROM users",
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := mustText(t, res)
	if !strings.Contains(text, "actual time") {
		t.Errorf("expected ANALYZE by default, got: %s", text)
	}
}

// --- check_compliance ---

func writeRulesFile(t *testing.T, rules any) string {
	t.Helper()
	data, err := json.Marshal(map[string]any{"rules": rules})
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.CreateTemp("", "compliance-*.json")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.Write(data)
	_ = f.Close()
	t.Cleanup(func() {
		_ = os.Remove(f.Name())
	})
	return f.Name()
}

func TestCheckCompliance_NotNull_Pass(t *testing.T) {
	rules := []any{
		map[string]any{"name": "name not null", "type": "not_null", "table": "users", "column": "name"},
	}
	h := newComplianceHandler(testPool, writeRulesFile(t, rules))
	res, err := h(context.Background(), toolReq(map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	text := mustText(t, res)
	var report complianceReport
	if err := json.Unmarshal([]byte(text), &report); err != nil {
		t.Fatal(err)
	}
	if report.Passed != 1 || report.Failed != 0 {
		t.Errorf("expected 1 pass/0 fail, got %d/%d", report.Passed, report.Failed)
	}
}

func TestCheckCompliance_NotNull_Fail(t *testing.T) {
	rules := []any{
		map[string]any{"name": "email not null", "type": "not_null", "table": "users", "column": "email"},
	}
	h := newComplianceHandler(testPool, writeRulesFile(t, rules))
	res, err := h(context.Background(), toolReq(map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	text := mustText(t, res)
	var report complianceReport
	if err := json.Unmarshal([]byte(text), &report); err != nil {
		t.Fatal(err)
	}
	if report.Failed != 1 {
		t.Errorf("expected 1 failure, got %d", report.Failed)
	}
	// Charlie has NULL email
	if report.Results[0].Violations != 1 {
		t.Errorf("expected 1 violation, got %d", report.Results[0].Violations)
	}
}

func TestCheckCompliance_Regex_Fail(t *testing.T) {
	rules := []any{
		map[string]any{
			"name":    "valid email format",
			"type":    "regex",
			"table":   "users",
			"column":  "email",
			"pattern": `^[^@\s]+@[^@\s]+\.[^@\s]+$`,
		},
	}
	h := newComplianceHandler(testPool, writeRulesFile(t, rules))
	res, err := h(context.Background(), toolReq(map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	text := mustText(t, res)
	var report complianceReport
	if err := json.Unmarshal([]byte(text), &report); err != nil {
		t.Fatal(err)
	}
	// Dave has "not-an-email"; Charlie's NULL is skipped by regex rule
	if report.Results[0].Violations != 1 {
		t.Errorf("expected 1 violation, got %d", report.Results[0].Violations)
	}
}

func TestCheckCompliance_Range_Fail(t *testing.T) {
	min := float64(0)
	max := float64(100)
	rules := []any{
		map[string]any{
			"name":   "score in range",
			"type":   "range",
			"table":  "users",
			"column": "score",
			"min":    min,
			"max":    max,
		},
	}
	h := newComplianceHandler(testPool, writeRulesFile(t, rules))
	res, err := h(context.Background(), toolReq(map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	text := mustText(t, res)
	var report complianceReport
	if err := json.Unmarshal([]byte(text), &report); err != nil {
		t.Fatal(err)
	}
	// Charlie (105 > 100) and Dave (-5 < 0)
	if report.Results[0].Violations != 2 {
		t.Errorf("expected 2 violations, got %d", report.Results[0].Violations)
	}
}

func TestCheckCompliance_SQL_Pass(t *testing.T) {
	rules := []any{
		map[string]any{
			"name": "all users have names",
			"type": "sql",
			"sql":  "SELECT id FROM users WHERE name IS NULL",
		},
	}
	h := newComplianceHandler(testPool, writeRulesFile(t, rules))
	res, err := h(context.Background(), toolReq(map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	text := mustText(t, res)
	var report complianceReport
	if err := json.Unmarshal([]byte(text), &report); err != nil {
		t.Fatal(err)
	}
	if report.Passed != 1 || report.Failed != 0 {
		t.Errorf("expected 1 pass/0 fail, got %d/%d", report.Passed, report.Failed)
	}
}

func TestCheckCompliance_SQL_Fail(t *testing.T) {
	rules := []any{
		map[string]any{
			"name": "no invalid emails",
			"type": "sql",
			"sql":  "SELECT id, email FROM users WHERE email IS NOT NULL AND email NOT LIKE '%@%.%'",
		},
	}
	h := newComplianceHandler(testPool, writeRulesFile(t, rules))
	res, err := h(context.Background(), toolReq(map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	text := mustText(t, res)
	var report complianceReport
	if err := json.Unmarshal([]byte(text), &report); err != nil {
		t.Fatal(err)
	}
	if report.Failed != 1 {
		t.Errorf("expected 1 failure, got %d", report.Failed)
	}
}

func TestCheckCompliance_FilterByTable(t *testing.T) {
	rules := []any{
		map[string]any{"name": "users-rule", "type": "not_null", "table": "users", "column": "name"},
		map[string]any{"name": "orders-rule", "type": "not_null", "table": "orders", "column": "id"},
	}
	h := newComplianceHandler(testPool, writeRulesFile(t, rules))
	res, err := h(context.Background(), toolReq(map[string]any{"table": "users"}))
	if err != nil {
		t.Fatal(err)
	}
	text := mustText(t, res)
	var report complianceReport
	if err := json.Unmarshal([]byte(text), &report); err != nil {
		t.Fatal(err)
	}
	if report.TotalRules != 1 {
		t.Errorf("expected 1 rule after filter, got %d", report.TotalRules)
	}
}

func TestCheckCompliance_SampleCappedAtFive(t *testing.T) {
	// Insert extra rows to exceed sample cap
	_, err := testPool.Exec(context.Background(), `
		INSERT INTO users (name, email, score) VALUES
			('E', NULL, 1), ('F', NULL, 2), ('G', NULL, 3),
			('H', NULL, 4), ('I', NULL, 5), ('J', NULL, 6)
	`)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(),
			"DELETE FROM users WHERE name IN ('E','F','G','H','I','J')")
	})

	rules := []any{
		map[string]any{"name": "email not null", "type": "not_null", "table": "users", "column": "email"},
	}
	h := newComplianceHandler(testPool, writeRulesFile(t, rules))
	res, err := h(context.Background(), toolReq(map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	text := mustText(t, res)
	var report complianceReport
	if err := json.Unmarshal([]byte(text), &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Results[0].Sample) > 5 {
		t.Errorf("expected sample capped at 5, got %d", len(report.Results[0].Sample))
	}
}

func TestCheckCompliance_NoRulesFile(t *testing.T) {
	h := newComplianceHandler(testPool, "")
	res, err := h(context.Background(), toolReq(map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Error("expected error when COMPLIANCE_RULES_FILE is not set")
	}
}
