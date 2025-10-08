package security

import (
	"fmt"
	"strings"

	pg_query "github.com/pganalyze/pg_query_go/v5"
)

// SQLOperation represents the type of SQL operation
type SQLOperation string

const (
	OpSelect   SQLOperation = "SELECT"
	OpInsert   SQLOperation = "INSERT"
	OpUpdate   SQLOperation = "UPDATE"
	OpDelete   SQLOperation = "DELETE"
	OpTruncate SQLOperation = "TRUNCATE"
	OpDrop     SQLOperation = "DROP"
	OpAlter    SQLOperation = "ALTER"
	OpCreate   SQLOperation = "CREATE"
	OpGrant    SQLOperation = "GRANT"
	OpRevoke   SQLOperation = "REVOKE"
	OpUnknown  SQLOperation = "UNKNOWN"
)

// QueryAnalysis contains the results of analyzing a SQL query
type QueryAnalysis struct {
	Valid      bool           // Whether the query parsed successfully
	Operations []SQLOperation // List of operations (e.g., SELECT, INSERT)
	Tables     []string       // List of tables accessed
	Columns    []string       // List of columns accessed (if determinable)
	HasJoin    bool           // Whether query contains JOIN
	Error      error          // Parse error if any
}

// SQLAnalyzer provides SQL parsing and semantic analysis
type SQLAnalyzer struct{}

// NewSQLAnalyzer creates a new SQL analyzer
func NewSQLAnalyzer() *SQLAnalyzer {
	return &SQLAnalyzer{}
}

// AnalyzeQuery parses a SQL query and extracts semantic information
func (a *SQLAnalyzer) AnalyzeQuery(sql string) *QueryAnalysis {
	analysis := &QueryAnalysis{
		Operations: []SQLOperation{},
		Tables:     []string{},
		Columns:    []string{},
		HasJoin:    false,
		Valid:      false,
	}

	// Parse the SQL query
	result, err := pg_query.Parse(sql)
	if err != nil {
		analysis.Error = fmt.Errorf("SQL parse error: %w", err)
		return analysis
	}

	analysis.Valid = true

	// Extract information from each statement
	for _, stmt := range result.Stmts {
		// Determine operation type
		op := a.detectOperation(stmt)
		if op != OpUnknown {
			analysis.Operations = append(analysis.Operations, op)
		}

		// Extract table names
		tables := a.extractTables(stmt)
		analysis.Tables = append(analysis.Tables, tables...)
	}

	// Deduplicate tables
	analysis.Tables = uniqueStrings(analysis.Tables)

	return analysis
}

// detectOperation determines the SQL operation type from a statement
func (a *SQLAnalyzer) detectOperation(stmt *pg_query.RawStmt) SQLOperation {
	if stmt.Stmt == nil {
		return OpUnknown
	}

	switch node := stmt.Stmt.Node.(type) {
	case *pg_query.Node_SelectStmt:
		return OpSelect
	case *pg_query.Node_InsertStmt:
		return OpInsert
	case *pg_query.Node_UpdateStmt:
		return OpUpdate
	case *pg_query.Node_DeleteStmt:
		return OpDelete
	case *pg_query.Node_TruncateStmt:
		return OpTruncate
	case *pg_query.Node_DropStmt:
		return OpDrop
	case *pg_query.Node_AlterTableStmt:
		return OpAlter
	case *pg_query.Node_CreateStmt:
		return OpCreate
	case *pg_query.Node_GrantStmt:
		return OpGrant
	case *pg_query.Node_GrantRoleStmt:
		return OpGrant
	default:
		_ = node // Avoid unused variable warning
		return OpUnknown
	}
}

// extractTables extracts table names from a statement
func (a *SQLAnalyzer) extractTables(stmt *pg_query.RawStmt) []string {
	tables := []string{}

	if stmt.Stmt == nil {
		return tables
	}

	switch node := stmt.Stmt.Node.(type) {
	case *pg_query.Node_SelectStmt:
		if selectStmt := node.SelectStmt; selectStmt != nil {
			tables = append(tables, a.extractFromClause(selectStmt.FromClause)...)
		}

	case *pg_query.Node_InsertStmt:
		if insertStmt := node.InsertStmt; insertStmt != nil && insertStmt.Relation != nil {
			if rel := insertStmt.Relation; rel.Relname != "" {
				tables = append(tables, rel.Relname)
			}
		}

	case *pg_query.Node_UpdateStmt:
		if updateStmt := node.UpdateStmt; updateStmt != nil && updateStmt.Relation != nil {
			if rel := updateStmt.Relation.Relname; rel != "" {
				tables = append(tables, rel)
			}
		}

	case *pg_query.Node_DeleteStmt:
		if deleteStmt := node.DeleteStmt; deleteStmt != nil && deleteStmt.Relation != nil {
			if rel := deleteStmt.Relation.Relname; rel != "" {
				tables = append(tables, rel)
			}
		}

	case *pg_query.Node_TruncateStmt:
		if truncateStmt := node.TruncateStmt; truncateStmt != nil {
			for _, rel := range truncateStmt.Relations {
				if rangeVar := rel.GetRangeVar(); rangeVar != nil && rangeVar.Relname != "" {
					tables = append(tables, rangeVar.Relname)
				}
			}
		}

	case *pg_query.Node_DropStmt:
		if dropStmt := node.DropStmt; dropStmt != nil {
			for _, obj := range dropStmt.Objects {
				if list := obj.GetList(); list != nil {
					for _, item := range list.Items {
						if str := item.GetString_(); str != nil && str.Sval != "" {
							tables = append(tables, str.Sval)
						}
					}
				}
			}
		}
	}

	return tables
}

// extractFromClause extracts table names from FROM clause
func (a *SQLAnalyzer) extractFromClause(fromClause []*pg_query.Node) []string {
	tables := []string{}

	for _, fromItem := range fromClause {
		if fromItem == nil {
			continue
		}

		switch node := fromItem.Node.(type) {
		case *pg_query.Node_RangeVar:
			if rangeVar := node.RangeVar; rangeVar != nil && rangeVar.Relname != "" {
				tables = append(tables, rangeVar.Relname)
			}

		case *pg_query.Node_JoinExpr:
			// Handle JOIN clauses recursively
			if joinExpr := node.JoinExpr; joinExpr != nil {
				if joinExpr.Larg != nil {
					tables = append(tables, a.extractFromClause([]*pg_query.Node{joinExpr.Larg})...)
				}
				if joinExpr.Rarg != nil {
					tables = append(tables, a.extractFromClause([]*pg_query.Node{joinExpr.Rarg})...)
				}
			}

		case *pg_query.Node_RangeSubselect:
			// Handle subqueries in FROM clause
			if rangeSubselect := node.RangeSubselect; rangeSubselect != nil && rangeSubselect.Subquery != nil {
				if selectStmt := rangeSubselect.Subquery.GetSelectStmt(); selectStmt != nil {
					tables = append(tables, a.extractFromClause(selectStmt.FromClause)...)
				}
			}
		}
	}

	return tables
}

// uniqueStrings removes duplicates from a string slice
func uniqueStrings(input []string) []string {
	seen := make(map[string]bool)
	result := []string{}

	for _, item := range input {
		if item != "" && !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}

	return result
}

// TablePermission defines access control for database tables
type TablePermission struct {
	Operations []SQLOperation // Allowed operations (SELECT, INSERT, etc.)
	Tables     []string       // Table names or patterns (*, users, logs_*)
	Columns    []string       // Column restrictions (optional, ["*"] = all)
}

// CheckTablePermissions verifies if a query is allowed based on table-level permissions
func (a *SQLAnalyzer) CheckTablePermissions(analysis *QueryAnalysis, permissions []TablePermission) (bool, string) {
	if !analysis.Valid {
		return false, "invalid SQL query"
	}

	// Check each operation in the query
	for _, operation := range analysis.Operations {
		allowed := false

		// Check if operation is allowed on all accessed tables
		for _, table := range analysis.Tables {
			tableAllowed := false

			// Check against each permission rule
			for _, perm := range permissions {
				// Check if operation is in allowed operations
				opAllowed := false
				for _, allowedOp := range perm.Operations {
					if allowedOp == operation {
						opAllowed = true
						break
					}
				}

				if !opAllowed {
					continue
				}

				// Check if table matches any allowed pattern
				for _, allowedTable := range perm.Tables {
					if matchTablePattern(table, allowedTable) {
						tableAllowed = true
						break
					}
				}

				if tableAllowed {
					break
				}
			}

			if !tableAllowed {
				return false, fmt.Sprintf("operation %s not allowed on table '%s'", operation, table)
			}

			allowed = true
		}

		if !allowed && len(analysis.Tables) == 0 {
			// Query has no tables (e.g., "SELECT 1") - check if operation is allowed at all
			opAllowed := false
			for _, perm := range permissions {
				for _, allowedOp := range perm.Operations {
					if allowedOp == operation {
						opAllowed = true
						break
					}
				}
				if opAllowed {
					break
				}
			}
			if !opAllowed {
				return false, fmt.Sprintf("operation %s not allowed", operation)
			}
		}
	}

	return true, ""
}

// matchTablePattern checks if a table name matches a pattern
// Supports wildcards: * (match all), prefix_* (prefix match), *_suffix (suffix match)
func matchTablePattern(tableName, pattern string) bool {
	// Exact match
	if pattern == tableName {
		return true
	}

	// Wildcard: match all
	if pattern == "*" {
		return true
	}

	// Prefix wildcard: logs_*
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(tableName, prefix)
	}

	// Suffix wildcard: *_logs
	if strings.HasPrefix(pattern, "*") {
		suffix := strings.TrimPrefix(pattern, "*")
		return strings.HasSuffix(tableName, suffix)
	}

	return false
}
