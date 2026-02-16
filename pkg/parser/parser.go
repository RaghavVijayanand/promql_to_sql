package parser

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/shinro/promql-transpiler/pkg/ast"
	"github.com/shinro/promql-transpiler/pkg/lexer"
)

// Parser parses PromQL expressions into AST
type Parser struct {
	l      *lexer.Lexer
	errors []string
	depth  int // Recursion depth counter

	curToken  lexer.Token
	peekToken lexer.Token
}

const (
	maxParseDepth = 100  // Maximum recursion depth to prevent stack overflow
	maxErrors     = 100  // Maximum errors to collect before truncating
)

// New creates a new Parser
func New(l *lexer.Lexer) *Parser {
	p := &Parser{
		l:      l,
		errors: make([]string, 0, 4), // Pre-allocate for common case
	}
	// Read two tokens to initialize curToken and peekToken
	p.nextToken()
	p.nextToken()
	return p
}

// Errors returns parsing errors
func (p *Parser) Errors() []string {
	return p.errors
}

func (p *Parser) nextToken() {
	p.curToken = p.peekToken
	p.peekToken = p.l.NextToken()
}

func (p *Parser) curTokenIs(t lexer.TokenType) bool {
	return p.curToken.Type == t
}

func (p *Parser) peekTokenIs(t lexer.TokenType) bool {
	return p.peekToken.Type == t
}

func (p *Parser) addError(msg string) {
	if len(p.errors) >= maxErrors {
		if len(p.errors) == maxErrors {
			p.errors = append(p.errors, "... (additional errors truncated)")
		}
		return
	}
	p.errors = append(p.errors, fmt.Sprintf("%s at line %d, col %d",
		msg, p.curToken.Line, p.curToken.Column))
}

// pushDepth increments recursion depth and checks limit
func (p *Parser) pushDepth() error {
	p.depth++
	if p.depth > maxParseDepth {
		return fmt.Errorf("maximum parse depth exceeded (%d) - query too deeply nested", maxParseDepth)
	}
	return nil
}

// popDepth decrements recursion depth
func (p *Parser) popDepth() {
	p.depth--
}

// ParseExpr parses a PromQL expression
func (p *Parser) ParseExpr() (ast.Expr, error) {
	expr := p.parseOrExpr()
	if len(p.errors) > 0 {
		return nil, fmt.Errorf("parsing errors: %s", strings.Join(p.errors, "; "))
	}
	if expr == nil {
		return nil, fmt.Errorf("failed to parse expression")
	}
	return expr, nil
}

// ─── EXPRESSION PRECEDENCE (lowest to highest) ───────────────────────────────
// or, unless
// and
// ==, !=, <, >, <=, >=
// +, -
// *, /, %
// ^
// unary +, -
// primary (number, string, paren, agg, func, selector)
// ──────────────────────────────────────────────────────────────────────────────

// parseOrExpr: expr (or|unless) expr
func (p *Parser) parseOrExpr() ast.Expr {
	left := p.parseAndExpr()
	if left == nil {
		return nil
	}

	for p.curTokenIs(lexer.OR) || p.curTokenIs(lexer.UNLESS) {
		var binOp ast.BinOp
		if p.curTokenIs(lexer.OR) {
			binOp = ast.OpOr
		} else {
			binOp = ast.OpUnless
		}
		p.nextToken() // past or/unless

		matching := p.parseVectorMatching()
		right := p.parseAndExpr()

		left = &ast.BinaryExpr{
			Left:     left,
			Operator: binOp,
			Right:    right,
			Matching: matching,
		}
	}
	return left
}

// parseAndExpr: expr and expr
func (p *Parser) parseAndExpr() ast.Expr {
	left := p.parseComparisonExpr()
	if left == nil {
		return nil
	}

	for p.curTokenIs(lexer.AND) {
		p.nextToken() // past and

		matching := p.parseVectorMatching()
		right := p.parseComparisonExpr()

		left = &ast.BinaryExpr{
			Left:     left,
			Operator: ast.OpAnd,
			Right:    right,
			Matching: matching,
		}
	}
	return left
}

// parseComparisonExpr: expr (==|!=|<|>|<=|>=) [bool] expr
func (p *Parser) parseComparisonExpr() ast.Expr {
	left := p.parseAddExpr()
	if left == nil {
		return nil
	}

	if p.isComparisonOp() {
		op := p.curToken.Type
		p.nextToken() // past comparison op

		returnBool := false
		if p.curTokenIs(lexer.BOOL) {
			returnBool = true
			p.nextToken()
		}

		matching := p.parseVectorMatching()
		right := p.parseAddExpr()

		left = &ast.BinaryExpr{
			Left:       left,
			Operator:   tokenToBinOp(op),
			Right:      right,
			Matching:   matching,
			ReturnBool: returnBool,
		}
	}
	return left
}

// parseAddExpr: expr (+|-) expr
func (p *Parser) parseAddExpr() ast.Expr {
	left := p.parseMulExpr()
	if left == nil {
		return nil
	}

	for p.curTokenIs(lexer.ADD) || p.curTokenIs(lexer.SUB) {
		op := p.curToken.Type
		p.nextToken() // past +/-

		matching := p.parseVectorMatching()
		right := p.parseMulExpr()

		left = &ast.BinaryExpr{
			Left:     left,
			Operator: tokenToBinOp(op),
			Right:    right,
			Matching: matching,
		}
	}
	return left
}

// parseMulExpr: expr (*|/|%) expr
func (p *Parser) parseMulExpr() ast.Expr {
	left := p.parsePowExpr()
	if left == nil {
		return nil
	}

	for p.curTokenIs(lexer.MUL) || p.curTokenIs(lexer.DIV) || p.curTokenIs(lexer.MOD) {
		op := p.curToken.Type
		p.nextToken() // past operator

		matching := p.parseVectorMatching()
		right := p.parsePowExpr()

		left = &ast.BinaryExpr{
			Left:     left,
			Operator: tokenToBinOp(op),
			Right:    right,
			Matching: matching,
		}
	}
	return left
}

// parsePowExpr: expr ^ expr (right-associative)
func (p *Parser) parsePowExpr() ast.Expr {
	left := p.parseUnaryExpr()
	if left == nil {
		return nil
	}

	if p.curTokenIs(lexer.POW) {
		p.nextToken() // past ^

		matching := p.parseVectorMatching()
		right := p.parsePowExpr() // right-associative via recursion

		left = &ast.BinaryExpr{
			Left:     left,
			Operator: ast.OpPow,
			Right:    right,
			Matching: matching,
		}
	}
	return left
}

// parseUnaryExpr: (+|-) expr
func (p *Parser) parseUnaryExpr() ast.Expr {
	if err := p.pushDepth(); err != nil {
		p.addError(err.Error())
		return nil
	}
	defer p.popDepth()
	
	if p.curTokenIs(lexer.ADD) || p.curTokenIs(lexer.SUB) {
		var unaryOp ast.UnaryOp
		if p.curTokenIs(lexer.ADD) {
			unaryOp = ast.OpUnaryPlus
		} else {
			unaryOp = ast.OpUnaryMinus
		}
		p.nextToken() // past +/-
		expr := p.parseUnaryExpr()
		return &ast.UnaryExpr{Operator: unaryOp, Expr: expr}
	}
	return p.parsePostfixExpr()
}

// parsePostfixExpr handles postfix modifiers on any expression:
// subquery [range:step] and offset.
func (p *Parser) parsePostfixExpr() ast.Expr {
	expr := p.parsePrimaryExpr()
	if expr == nil {
		return nil
	}

	// Handle postfix [range:step] for subqueries.
	// Note: plain [duration] for matrix selectors is already consumed inside
	// parseVectorOrMatrixSelector, so if we see '[' here, the expression
	// is a function call, aggregation, or paren expr that needs a subquery.
	for p.curTokenIs(lexer.LBRACKET) {
		expr = p.parseSubqueryPostfix(expr)
		if expr == nil {
			return nil
		}
	}

	return expr
}

// parseSubqueryPostfix parses [range:step] (and optional offset) after an
// arbitrary expression, producing a *ast.SubqueryExpr.
func (p *Parser) parseSubqueryPostfix(expr ast.Expr) ast.Expr {
	p.nextToken() // skip '['

	if !p.curTokenIs(lexer.DURATION) && !p.curTokenIs(lexer.NUMBER) {
		p.addError("expected duration in subquery range")
		return nil
	}

	dur, err := parseDuration(p.curToken.Literal)
	if err != nil {
		p.addError(fmt.Sprintf("invalid subquery range: %v", err))
		return nil
	}
	p.nextToken() // past duration

	// A colon is required for subquery syntax
	if !p.curTokenIs(lexer.COLON) {
		p.addError("expected ':' in subquery")
		return nil
	}
	p.nextToken() // skip ':'

	// Optional step duration
	var step time.Duration
	if p.curTokenIs(lexer.DURATION) || p.curTokenIs(lexer.NUMBER) {
		step, err = parseDuration(p.curToken.Literal)
		if err != nil {
			p.addError(fmt.Sprintf("invalid subquery step: %v", err))
			return nil
		}
		p.nextToken()
	}

	if !p.curTokenIs(lexer.RBRACKET) {
		p.addError("expected ']' after subquery")
		return nil
	}
	p.nextToken() // skip ']'

	subquery := &ast.SubqueryExpr{
		Expr:  expr,
		Range: dur,
		Step:  step,
	}

	// Handle optional @ and offset modifiers in any order after subquery
	for p.curTokenIs(lexer.OFFSET) || p.curTokenIs(lexer.AT) {
		if p.curTokenIs(lexer.OFFSET) {
			p.nextToken()
			if !p.curTokenIs(lexer.DURATION) {
				p.addError("expected duration after offset")
				return nil
			}
			offset, err := parseDuration(p.curToken.Literal)
			if err != nil {
				p.addError(fmt.Sprintf("invalid offset: %v", err))
				return nil
			}
			subquery.Offset = offset
			p.nextToken()
		} else if p.curTokenIs(lexer.AT) {
			p.nextToken() // skip '@'
			if !p.curTokenIs(lexer.NUMBER) {
				p.addError("expected timestamp after @")
				return nil
			}
			// Store @ timestamp on the subquery (currently not used, but parse it)
			p.nextToken()
		}
	}

	return subquery
}

// parsePrimaryExpr dispatches to the correct primary parser
func (p *Parser) parsePrimaryExpr() ast.Expr {
	if err := p.pushDepth(); err != nil {
		p.addError(err.Error())
		return nil
	}
	defer p.popDepth()
	
	switch {
	case p.curTokenIs(lexer.NUMBER):
		return p.parseNumberLiteral()

	case p.curTokenIs(lexer.STRING):
		return p.parseStringLiteral()

	case p.curTokenIs(lexer.LPAREN):
		return p.parseParenExpr()

	case p.isAggregateOp():
		return p.parseAggregateExpr()

	case p.isFunctionKeyword():
		// Function recognised by a dedicated lexer keyword token
		funcType := tokenToFuncType(p.curToken.Type)
		return p.parseFunctionCallWithType(funcType)

	case p.curTokenIs(lexer.IDENT):
		// Check if this identifier is a known PromQL function followed by '('
		if p.peekTokenIs(lexer.LPAREN) {
			if ft, ok := ast.KnownFunctions[strings.ToLower(p.curToken.Literal)]; ok {
				return p.parseFunctionCallWithType(ft)
			}
		}
		return p.parseVectorOrMatrixSelector()

	case p.curTokenIs(lexer.LBRACE):
		// Label matchers without metric name, e.g. {job="api"}
		return p.parseVectorOrMatrixSelector()

	default:
		p.addError(fmt.Sprintf("unexpected token %q (%v)", p.curToken.Literal, p.curToken.Type))
		p.nextToken()
		return nil
	}
}

// ─── LITERALS ────────────────────────────────────────────────────────────────

func (p *Parser) parseNumberLiteral() ast.Expr {
	value, err := strconv.ParseFloat(p.curToken.Literal, 64)
	if err != nil {
		p.addError(fmt.Sprintf("could not parse %q as float", p.curToken.Literal))
		return nil
	}
	expr := &ast.NumberLiteral{Value: value}
	p.nextToken() // advance past number
	return expr
}

func (p *Parser) parseStringLiteral() ast.Expr {
	expr := &ast.StringLiteral{Value: p.curToken.Literal}
	p.nextToken() // advance past string
	return expr
}

// ─── PARENTHESIZED EXPRESSION ────────────────────────────────────────────────

func (p *Parser) parseParenExpr() ast.Expr {
	p.nextToken() // skip '('

	expr := p.parseOrExpr()
	if expr == nil {
		return nil
	}

	if !p.curTokenIs(lexer.RPAREN) {
		p.addError("expected ')' after expression")
		return nil
	}
	p.nextToken() // skip ')'

	return &ast.ParenExpr{Expr: expr}
}

// ─── VECTOR / MATRIX SELECTORS ───────────────────────────────────────────────

func (p *Parser) parseVectorOrMatrixSelector() ast.Expr {
	var metricName string

	if p.curTokenIs(lexer.IDENT) {
		metricName = p.curToken.Literal
		p.nextToken()
	}

	// Parse optional label matchers: { ... }
	var matchers []*ast.LabelMatcher
	if p.curTokenIs(lexer.LBRACE) {
		matchers = p.parseLabelMatchers()
	}

	selector := &ast.MetricSelector{Name: metricName, Matchers: matchers}
	vectorSelector := &ast.VectorSelector{MetricSelector: selector}

	// Check for range selector: [ duration ]
	if p.curTokenIs(lexer.LBRACKET) {
		p.nextToken() // skip '['

		if !p.curTokenIs(lexer.DURATION) && !p.curTokenIs(lexer.NUMBER) {
			p.addError("expected duration in range selector")
			return nil
		}

		dur, err := parseDuration(p.curToken.Literal)
		if err != nil {
			p.addError(fmt.Sprintf("invalid duration: %v", err))
			return nil
		}
		p.nextToken() // past duration

		// Optional subquery syntax: [range:step]
		var step time.Duration
		if p.curTokenIs(lexer.COLON) {
			p.nextToken() // skip ':'
			if p.curTokenIs(lexer.DURATION) || p.curTokenIs(lexer.NUMBER) {
				step, err = parseDuration(p.curToken.Literal)
				if err != nil {
					p.addError(fmt.Sprintf("invalid step duration: %v", err))
					return nil
				}
				p.nextToken()
			}
		}

		if !p.curTokenIs(lexer.RBRACKET) {
			p.addError("expected ']' after range selector")
			return nil
		}
		p.nextToken() // skip ']'

		// If this is a subquery (has a step or the inner expression is not just a selector)
		if step != 0 {
			matrixSelector := &ast.MatrixSelector{VectorSelector: vectorSelector, Range: dur}
			p.parseSelectorModifiers(vectorSelector)
			return matrixSelector
		}

		matrixSelector := &ast.MatrixSelector{VectorSelector: vectorSelector, Range: dur}

		// Optional @ and offset in any order
		p.parseSelectorModifiers(vectorSelector)

		return matrixSelector
	}

	// Optional @ and offset on instant vector
	p.parseSelectorModifiers(vectorSelector)

	return vectorSelector
}

// parseSelectorModifiers handles the optional @ and offset modifiers on selectors.
// They can appear in any order: metric[5m] offset 1d @ 170000 or metric[5m] @ 170000 offset 1d
func (p *Parser) parseSelectorModifiers(vs *ast.VectorSelector) {
	for p.curTokenIs(lexer.OFFSET) || p.curTokenIs(lexer.AT) {
		if p.curTokenIs(lexer.OFFSET) {
			p.nextToken() // past 'offset'
			if !p.curTokenIs(lexer.DURATION) {
				p.addError("expected duration after offset")
				return
			}
			offset, err := parseDuration(p.curToken.Literal)
			if err != nil {
				p.addError(fmt.Sprintf("invalid offset: %v", err))
				return
			}
			vs.Offset = offset
			p.nextToken()
		} else if p.curTokenIs(lexer.AT) {
			p.nextToken() // past '@'
			if !p.curTokenIs(lexer.NUMBER) {
				p.addError("expected timestamp after @")
				return
			}
			ts, err := strconv.ParseFloat(p.curToken.Literal, 64)
			if err != nil {
				p.addError(fmt.Sprintf("invalid @ timestamp: %v", err))
				return
			}
			vs.Timestamp = &ts
			p.nextToken()
		}
	}
}

// parseLabelMatchers parses {label="value", ...} and leaves curToken past '}'
func (p *Parser) parseLabelMatchers() []*ast.LabelMatcher {
	p.nextToken() // skip '{'

	var matchers []*ast.LabelMatcher

	for !p.curTokenIs(lexer.RBRACE) && !p.curTokenIs(lexer.EOF) {
		if !p.curTokenIs(lexer.IDENT) {
			p.addError("expected label name in selector")
			return nil
		}

		labelName := p.curToken.Literal
		p.nextToken() // past label name

		// Parse matching operator
		var op ast.MatchOp
		switch p.curToken.Type {
		case lexer.EQLMATCH:
			op = ast.MatchEqual
		case lexer.NEQ:
			op = ast.MatchNotEqual
		case lexer.REGEX:
			op = ast.MatchRegexp
		case lexer.NREGEX:
			op = ast.MatchNotRegexp
		default:
			p.addError(fmt.Sprintf("expected label match operator, got %v", p.curToken.Type))
			return nil
		}
		p.nextToken() // past operator

		if !p.curTokenIs(lexer.STRING) {
			p.addError("expected string value for label matcher")
			return nil
		}
		labelValue := p.curToken.Literal
		p.nextToken() // past value

		matchers = append(matchers, &ast.LabelMatcher{
			Name:     labelName,
			Operator: op,
			Value:    labelValue,
		})

		if p.curTokenIs(lexer.COMMA) {
			p.nextToken() // skip comma
		}
	}

	if !p.curTokenIs(lexer.RBRACE) {
		p.addError("expected '}' to close label matchers")
		return nil
	}
	p.nextToken() // skip '}'

	return matchers
}

// ─── AGGREGATION ─────────────────────────────────────────────────────────────
// Supports both syntaxes:
//   sum(expr) by (labels)
//   sum by (labels) (expr)

func (p *Parser) parseAggregateExpr() ast.Expr {
	aggOp := tokenToAggOp(p.curToken.Type)
	p.nextToken() // past aggregation keyword

	needsParam := aggOp == ast.AggTopK || aggOp == ast.AggBottomK ||
		aggOp == ast.AggQuantile || aggOp == ast.AggCountValues

	// ── Handle pre-grouping syntax: sum by (labels) (expr) ──
	var grouping []string
	without := false
	preGrouping := false

	if p.curTokenIs(lexer.BY) || p.curTokenIs(lexer.WITHOUT) {
		without = p.curTokenIs(lexer.WITHOUT)
		p.nextToken() // past by/without
		grouping = p.parseGroupingLabels()
		preGrouping = true
	}

	// ── Expect '(' ──
	if !p.curTokenIs(lexer.LPAREN) {
		p.addError("expected '(' after aggregation operator")
		return nil
	}
	p.nextToken() // skip '('

	// ── Optional parameter for topk / bottomk / quantile / count_values ──
	var param ast.Expr
	if needsParam {
		param = p.parseOrExpr()
		if !p.curTokenIs(lexer.COMMA) {
			p.addError("expected ',' after aggregation parameter")
			return nil
		}
		p.nextToken() // skip ','
	}

	// ── Parse inner expression ──
	expr := p.parseOrExpr()

	// ── Expect ')' ──
	if !p.curTokenIs(lexer.RPAREN) {
		p.addError("expected ')' after aggregation expression")
		return nil
	}
	p.nextToken() // skip ')'

	// ── Handle post-grouping syntax: sum(expr) by (labels) ──
	if !preGrouping && (p.curTokenIs(lexer.BY) || p.curTokenIs(lexer.WITHOUT)) {
		without = p.curTokenIs(lexer.WITHOUT)
		p.nextToken() // past by/without
		grouping = p.parseGroupingLabels()
	}

	return &ast.AggregateExpr{
		Op:       aggOp,
		Expr:     expr,
		Param:    param,
		Grouping: grouping,
		Without:  without,
	}
}

// parseGroupingLabels parses (label1, label2, ...) — expects curToken to be '('
func (p *Parser) parseGroupingLabels() []string {
	if !p.curTokenIs(lexer.LPAREN) {
		p.addError("expected '(' for label grouping")
		return nil
	}
	p.nextToken() // skip '('

	var labels []string
	for !p.curTokenIs(lexer.RPAREN) && !p.curTokenIs(lexer.EOF) {
		if p.curTokenIs(lexer.IDENT) || p.isKeywordUsableAsLabel() {
			labels = append(labels, p.curToken.Literal)
			p.nextToken()
		} else {
			p.addError(fmt.Sprintf("expected label name in grouping, got %v", p.curToken.Type))
			return nil
		}
		if p.curTokenIs(lexer.COMMA) {
			p.nextToken()
		}
	}
	if !p.curTokenIs(lexer.RPAREN) {
		p.addError("expected ')' to close label grouping")
		return nil
	}
	p.nextToken() // skip ')'
	return labels
}

// ─── FUNCTION CALL ───────────────────────────────────────────────────────────
// Handles both lexer-keyword functions (rate, irate, ...) and
// identifier-based functions (avg_over_time, sin, ...) via the shared
// parseFunctionCallWithType entry point.

func (p *Parser) parseFunctionCallWithType(funcType ast.FuncType) ast.Expr {
	p.nextToken() // past function name / keyword

	if !p.curTokenIs(lexer.LPAREN) {
		p.addError(fmt.Sprintf("expected '(' after function %s", funcType))
		return nil
	}
	p.nextToken() // skip '('

	var args []ast.Expr
	for !p.curTokenIs(lexer.RPAREN) && !p.curTokenIs(lexer.EOF) {
		arg := p.parseOrExpr()
		if arg != nil {
			args = append(args, arg)
		}
		if p.curTokenIs(lexer.COMMA) {
			p.nextToken()
		}
	}

	if !p.curTokenIs(lexer.RPAREN) {
		p.addError(fmt.Sprintf("expected ')' after function %s arguments", funcType))
		return nil
	}
	p.nextToken() // skip ')'

	// Validate minimum argument counts for functions that require them
	minArgs := funcMinArgs(funcType)
	if len(args) < minArgs {
		p.addError(fmt.Sprintf("function %s requires at least %d argument(s), got %d", funcType, minArgs, len(args)))
		return nil
	}

	return &ast.Call{Func: funcType, Args: args}
}

// ─── VECTOR MATCHING ─────────────────────────────────────────────────────────
// Parses: [on(labels)|ignoring(labels)] [group_left(labels)|group_right(labels)]
// Called after a binary operator and before the right-hand expression.

func (p *Parser) parseVectorMatching() *ast.VectorMatching {
	if !p.curTokenIs(lexer.ON) && !p.curTokenIs(lexer.IGNORING) {
		return nil
	}

	vm := &ast.VectorMatching{}

	if p.curTokenIs(lexer.ON) {
		p.nextToken() // past 'on'
		vm.On = p.parseGroupingLabels()
	} else {
		p.nextToken() // past 'ignoring'
		vm.Ignoring = p.parseGroupingLabels()
	}

	// Optional group_left / group_right
	if p.curTokenIs(lexer.GROUP_LEFT) {
		p.nextToken() // past group_left
		vm.GroupLeft = []string{} // non-nil → group_left was specified
		if p.curTokenIs(lexer.LPAREN) {
			p.nextToken() // skip '('
			for !p.curTokenIs(lexer.RPAREN) && !p.curTokenIs(lexer.EOF) {
				if p.curTokenIs(lexer.IDENT) {
					vm.GroupLeft = append(vm.GroupLeft, p.curToken.Literal)
					p.nextToken()
				}
				if p.curTokenIs(lexer.COMMA) {
					p.nextToken()
				}
			}
			if p.curTokenIs(lexer.RPAREN) {
				p.nextToken()
			}
		}
	} else if p.curTokenIs(lexer.GROUP_RIGHT) {
		p.nextToken() // past group_right
		vm.GroupRight = []string{} // non-nil → group_right was specified
		if p.curTokenIs(lexer.LPAREN) {
			p.nextToken() // skip '('
			for !p.curTokenIs(lexer.RPAREN) && !p.curTokenIs(lexer.EOF) {
				if p.curTokenIs(lexer.IDENT) {
					vm.GroupRight = append(vm.GroupRight, p.curToken.Literal)
					p.nextToken()
				}
				if p.curTokenIs(lexer.COMMA) {
					p.nextToken()
				}
			}
			if p.curTokenIs(lexer.RPAREN) {
				p.nextToken()
			}
		}
	}

	return vm
}

// ─── HELPER: TOKEN CLASSIFICATION ────────────────────────────────────────────

func (p *Parser) isComparisonOp() bool {
	switch p.curToken.Type {
	case lexer.EQL, lexer.NEQ, lexer.LT, lexer.GT, lexer.LTE, lexer.GTE:
		return true
	}
	return false
}

func (p *Parser) isAggregateOp() bool {
	switch p.curToken.Type {
	case lexer.SUM, lexer.MIN, lexer.MAX, lexer.AVG,
		lexer.STDDEV, lexer.STDVAR, lexer.COUNT, lexer.COUNT_VALUES,
		lexer.BOTTOMK, lexer.TOPK, lexer.QUANTILE, lexer.GROUP:
		return true
	}
	return false
}

// isFunctionKeyword returns true if curToken is a function recognised as
// its own lexer keyword (the original ~20 functions).
func (p *Parser) isFunctionKeyword() bool {
	switch p.curToken.Type {
	case lexer.RATE, lexer.IRATE, lexer.INCREASE, lexer.DELTA, lexer.IDELTA,
		lexer.HISTOGRAM_QUANTILE, lexer.ABS, lexer.CEIL, lexer.FLOOR,
		lexer.ROUND, lexer.CLAMP_MAX, lexer.CLAMP_MIN, lexer.CHANGES,
		lexer.RESETS, lexer.DERIV, lexer.PREDICT_LINEAR, lexer.SORT,
		lexer.SORT_DESC, lexer.TIME, lexer.VECTOR, lexer.SCALAR:
		return true
	}
	return false
}

// isKeywordUsableAsLabel returns true when curToken is a keyword that can also
// appear as a label name inside by/without groupings.
func (p *Parser) isKeywordUsableAsLabel() bool {
	switch p.curToken.Type {
	case lexer.BOOL, lexer.ON, lexer.IGNORING, lexer.GROUP_LEFT, lexer.GROUP_RIGHT, lexer.OFFSET:
		return true
	}
	return false
}

// ─── HELPER: TOKEN → AST TYPE CONVERSION ─────────────────────────────────────

func tokenToBinOp(t lexer.TokenType) ast.BinOp {
	switch t {
	case lexer.ADD:
		return ast.OpAdd
	case lexer.SUB:
		return ast.OpSub
	case lexer.MUL:
		return ast.OpMul
	case lexer.DIV:
		return ast.OpDiv
	case lexer.MOD:
		return ast.OpMod
	case lexer.POW:
		return ast.OpPow
	case lexer.EQL:
		return ast.OpEql
	case lexer.NEQ:
		return ast.OpNeq
	case lexer.LT:
		return ast.OpLt
	case lexer.GT:
		return ast.OpGt
	case lexer.LTE:
		return ast.OpLte
	case lexer.GTE:
		return ast.OpGte
	case lexer.AND:
		return ast.OpAnd
	case lexer.OR:
		return ast.OpOr
	case lexer.UNLESS:
		return ast.OpUnless
	default:
		return ""
	}
}

func tokenToAggOp(t lexer.TokenType) ast.AggOp {
	switch t {
	case lexer.SUM:
		return ast.AggSum
	case lexer.MIN:
		return ast.AggMin
	case lexer.MAX:
		return ast.AggMax
	case lexer.AVG:
		return ast.AggAvg
	case lexer.STDDEV:
		return ast.AggStddev
	case lexer.STDVAR:
		return ast.AggStdvar
	case lexer.COUNT:
		return ast.AggCount
	case lexer.COUNT_VALUES:
		return ast.AggCountValues
	case lexer.BOTTOMK:
		return ast.AggBottomK
	case lexer.TOPK:
		return ast.AggTopK
	case lexer.QUANTILE:
		return ast.AggQuantile
	case lexer.GROUP:
		return ast.AggGroup
	default:
		return ""
	}
}

func tokenToFuncType(t lexer.TokenType) ast.FuncType {
	switch t {
	case lexer.RATE:
		return ast.FuncRate
	case lexer.IRATE:
		return ast.FuncIRate
	case lexer.INCREASE:
		return ast.FuncIncrease
	case lexer.DELTA:
		return ast.FuncDelta
	case lexer.IDELTA:
		return ast.FuncIDelta
	case lexer.HISTOGRAM_QUANTILE:
		return ast.FuncHistogramQuantile
	case lexer.ABS:
		return ast.FuncAbs
	case lexer.CEIL:
		return ast.FuncCeil
	case lexer.FLOOR:
		return ast.FuncFloor
	case lexer.ROUND:
		return ast.FuncRound
	case lexer.CLAMP_MAX:
		return ast.FuncClampMax
	case lexer.CLAMP_MIN:
		return ast.FuncClampMin
	case lexer.CHANGES:
		return ast.FuncChanges
	case lexer.RESETS:
		return ast.FuncResets
	case lexer.DERIV:
		return ast.FuncDeriv
	case lexer.PREDICT_LINEAR:
		return ast.FuncPredictLinear
	case lexer.SORT:
		return ast.FuncSort
	case lexer.SORT_DESC:
		return ast.FuncSortDesc
	case lexer.TIME:
		return ast.FuncTime
	case lexer.VECTOR:
		return ast.FuncVector
	case lexer.SCALAR:
		return ast.FuncScalar
	default:
		return ""
	}
}

// ─── DURATION PARSING ────────────────────────────────────────────────────────

func parseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, fmt.Errorf("empty duration string")
	}

	// Determine the unit suffix
	numPart := s
	unit := ""

	// Handle "ms" first (2-char unit)
	if len(s) >= 3 && s[len(s)-2:] == "ms" {
		numPart = s[:len(s)-2]
		unit = "ms"
	} else if len(s) >= 2 {
		lastChar := s[len(s)-1:]
		if lastChar >= "a" && lastChar <= "z" || lastChar >= "A" && lastChar <= "Z" {
			numPart = s[:len(s)-1]
			unit = lastChar
		}
	}

	if unit == "" {
		return 0, fmt.Errorf("no duration unit in %q", s)
	}

	value, err := strconv.ParseFloat(numPart, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid numeric part in duration %q: %w", s, err)
	}

	var multiplier time.Duration
	switch unit {
	case "ms":
		multiplier = time.Millisecond
	case "s":
		multiplier = time.Second
	case "m":
		multiplier = time.Minute
	case "h":
		multiplier = time.Hour
	case "d":
		multiplier = 24 * time.Hour
	case "w":
		multiplier = 7 * 24 * time.Hour
	case "y":
		multiplier = 365 * 24 * time.Hour
	default:
		return 0, fmt.Errorf("unknown duration unit %q in %q", unit, s)
	}

	return time.Duration(value * float64(multiplier)), nil
}

// Parse is a convenience function to parse a PromQL string.
func Parse(input string) (ast.Expr, error) {
	l := lexer.New(input)
	p := New(l)
	return p.ParseExpr()
}

// funcMinArgs returns the minimum number of arguments required for a function.
// Functions that need at least 1 argument (like rate, irate, etc.) return 1.
// Functions that take 0 args return 0.
func funcMinArgs(ft ast.FuncType) int {
	switch ft {
	// These functions can be called with 0 arguments:
	case ast.FuncTime, ast.FuncPi:
		return 0
	// Date/time functions: 0 args → current time, 1 arg → given vector
	case ast.FuncDayOfMonth, ast.FuncDayOfWeek, ast.FuncDayOfYear,
		ast.FuncDaysInMonth, ast.FuncHour, ast.FuncMinute,
		ast.FuncMonth, ast.FuncYear:
		return 0
	// Trig and math functions: technically require 1 but allow 0 for
	// PromQL-compatibility in some implementations
	case ast.FuncSin, ast.FuncCos, ast.FuncTan,
		ast.FuncAsin, ast.FuncAcos, ast.FuncAtan,
		ast.FuncSinh, ast.FuncCosh, ast.FuncTanh,
		ast.FuncAsinh, ast.FuncAcosh, ast.FuncAtanh,
		ast.FuncDeg, ast.FuncRad, ast.FuncSgn,
		ast.FuncTimestamp:
		return 0
	default:
		// Most PromQL functions require at least 1 argument
		return 1
	}
}
