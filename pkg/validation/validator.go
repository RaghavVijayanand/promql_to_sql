package validation

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
	
	"github.com/shinro/promql-transpiler/pkg/ast"
)

// Validator validates PromQL expressions and transpiler inputs
// Following Single Responsibility Principle
type Validator interface {
	Validate(input interface{}) error
}

// PromQLValidator validates PromQL expressions
type PromQLValidator struct {
	maxQueryLength    int
	maxLabelMatchers  int
	maxAggregations   int
	allowedFunctions  map[string]bool
}

// NewPromQLValidator creates a new PromQL validator
func NewPromQLValidator() *PromQLValidator {
	return &PromQLValidator{
		maxQueryLength:   10000,
		maxLabelMatchers: 100,
		maxAggregations:  10,
		allowedFunctions: make(map[string]bool),
	}
}

// Validate validates a PromQL expression string
func (v *PromQLValidator) Validate(input interface{}) error {
	query, ok := input.(string)
	if !ok {
		return fmt.Errorf("input must be a string")
	}
	
	// Check length
	if len(query) > v.maxQueryLength {
		return fmt.Errorf("query too long: %d characters (max: %d)", len(query), v.maxQueryLength)
	}
	
	// Check for empty query
	if strings.TrimSpace(query) == "" {
		return fmt.Errorf("query cannot be empty")
	}
	
	// Check for valid UTF-8
	if !utf8.ValidString(query) {
		return fmt.Errorf("query contains invalid UTF-8")
	}
	
	// Check for SQL injection patterns
	if err := v.checkSQLInjection(query); err != nil {
		return err
	}
	
	// Check for balanced parentheses
	if err := v.checkBalancedParentheses(query); err != nil {
		return err
	}
	
	return nil
}

// Pre-compiled regex patterns for SQL injection detection (avoids recompilation per call)
var sqlInjectionPatterns = func() []*regexp.Regexp {
	patterns := []string{
		`(?i);\s*DROP`,
		`(?i);\s*DELETE`,
		`(?i);\s*UPDATE`,
		`(?i);\s*INSERT`,
		`(?i);\s*CREATE`,
		`(?i);\s*ALTER`,
		`(?i);\s*TRUNCATE`,
		`(?i)UNION\s+SELECT`,
		`--\s*$`,
	}
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		compiled = append(compiled, regexp.MustCompile(p))
	}
	return compiled
}()

// checkSQLInjection checks for SQL injection patterns
func (v *PromQLValidator) checkSQLInjection(query string) error {
	for _, re := range sqlInjectionPatterns {
		if re.MatchString(query) {
			return fmt.Errorf("potentially dangerous pattern detected: %s", re.String())
		}
	}
	
	return nil
}

// checkBalancedParentheses checks if parentheses are balanced
func (v *PromQLValidator) checkBalancedParentheses(query string) error {
	stack := 0
	for _, ch := range query {
		if ch == '(' {
			stack++
		} else if ch == ')' {
			stack--
			if stack < 0 {
				return fmt.Errorf("unbalanced parentheses: too many closing parentheses")
			}
		}
	}
	
	if stack > 0 {
		return fmt.Errorf("unbalanced parentheses: %d unclosed opening parentheses", stack)
	}
	
	return nil
}

// ASTValidator validates AST nodes
type ASTValidator struct {
	maxDepth int
}

// NewASTValidator creates a new AST validator
func NewASTValidator() *ASTValidator {
	return &ASTValidator{
		maxDepth: 20,
	}
}

// Validate validates an AST expression
func (v *ASTValidator) Validate(input interface{}) error {
	expr, ok := input.(ast.Expr)
	if !ok {
		return fmt.Errorf("input must be an ast.Expr")
	}
	
	// Check depth
	depth := v.calculateDepth(expr)
	if depth > v.maxDepth {
		return fmt.Errorf("expression too deeply nested: %d (max: %d)", depth, v.maxDepth)
	}
	
	// Validate specific node types
	return v.validateNode(expr)
}

// calculateDepth calculates the depth of an AST
func (v *ASTValidator) calculateDepth(expr ast.Expr) int {
	if expr == nil {
		return 0
	}
	
	switch e := expr.(type) {
	case *ast.BinaryExpr:
		leftDepth := v.calculateDepth(e.Left)
		rightDepth := v.calculateDepth(e.Right)
		if leftDepth > rightDepth {
			return leftDepth + 1
		}
		return rightDepth + 1
		
	case *ast.UnaryExpr:
		return v.calculateDepth(e.Expr) + 1
		
	case *ast.AggregateExpr:
		return v.calculateDepth(e.Expr) + 1
		
	case *ast.Call:
		maxDepth := 0
		for _, arg := range e.Args {
			d := v.calculateDepth(arg)
			if d > maxDepth {
				maxDepth = d
			}
		}
		return maxDepth + 1
		
	default:
		return 1
	}
}

// validateNode validates a specific AST node and recurses into children
func (v *ASTValidator) validateNode(expr ast.Expr) error {
	switch e := expr.(type) {
	case *ast.VectorSelector:
		if e.MetricSelector.Name == "" && len(e.MetricSelector.Matchers) == 0 {
			return fmt.Errorf("vector selector must have either a name or label matchers")
		}
		
	case *ast.MatrixSelector:
		if e.Range == 0 {
			return fmt.Errorf("matrix selector must have a non-zero range")
		}
		if e.VectorSelector == nil {
			return fmt.Errorf("matrix selector must have a vector selector")
		}
		// Recurse into vector selector
		if err := v.validateNode(e.VectorSelector); err != nil {
			return err
		}
		
	case *ast.AggregateExpr:
		if e.Op == "" {
			return fmt.Errorf("aggregate expression must have an operator")
		}
		if e.Expr == nil {
			return fmt.Errorf("aggregate expression must have an expression")
		}
		// Recurse into inner expression
		if err := v.validateNode(e.Expr); err != nil {
			return err
		}
		if e.Param != nil {
			if err := v.validateNode(e.Param); err != nil {
				return err
			}
		}
		
	case *ast.Call:
		if e.Func == "" {
			return fmt.Errorf("call must have a function name")
		}
		// Recurse into arguments
		for _, arg := range e.Args {
			if err := v.validateNode(arg); err != nil {
				return err
			}
		}
		
	case *ast.BinaryExpr:
		if e.Left == nil || e.Right == nil {
			return fmt.Errorf("binary expression must have both left and right operands")
		}
		// Recurse into both sides
		if err := v.validateNode(e.Left); err != nil {
			return err
		}
		if err := v.validateNode(e.Right); err != nil {
			return err
		}
		
	case *ast.UnaryExpr:
		if e.Expr == nil {
			return fmt.Errorf("unary expression must have an operand")
		}
		if err := v.validateNode(e.Expr); err != nil {
			return err
		}
		
	case *ast.ParenExpr:
		if e.Expr == nil {
			return fmt.Errorf("parenthesized expression must have an inner expression")
		}
		if err := v.validateNode(e.Expr); err != nil {
			return err
		}
		
	case *ast.SubqueryExpr:
		if e.Expr == nil {
			return fmt.Errorf("subquery must have an inner expression")
		}
		if err := v.validateNode(e.Expr); err != nil {
			return err
		}
	}
	
	return nil
}

// InputSanitizer sanitizes user inputs
type InputSanitizer struct {
	maxLabelLength int
	maxValueLength int
}

// NewInputSanitizer creates a new input sanitizer
func NewInputSanitizer() *InputSanitizer {
	return &InputSanitizer{
		maxLabelLength: 256,
		maxValueLength: 1024,
	}
}

// Pre-compiled regex for label sanitization
var labelSanitizeRegex = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// SanitizeLabel sanitizes a label name or value
func (s *InputSanitizer) SanitizeLabel(label string) (string, error) {
	if len(label) > s.maxLabelLength {
		return "", fmt.Errorf("label too long: %d (max: %d)", len(label), s.maxLabelLength)
	}
	
	// Remove any non-alphanumeric characters except underscore and dash
	sanitized := labelSanitizeRegex.ReplaceAllString(label, "")
	
	if sanitized == "" {
		return "", fmt.Errorf("label contains only invalid characters")
	}
	
	return sanitized, nil
}

// SanitizeValue sanitizes a value
func (s *InputSanitizer) SanitizeValue(value string) (string, error) {
	if len(value) > s.maxValueLength {
		return "", fmt.Errorf("value too long: %d (max: %d)", len(value), s.maxValueLength)
	}
	
	// Escape single quotes
	sanitized := strings.ReplaceAll(value, "'", "''")
	
	// Remove null bytes
	sanitized = strings.ReplaceAll(sanitized, "\x00", "")
	
	return sanitized, nil
}

// ComplexityValidator validates query complexity
type ComplexityValidator struct {
	maxFunctions     int
	maxAggregations  int
	maxJoins         int
	maxSubqueries    int
}

// NewComplexityValidator creates a new complexity validator
func NewComplexityValidator() *ComplexityValidator {
	return &ComplexityValidator{
		maxFunctions:    50,
		maxAggregations: 10,
		maxJoins:        5,
		maxSubqueries:   3,
	}
}

// Validate validates query complexity
func (v *ComplexityValidator) Validate(input interface{}) error {
	expr, ok := input.(ast.Expr)
	if !ok {
		return fmt.Errorf("input must be an ast.Expr")
	}
	
	stats := v.calculateComplexity(expr)
	
	if stats.functions > v.maxFunctions {
		return fmt.Errorf("too many functions: %d (max: %d)", stats.functions, v.maxFunctions)
	}
	
	if stats.aggregations > v.maxAggregations {
		return fmt.Errorf("too many aggregations: %d (max: %d)", stats.aggregations, v.maxAggregations)
	}
	
	return nil
}

type complexityStats struct {
	functions    int
	aggregations int
	depth        int
}

func (v *ComplexityValidator) calculateComplexity(expr ast.Expr) complexityStats {
	stats := complexityStats{}
	
	switch e := expr.(type) {
	case *ast.Call:
		stats.functions++
		for _, arg := range e.Args {
			childStats := v.calculateComplexity(arg)
			stats.functions += childStats.functions
			stats.aggregations += childStats.aggregations
		}
		
	case *ast.AggregateExpr:
		stats.aggregations++
		childStats := v.calculateComplexity(e.Expr)
		stats.functions += childStats.functions
		stats.aggregations += childStats.aggregations
		
	case *ast.BinaryExpr:
		leftStats := v.calculateComplexity(e.Left)
		rightStats := v.calculateComplexity(e.Right)
		stats.functions = leftStats.functions + rightStats.functions
		stats.aggregations = leftStats.aggregations + rightStats.aggregations
		
	case *ast.UnaryExpr:
		childStats := v.calculateComplexity(e.Expr)
		stats.functions = childStats.functions
		stats.aggregations = childStats.aggregations
	}
	
	return stats
}

// ValidationChain chains multiple validators
type ValidationChain struct {
	validators []Validator
}

// NewValidationChain creates a new validation chain
func NewValidationChain(validators ...Validator) *ValidationChain {
	return &ValidationChain{
		validators: validators,
	}
}

// Validate runs all validators in the chain
func (c *ValidationChain) Validate(input interface{}) error {
	for i, validator := range c.validators {
		if err := validator.Validate(input); err != nil {
			return fmt.Errorf("validation error at step %d: %w", i+1, err)
		}
	}
	return nil
}

// Add adds a validator to the chain
func (c *ValidationChain) Add(validator Validator) *ValidationChain {
	c.validators = append(c.validators, validator)
	return c
}
