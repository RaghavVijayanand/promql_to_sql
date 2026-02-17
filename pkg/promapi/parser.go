package promapi

import (
	"fmt"
)

// Parser wraps the API client to provide a parser-like interface
type Parser struct {
	client *Client
}

// NewParser creates a new API-based parser
func NewParser(prometheusURL string) *Parser {
	return &Parser{
		client: NewClient(prometheusURL),
	}
}

// Parse parses a PromQL query using the Prometheus API and returns the JSON AST
func (p *Parser) Parse(query string) (*ASTNode, error) {
	// Get AST from Prometheus API
	astNode, err := p.client.ParseQuery(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get AST from Prometheus API: %w", err)
	}

	return astNode, nil
}
