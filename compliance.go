package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mark3labs/mcp-go/mcp"
)

type ruleType string

const (
	ruleNotNull  ruleType = "not_null"
	ruleRegex    ruleType = "regex"
	ruleRange    ruleType = "range"
	ruleSQL      ruleType = "sql"
)

type complianceRule struct {
	Name        string   `json:"name"`
	Type        ruleType `json:"type"`
	Description string   `json:"description,omitempty"`
	// for not_null, regex, range
	Table  string `json:"table,omitempty"`
	Column string `json:"column,omitempty"`
	// for regex
	Pattern string `json:"pattern,omitempty"`
	// for range
	Min *float64 `json:"min,omitempty"`
	Max *float64 `json:"max,omitempty"`
	// for sql — query should return violation rows (0 rows = pass)
	SQL string `json:"sql,omitempty"`
}

type rulesFile struct {
	Rules []complianceRule `json:"rules"`
}

type ruleResult struct {
	Rule       string   `json:"rule"`
	Status     string   `json:"status"`
	Violations int      `json:"violations"`
	Sample     []any    `json:"sample,omitempty"`
	Error      string   `json:"error,omitempty"`
}

type complianceReport struct {
	TotalRules int          `json:"total_rules"`
	Passed     int          `json:"passed"`
	Failed     int          `json:"failed"`
	Results    []ruleResult `json:"results"`
}

func newComplianceHandler(pool *pgxpool.Pool, rulesFilePath string) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if rulesFilePath == "" {
			return mcp.NewToolResultError("COMPLIANCE_RULES_FILE environment variable is not set"), nil
		}

		rules, err := loadRules(rulesFilePath)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to load rules file: %v", err)), nil
		}

		filterTable := req.GetString("table", "")
		filterTable = strings.TrimSpace(filterTable)

		var filtered []complianceRule
		for _, r := range rules {
			if filterTable == "" || strings.EqualFold(r.Table, filterTable) {
				filtered = append(filtered, r)
			}
		}

		if len(filtered) == 0 {
			msg := fmt.Sprintf("no rules found (total rules loaded: %d)", len(rules))
			if filterTable != "" {
				msg = fmt.Sprintf("no rules found for table %q (total rules loaded: %d)", filterTable, len(rules))
			}
			return mcp.NewToolResultText(msg), nil
		}

		report := complianceReport{}
		for _, rule := range filtered {
			result := checkRule(ctx, pool, rule)
			report.Results = append(report.Results, result)
			report.TotalRules++
			if result.Status == "PASS" {
				report.Passed++
			} else {
				report.Failed++
			}
		}

		out, _ := json.MarshalIndent(report, "", "  ")
		return mcp.NewToolResultText(string(out)), nil
	}
}

func loadRules(path string) ([]complianceRule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rf rulesFile
	if err := json.Unmarshal(data, &rf); err != nil {
		return nil, err
	}
	return rf.Rules, nil
}

func checkRule(ctx context.Context, pool *pgxpool.Pool, rule complianceRule) ruleResult {
	result := ruleResult{Rule: rule.Name}

	q, err := buildRuleQuery(rule)
	if err != nil {
		result.Status = "ERROR"
		result.Error = err.Error()
		return result
	}

	rows, err := pool.Query(ctx, q)
	if err != nil {
		result.Status = "ERROR"
		result.Error = fmt.Sprintf("query error: %v", err)
		return result
	}
	defer rows.Close()

	fieldDescs := rows.FieldDescriptions()
	var violations []any
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			result.Status = "ERROR"
			result.Error = fmt.Sprintf("scan error: %v", err)
			return result
		}
		row := make(map[string]any, len(fieldDescs))
		for i, fd := range fieldDescs {
			row[fd.Name] = values[i]
		}
		violations = append(violations, row)
	}
	if err := rows.Err(); err != nil {
		result.Status = "ERROR"
		result.Error = err.Error()
		return result
	}

	result.Violations = len(violations)
	if result.Violations == 0 {
		result.Status = "PASS"
	} else {
		result.Status = "FAIL"
		// Return up to 5 sample violations
		if len(violations) > 5 {
			result.Sample = violations[:5]
		} else {
			result.Sample = violations
		}
	}
	return result
}

func buildRuleQuery(rule complianceRule) (string, error) {
	switch rule.Type {
	case ruleNotNull:
		if rule.Table == "" || rule.Column == "" {
			return "", fmt.Errorf("not_null rule requires table and column")
		}
		return fmt.Sprintf(
			`SELECT %s FROM %s WHERE %s IS NULL`,
			pgQuoteIdent(rule.Column), pgQuoteIdent(rule.Table), pgQuoteIdent(rule.Column),
		), nil

	case ruleRegex:
		if rule.Table == "" || rule.Column == "" || rule.Pattern == "" {
			return "", fmt.Errorf("regex rule requires table, column, and pattern")
		}
		return fmt.Sprintf(
			`SELECT %s FROM %s WHERE %s IS NOT NULL AND %s !~ %s`,
			pgQuoteIdent(rule.Column), pgQuoteIdent(rule.Table),
			pgQuoteIdent(rule.Column), pgQuoteIdent(rule.Column),
			pgQuoteLiteral(rule.Pattern),
		), nil

	case ruleRange:
		if rule.Table == "" || rule.Column == "" {
			return "", fmt.Errorf("range rule requires table and column")
		}
		var conditions []string
		if rule.Min != nil {
			conditions = append(conditions, fmt.Sprintf("%s < %g", pgQuoteIdent(rule.Column), *rule.Min))
		}
		if rule.Max != nil {
			conditions = append(conditions, fmt.Sprintf("%s > %g", pgQuoteIdent(rule.Column), *rule.Max))
		}
		if len(conditions) == 0 {
			return "", fmt.Errorf("range rule requires at least one of min or max")
		}
		return fmt.Sprintf(
			`SELECT %s FROM %s WHERE %s IS NOT NULL AND (%s)`,
			pgQuoteIdent(rule.Column), pgQuoteIdent(rule.Table),
			pgQuoteIdent(rule.Column), strings.Join(conditions, " OR "),
		), nil

	case ruleSQL:
		if strings.TrimSpace(rule.SQL) == "" {
			return "", fmt.Errorf("sql rule requires sql field")
		}
		return rule.SQL, nil

	default:
		return "", fmt.Errorf("unknown rule type: %q", rule.Type)
	}
}

// pgQuoteIdent double-quotes an identifier, escaping any embedded double quotes.
func pgQuoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// pgQuoteLiteral single-quotes a string literal, escaping single quotes.
func pgQuoteLiteral(s string) string {
	return `'` + strings.ReplaceAll(s, `'`, `''`) + `'`
}
