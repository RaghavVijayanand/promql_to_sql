package promapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Client handles communication with Prometheus HTTP API
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new Prometheus API client
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// ParseQueryResponse represents the response from /api/v1/parse_query
type ParseQueryResponse struct {
	Status string   `json:"status"`
	Data   *ASTNode `json:"data"`
	Error  string   `json:"error,omitempty"`
}

// ASTNode represents a node in the Prometheus AST
type ASTNode struct {
	Type string `json:"type"`

	// For vectorSelector and matrixSelector
	Name       string    `json:"name,omitempty"`
	Matchers   []Matcher `json:"matchers,omitempty"`
	Offset     int64     `json:"offset,omitempty"`     // milliseconds
	Timestamp  *float64  `json:"timestamp,omitempty"`  // Unix seconds
	StartOrEnd *string   `json:"startOrEnd,omitempty"` // "start" or "end"
	Range      int64     `json:"range,omitempty"`      // milliseconds (for matrixSelector)
	Anchored   bool      `json:"anchored,omitempty"`
	Smoothed   bool      `json:"smoothed,omitempty"`

	// For binaryExpr
	Op       string          `json:"op,omitempty"`
	LHS      *ASTNode        `json:"lhs,omitempty"`
	RHS      *ASTNode        `json:"rhs,omitempty"`
	Bool     bool            `json:"bool,omitempty"`
	Matching *VectorMatching `json:"matching,omitempty"`

	// For unaryExpr
	Expr *ASTNode `json:"expr,omitempty"`

	// For call (function)
	Func *FuncDef  `json:"func,omitempty"`
	Args []ASTNode `json:"args,omitempty"`

	// For aggregation
	Grouping []string `json:"grouping,omitempty"`
	Without  bool     `json:"without,omitempty"`
	Param    *ASTNode `json:"param,omitempty"`

	// For numberLiteral
	Val string `json:"val,omitempty"`
}

// Matcher represents a label matcher
type Matcher struct {
	Name  string `json:"name"`
	Type  string `json:"type"` // "=", "!=", "=~", "!~"
	Value string `json:"value"`
}

// VectorMatching represents vector matching rules in binary operations
type VectorMatching struct {
	Card    string   `json:"card"`              // "one-to-one", "one-to-many", "many-to-one"
	On      bool     `json:"on"`                // true if ON clause used
	Include []string `json:"include,omitempty"` // group_left/group_right labels
	Labels  []string `json:"labels,omitempty"`  // ON or IGNORING labels
}

// FuncDef represents a function definition
type FuncDef struct {
	Name       string   `json:"name"`
	ArgTypes   []string `json:"argTypes"`
	ReturnType string   `json:"returnType"`
	Variadic   int      `json:"variadic"`
}

// ParseQuery sends a PromQL query to Prometheus /api/v1/parse_query and returns the AST
func (c *Client) ParseQuery(query string) (*ASTNode, error) {
	// Use POST with form-encoded body to avoid URL length limits
	formData := url.Values{}
	formData.Set("query", query)

	reqURL := fmt.Sprintf("%s/api/v1/parse_query", c.baseURL)
	req, err := http.NewRequest("POST", reqURL, bytes.NewBufferString(formData.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result ParseQueryResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Status != "success" {
		return nil, fmt.Errorf("API error: %s", result.Error)
	}

	if result.Data == nil {
		return nil, fmt.Errorf("no AST data in response")
	}

	return result.Data, nil
}
