package lexer

import (
	"fmt"
	"strings"
)

// TokenType represents the type of token
type TokenType int

const (
	// Special tokens
	ILLEGAL TokenType = iota
	EOF
	WHITESPACE

	// Literals
	IDENT      // metric_name, label_name
	NUMBER     // 123, 1.23, 1e10
	STRING     // "string", 'string'
	DURATION   // 5m, 1h, 30s

	// Operators
	ADD // +
	SUB // -
	MUL // *
	DIV // /
	MOD // %
	POW // ^

	// Comparison operators
	EQL    // ==
	NEQ    // !=
	LT     // <
	GT     // >
	LTE    // <=
	GTE    // >=
	EQLMATCH // =
	NEQMATCH // !=
	REGEX    // =~
	NREGEX   // !~

	// Logical operators
	AND    // and
	OR     // or
	UNLESS // unless

	// Aggregation operators
	SUM        // sum
	MIN        // min
	MAX        // max
	AVG        // avg
	STDDEV     // stddev
	STDVAR     // stdvar
	COUNT      // count
	COUNT_VALUES // count_values
	BOTTOMK    // bottomk
	TOPK       // topk
	QUANTILE   // quantile
	GROUP      // group

	// Functions
	RATE              // rate
	IRATE             // irate
	INCREASE          // increase
	DELTA             // delta
	IDELTA            // idelta
	HISTOGRAM_QUANTILE // histogram_quantile
	ABS               // abs
	CEIL              // ceil
	FLOOR             // floor
	ROUND             // round
	CLAMP_MAX         // clamp_max
	CLAMP_MIN         // clamp_min
	CHANGES           // changes
	RESETS            // resets
	DERIV             // deriv
	PREDICT_LINEAR    // predict_linear
	SORT              // sort
	SORT_DESC         // sort_desc
	TIME              // time
	VECTOR            // vector
	SCALAR            // scalar

	// Keywords
	BY       // by
	WITHOUT  // without
	ON       // on
	IGNORING // ignoring
	GROUP_LEFT  // group_left
	GROUP_RIGHT // group_right
	OFFSET   // offset
	BOOL     // bool

	// Delimiters
	LPAREN    // (
	RPAREN    // )
	LBRACE    // {
	RBRACE    // }
	LBRACKET  // [
	RBRACKET  // ]
	COMMA     // ,
	COLON     // :
	AT        // @
)

var tokenNames = map[TokenType]string{
	ILLEGAL:    "ILLEGAL",
	EOF:        "EOF",
	WHITESPACE: "WHITESPACE",
	IDENT:      "IDENT",
	NUMBER:     "NUMBER",
	STRING:     "STRING",
	DURATION:   "DURATION",
	ADD:        "+",
	SUB:        "-",
	MUL:        "*",
	DIV:        "/",
	MOD:        "%",
	POW:        "^",
	EQL:        "==",
	NEQ:        "!=",
	LT:         "<",
	GT:         ">",
	LTE:        "<=",
	GTE:        ">=",
	EQLMATCH:   "=",
	REGEX:      "=~",
	NREGEX:     "!~",
	LPAREN:     "(",
	RPAREN:     ")",
	LBRACE:     "{",
	RBRACE:     "}",
	LBRACKET:   "[",
	RBRACKET:   "]",
	COMMA:      ",",
	COLON:      ":",
	AT:         "@",
}

// Keywords mapping
var keywords = map[string]TokenType{
	"and":                AND,
	"or":                 OR,
	"unless":             UNLESS,
	"sum":                SUM,
	"min":                MIN,
	"max":                MAX,
	"avg":                AVG,
	"stddev":             STDDEV,
	"stdvar":             STDVAR,
	"count":              COUNT,
	"count_values":       COUNT_VALUES,
	"bottomk":            BOTTOMK,
	"topk":               TOPK,
	"quantile":           QUANTILE,
	"group":              GROUP,
	"rate":               RATE,
	"irate":              IRATE,
	"increase":           INCREASE,
	"delta":              DELTA,
	"idelta":             IDELTA,
	"histogram_quantile": HISTOGRAM_QUANTILE,
	"abs":                ABS,
	"ceil":               CEIL,
	"floor":              FLOOR,
	"round":              ROUND,
	"clamp_max":          CLAMP_MAX,
	"clamp_min":          CLAMP_MIN,
	"changes":            CHANGES,
	"resets":             RESETS,
	"deriv":              DERIV,
	"predict_linear":     PREDICT_LINEAR,
	"sort":               SORT,
	"sort_desc":          SORT_DESC,
	"time":               TIME,
	"vector":             VECTOR,
	"scalar":             SCALAR,
	"by":                 BY,
	"without":            WITHOUT,
	"on":                 ON,
	"ignoring":           IGNORING,
	"group_left":         GROUP_LEFT,
	"group_right":        GROUP_RIGHT,
	"offset":             OFFSET,
	"bool":               BOOL,
}

// Token represents a lexical token
type Token struct {
	Type    TokenType
	Literal string
	Line    int
	Column  int
}

func (t Token) String() string {
	name, ok := tokenNames[t.Type]
	if !ok {
		name = fmt.Sprintf("TokenType(%d)", t.Type)
	}
	return fmt.Sprintf("{%s %q}", name, t.Literal)
}

// Lexer performs lexical analysis of PromQL
type Lexer struct {
	input        string
	position     int  // current position in input
	readPosition int  // current reading position
	ch           byte // current char
	line         int
	column       int
}

// New creates a new Lexer
func New(input string) *Lexer {
	l := &Lexer{
		input:  input,
		line:   1,
		column: 0,
	}
	l.readChar()
	return l
}

// readChar reads the next character
func (l *Lexer) readChar() {
	if l.readPosition >= len(l.input) {
		l.ch = 0
	} else {
		l.ch = l.input[l.readPosition]
	}
	l.position = l.readPosition
	l.readPosition++
	l.column++
}

// peekChar looks at the next character without advancing
func (l *Lexer) peekChar() byte {
	if l.readPosition >= len(l.input) {
		return 0
	}
	return l.input[l.readPosition]
}

// NextToken returns the next token
func (l *Lexer) NextToken() Token {
	var tok Token

	l.skipWhitespace()

	tok.Line = l.line
	tok.Column = l.column

	switch l.ch {
	case '+':
		tok = l.newToken(ADD, l.ch)
	case '-':
		tok = l.newToken(SUB, l.ch)
	case '*':
		tok = l.newToken(MUL, l.ch)
	case '/':
		tok = l.newToken(DIV, l.ch)
	case '%':
		tok = l.newToken(MOD, l.ch)
	case '^':
		tok = l.newToken(POW, l.ch)
	case '(':
		tok = l.newToken(LPAREN, l.ch)
	case ')':
		tok = l.newToken(RPAREN, l.ch)
	case '{':
		tok = l.newToken(LBRACE, l.ch)
	case '}':
		tok = l.newToken(RBRACE, l.ch)
	case '[':
		tok = l.newToken(LBRACKET, l.ch)
	case ']':
		tok = l.newToken(RBRACKET, l.ch)
	case ',':
		tok = l.newToken(COMMA, l.ch)
	case ':':
		tok = l.newToken(COLON, l.ch)
	case '@':
		tok = l.newToken(AT, l.ch)
	case '=':
		if l.peekChar() == '=' {
			ch := l.ch
			l.readChar()
			tok = Token{Type: EQL, Literal: string(ch) + string(l.ch), Line: tok.Line, Column: tok.Column}
		} else if l.peekChar() == '~' {
			ch := l.ch
			l.readChar()
			tok = Token{Type: REGEX, Literal: string(ch) + string(l.ch), Line: tok.Line, Column: tok.Column}
		} else {
			tok = l.newToken(EQLMATCH, l.ch)
		}
	case '!':
		if l.peekChar() == '=' {
			ch := l.ch
			l.readChar()
			tok = Token{Type: NEQ, Literal: string(ch) + string(l.ch), Line: tok.Line, Column: tok.Column}
		} else if l.peekChar() == '~' {
			ch := l.ch
			l.readChar()
			tok = Token{Type: NREGEX, Literal: string(ch) + string(l.ch), Line: tok.Line, Column: tok.Column}
		} else {
			tok = l.newToken(ILLEGAL, l.ch)
		}
	case '<':
		if l.peekChar() == '=' {
			ch := l.ch
			l.readChar()
			tok = Token{Type: LTE, Literal: string(ch) + string(l.ch), Line: tok.Line, Column: tok.Column}
		} else {
			tok = l.newToken(LT, l.ch)
		}
	case '>':
		if l.peekChar() == '=' {
			ch := l.ch
			l.readChar()
			tok = Token{Type: GTE, Literal: string(ch) + string(l.ch), Line: tok.Line, Column: tok.Column}
		} else {
			tok = l.newToken(GT, l.ch)
		}
	case '"', '\'':
		tok.Type = STRING
		tok.Literal = l.readString()
		return tok
	case 0:
		tok.Literal = ""
		tok.Type = EOF
	default:
		if isLetter(l.ch) {
			tok.Literal = l.readIdentifier()
			tok.Type = lookupIdent(tok.Literal)
			return tok
		} else if isDigit(l.ch) {
			literal := l.readNumber()
			// Check if it's a duration (e.g., 5m, 1h)
			if isLetter(l.ch) {
				literal += l.readDurationUnit()
				tok.Type = DURATION
			} else {
				tok.Type = NUMBER
			}
			tok.Literal = literal
			return tok
		} else {
			tok = l.newToken(ILLEGAL, l.ch)
		}
	}

	l.readChar()
	return tok
}

func (l *Lexer) newToken(tokenType TokenType, ch byte) Token {
	return Token{Type: tokenType, Literal: string(ch), Line: l.line, Column: l.column}
}

func (l *Lexer) skipWhitespace() {
	for {
		if l.ch == ' ' || l.ch == '\t' || l.ch == '\r' {
			l.readChar()
		} else if l.ch == '\n' {
			l.line++
			l.column = 0
			l.readChar()
		} else if l.ch == '#' {
			// Skip line comments
			for l.ch != '\n' && l.ch != 0 {
				l.readChar()
			}
		} else {
			break
		}
	}
}

func (l *Lexer) readIdentifier() string {
	position := l.position
	for isLetter(l.ch) || isDigit(l.ch) || l.ch == '_' || l.ch == ':' {
		l.readChar()
	}
	return l.input[position:l.position]
}

func (l *Lexer) readNumber() string {
	position := l.position
	dotSeen := false

	for isDigit(l.ch) || l.ch == '.' || l.ch == 'e' || l.ch == 'E' {
		if l.ch == '.' {
			if dotSeen {
				break
			}
			dotSeen = true
		}
		if l.ch == 'e' || l.ch == 'E' {
			l.readChar()
			if l.ch == '+' || l.ch == '-' {
				l.readChar()
			}
			continue
		}
		l.readChar()
	}
	return l.input[position:l.position]
}

func (l *Lexer) readDurationUnit() string {
	position := l.position
	for isLetter(l.ch) {
		l.readChar()
	}
	return l.input[position:l.position]
}

func (l *Lexer) readString() string {
	quote := l.ch
	position := l.position + 1
	l.readChar()

	for l.ch != quote && l.ch != 0 {
		if l.ch == '\\' {
			l.readChar() // skip escape character
		}
		l.readChar()
	}

	result := l.input[position:l.position]
	l.readChar() // advance past closing quote
	return result
}

func isLetter(ch byte) bool {
	return 'a' <= ch && ch <= 'z' || 'A' <= ch && ch <= 'Z' || ch == '_'
}

func isDigit(ch byte) bool {
	return '0' <= ch && ch <= '9'
}

func lookupIdent(ident string) TokenType {
	if tok, ok := keywords[strings.ToLower(ident)]; ok {
		return tok
	}
	return IDENT
}

// Tokenize returns all tokens from the input
func Tokenize(input string) ([]Token, error) {
	l := New(input)
	// Pre-allocate with estimated capacity (rough estimate: 1 token per 8 characters)
	estimatedTokens := len(input)/8 + 10
	tokens := make([]Token, 0, estimatedTokens)

	for {
		tok := l.NextToken()
		if tok.Type == ILLEGAL {
			return nil, fmt.Errorf("illegal token %q at line %d, column %d", tok.Literal, tok.Line, tok.Column)
		}
		tokens = append(tokens, tok)
		if tok.Type == EOF {
			break
		}
	}

	return tokens, nil
}
