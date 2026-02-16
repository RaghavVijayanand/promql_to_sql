package lexer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLexer_BasicTokens(t *testing.T) {
	input := `http_requests_total{job="api",status="200"}[5m]`

	l := New(input)
	
	tests := []struct {
		expectedType    TokenType
		expectedLiteral string
	}{
		{IDENT, "http_requests_total"},
		{LBRACE, "{"},
		{IDENT, "job"},
		{EQLMATCH, "="},
		{STRING, "api"},
		{COMMA, ","},
		{IDENT, "status"},
		{EQLMATCH, "="},
		{STRING, "200"},
		{RBRACE, "}"},
		{LBRACKET, "["},
		{DURATION, "5m"},
		{RBRACKET, "]"},
		{EOF, ""},
	}

	for i, tt := range tests {
		tok := l.NextToken()
		
		assert.Equal(t, tt.expectedType, tok.Type, 
			"tests[%d] - token type wrong. expected=%v, got=%v", i, tt.expectedType, tok.Type)
		
		assert.Equal(t, tt.expectedLiteral, tok.Literal,
			"tests[%d] - literal wrong. expected=%q, got=%q", i, tt.expectedLiteral, tok.Literal)
	}
}

func TestLexer_Operators(t *testing.T) {
	input := `+ - * / % ^ == != < > <= >= =~ !~`

	l := New(input)
	
	tests := []TokenType{
		ADD, SUB, MUL, DIV, MOD, POW,
		EQL, NEQ, LT, GT, LTE, GTE,
		REGEX, NREGEX, EOF,
	}

	for i, expectedType := range tests {
		tok := l.NextToken()
		assert.Equal(t, expectedType, tok.Type,
			"tests[%d] - token type wrong. expected=%v, got=%v", i, expectedType, tok.Type)
	}
}

func TestLexer_Keywords(t *testing.T) {
	input := `sum avg max min count rate by without`

	l := New(input)
	
	tests := []TokenType{
		SUM, AVG, MAX, MIN, COUNT, RATE, BY, WITHOUT, EOF,
	}

	for i, expectedType := range tests {
		tok := l.NextToken()
		assert.Equal(t, expectedType, tok.Type,
			"tests[%d] - token type wrong. expected=%v, got=%v", i, expectedType, tok.Type)
	}
}

func TestLexer_Numbers(t *testing.T) {
	input := `123 45.67 1.23e10 1e-5`

	l := New(input)
	
	tests := []struct {
		expectedType    TokenType
		expectedLiteral string
	}{
		{NUMBER, "123"},
		{NUMBER, "45.67"},
		{NUMBER, "1.23e10"},
		{NUMBER, "1e-5"},
		{EOF, ""},
	}

	for i, tt := range tests {
		tok := l.NextToken()
		assert.Equal(t, tt.expectedType, tok.Type,
			"tests[%d] - token type wrong", i)
		assert.Equal(t, tt.expectedLiteral, tok.Literal,
			"tests[%d] - literal wrong", i)
	}
}

func TestLexer_Durations(t *testing.T) {
	input := `5m 1h 30s 2d 1w`

	l := New(input)
	
	tests := []string{"5m", "1h", "30s", "2d", "1w"}

	for i, expected := range tests {
		tok := l.NextToken()
		assert.Equal(t, DURATION, tok.Type,
			"tests[%d] - expected DURATION token", i)
		assert.Equal(t, expected, tok.Literal,
			"tests[%d] - duration literal wrong", i)
	}
}

func TestLexer_ComplexQuery(t *testing.T) {
	input := `sum(rate(http_requests_total[5m])) by (status)`

	tokens, err := Tokenize(input)
	assert.NoError(t, err)
	assert.Greater(t, len(tokens), 0)
	
	// Verify first and last tokens
	assert.Equal(t, SUM, tokens[0].Type)
	assert.Equal(t, EOF, tokens[len(tokens)-1].Type)
}
