package security

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// SecurityValidator validates queries for security issues
type SecurityValidator struct {
	maxQueryLength    int
	maxComplexity     int
	allowedTables     []string
	deniedPatterns    []string
	requireValidation bool
}

// NewSecurityValidator creates a new security validator
func NewSecurityValidator() *SecurityValidator {
	return &SecurityValidator{
		maxQueryLength:    10000,
		maxComplexity:     1000,
		allowedTables:     []string{"metrics", "metrics_cardinality", "alerts"},
		deniedPatterns:    []string{"DROP", "DELETE", "TRUNCATE", "ALTER TABLE", "CREATE TABLE", "INSERT", "UPDATE", "ATTACH", "DETACH", "RENAME TABLE", "OPTIMIZE TABLE"},
		requireValidation: true,
	}
}

// WithMaxQueryLength sets maximum query length
func (v *SecurityValidator) WithMaxQueryLength(length int) *SecurityValidator {
	v.maxQueryLength = length
	return v
}

// WithMaxComplexity sets maximum query complexity
func (v *SecurityValidator) WithMaxComplexity(complexity int) *SecurityValidator {
	v.maxComplexity = complexity
	return v
}

// WithAllowedTables sets allowed tables
func (v *SecurityValidator) WithAllowedTables(tables []string) *SecurityValidator {
	v.allowedTables = tables
	return v
}

// WithDeniedPatterns sets denied SQL patterns
func (v *SecurityValidator) WithDeniedPatterns(patterns []string) *SecurityValidator {
	v.deniedPatterns = patterns
	return v
}

// Validate validates query for security issues
func (v *SecurityValidator) Validate(query string) error {
	// Check query length
	if len(query) > v.maxQueryLength {
		return fmt.Errorf("query exceeds maximum length of %d characters", v.maxQueryLength)
	}
	
	// Check for SQL injection patterns
	if err := v.checkSQLInjection(query); err != nil {
		return err
	}
	
	// Check for denied patterns
	if err := v.checkDeniedPatterns(query); err != nil {
		return err
	}
	
	// Check complexity
	if err := v.checkComplexity(query); err != nil {
		return err
	}
	
	return nil
}

// Pre-compiled SQL injection patterns (avoids recompilation per call)
var securityInjectionPatterns = func() []*regexp.Regexp {
	patterns := []string{
		`(?i)union\s+select`,
		`(?i);\s*drop\s+table`,
		`(?i);\s*delete\s+from`,
		`(?i)1\s*=\s*1`,
		`(?i)or\s+1\s*=\s*1`,
		`(?i)'\s+or\s+'1'\s*=\s*'1`,
		`--`,
		`/\*.*\*/`,
	}
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		compiled = append(compiled, regexp.MustCompile(p))
	}
	return compiled
}()

// checkSQLInjection checks for SQL injection patterns
func (v *SecurityValidator) checkSQLInjection(query string) error {
	for _, re := range securityInjectionPatterns {
		if re.MatchString(query) {
			return errors.New("potential SQL injection detected")
		}
	}
	
	return nil
}

// checkDeniedPatterns checks for denied SQL patterns
func (v *SecurityValidator) checkDeniedPatterns(query string) error {
	upperQuery := strings.ToUpper(query)
	
	for _, pattern := range v.deniedPatterns {
		if strings.Contains(upperQuery, pattern) {
			return fmt.Errorf("denied SQL pattern detected: %s", pattern)
		}
	}
	
	return nil
}

// checkComplexity checks query complexity
func (v *SecurityValidator) checkComplexity(query string) error {
	complexity := 0
	
	// Count SELECT statements
	complexity += strings.Count(strings.ToUpper(query), "SELECT") * 10
	
	// Count JOINs
	complexity += strings.Count(strings.ToUpper(query), "JOIN") * 20
	
	// Count subqueries
	complexity += strings.Count(query, "(") * 5
	
	// Count aggregations
	complexity += strings.Count(strings.ToUpper(query), "GROUP BY") * 15
	
	if complexity > v.maxComplexity {
		return fmt.Errorf("query complexity (%d) exceeds maximum (%d)", complexity, v.maxComplexity)
	}
	
	return nil
}

// SanitizeInput sanitizes user input for ClickHouse queries.
// Removes characters that could be used for SQL injection.
func SanitizeInput(input string) string {
	// Only remove patterns dangerous for ClickHouse.
	// Note: "--" is a valid SQL comment and is NOT removed;
	//       "xp_"/"sp_" are MS SQL specific and irrelevant.
	dangerous := []string{";", "/*", "*/"}

	result := input
	for _, char := range dangerous {
		result = strings.ReplaceAll(result, char, "")
	}

	return result
}

// ValidateTableName validates table name
func ValidateTableName(tableName string, allowedTables []string) error {
	// Check if table is in allowed list
	for _, allowed := range allowedTables {
		if tableName == allowed {
			return nil
		}
	}
	
	return fmt.Errorf("table %s is not in allowed list", tableName)
}

// ResourceQuota represents resource usage limits
type ResourceQuota struct {
	MaxMemoryBytes   int64
	MaxCPUSeconds    int64
	MaxRows          int64
	MaxBytesRead     int64
	MaxExecutionTime int64
}

// NewDefaultResourceQuota creates default resource quota
func NewDefaultResourceQuota() *ResourceQuota {
	return &ResourceQuota{
		MaxMemoryBytes:   1024 * 1024 * 1024, // 1GB
		MaxCPUSeconds:    60,
		MaxRows:          1000000,
		MaxBytesRead:     10 * 1024 * 1024 * 1024, // 10GB
		MaxExecutionTime: 300,                     // 5 minutes
	}
}

// ApplyToQuery applies ClickHouse resource quota SETTINGS to the query.
// Safely merges with any existing SETTINGS clause.
func (q *ResourceQuota) ApplyToQuery(query string) string {
	settings := []string{
		fmt.Sprintf("max_memory_usage = %d", q.MaxMemoryBytes),
		fmt.Sprintf("max_execution_time = %d", q.MaxExecutionTime),
		fmt.Sprintf("max_rows_to_read = %d", q.MaxRows),
		fmt.Sprintf("max_bytes_to_read = %d", q.MaxBytesRead),
	}

	settingsStr := strings.Join(settings, ", ")

	settingsIdx := strings.Index(query, "SETTINGS ")
	if settingsIdx != -1 {
		// Insert after existing "SETTINGS " prefix
		insertPos := settingsIdx + 9 // len("SETTINGS ")
		query = query[:insertPos] + settingsStr + ", " + query[insertPos:]
	} else {
		query = strings.TrimRight(query, " \n\t") + "\nSETTINGS " + settingsStr
	}

	return query
}

// ComplexityLimiter limits query complexity
type ComplexityLimiter struct {
	maxDepth       int
	maxJoins       int
	maxSubqueries  int
	maxAggregations int
}

// NewComplexityLimiter creates a new complexity limiter
func NewComplexityLimiter() *ComplexityLimiter {
	return &ComplexityLimiter{
		maxDepth:       5,
		maxJoins:       10,
		maxSubqueries:  5,
		maxAggregations: 20,
	}
}

// CheckComplexity checks if query exceeds complexity limits
func (l *ComplexityLimiter) CheckComplexity(query string) error {
	upperQuery := strings.ToUpper(query)
	
	// Check subquery depth
	depth := strings.Count(query, "(")
	if depth > l.maxDepth {
		return fmt.Errorf("query depth (%d) exceeds maximum (%d)", depth, l.maxDepth)
	}
	
	// Check number of JOINs
	joins := strings.Count(upperQuery, "JOIN")
	if joins > l.maxJoins {
		return fmt.Errorf("number of JOINs (%d) exceeds maximum (%d)", joins, l.maxJoins)
	}
	
	// Check number of subqueries
	subqueries := strings.Count(upperQuery, "SELECT") - 1
	if subqueries > l.maxSubqueries {
		return fmt.Errorf("number of subqueries (%d) exceeds maximum (%d)", subqueries, l.maxSubqueries)
	}
	
	return nil
}
