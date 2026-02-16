package alertmanager

import (
	"fmt"
	"strings"
	"time"
	
	"github.com/shinro/promql-transpiler/pkg/builder"
)

// AlertRule represents an Alertmanager alert rule
// Following Single Responsibility Principle
type AlertRule struct {
	Name        string
	Expr        string
	Duration    time.Duration
	Labels      map[string]string
	Annotations map[string]string
}

// AlertState represents the state of an alert
type AlertState string

const (
	AlertStateInactive AlertState = "inactive"
	AlertStatePending  AlertState = "pending"
	AlertStateFiring   AlertState = "firing"
)

// AlertEvaluationContext holds context for evaluating alerts
type AlertEvaluationContext struct {
	Rule          *AlertRule
	EvalTime      time.Time
	LookbackDelta time.Duration
	Builder       builder.SQLBuilder
}

// AlertQueryTranspiler transpiles Alertmanager alert expressions to ClickHouse
// Following Strategy Pattern for different alert types
type AlertQueryTranspiler interface {
	TranspileAlertExpression(rule *AlertRule, ctx *AlertEvaluationContext) (string, error)
	TranspileForDuration(expr string, duration time.Duration, ctx *AlertEvaluationContext) (string, error)
	GetAlertState(expr string, ctx *AlertEvaluationContext) (AlertState, error)
}

// DefaultAlertQueryTranspiler implements AlertQueryTranspiler
type DefaultAlertQueryTranspiler struct {
	sqlBuilder builder.SQLBuilder
}

// NewAlertQueryTranspiler creates a new alert query transpiler
func NewAlertQueryTranspiler(sqlBuilder builder.SQLBuilder) AlertQueryTranspiler {
	return &DefaultAlertQueryTranspiler{
		sqlBuilder: sqlBuilder,
	}
}

// TranspileAlertExpression transpiles an alert expression
func (t *DefaultAlertQueryTranspiler) TranspileAlertExpression(
	rule *AlertRule,
	ctx *AlertEvaluationContext,
) (string, error) {
	if rule.Duration > 0 {
		return t.TranspileForDuration(rule.Expr, rule.Duration, ctx)
	}
	
	// Simple alert without FOR duration
	return t.transpileSimpleAlert(rule, ctx)
}

// TranspileForDuration handles alerts with FOR duration
// Alert must be true for the specified duration before firing
func (t *DefaultAlertQueryTranspiler) TranspileForDuration(
	expr string,
	duration time.Duration,
	ctx *AlertEvaluationContext,
) (string, error) {
	builder := ctx.Builder
	builder.Reset()
	
	// Calculate time window
	endTime := ctx.EvalTime.Unix()
	startTime := ctx.EvalTime.Add(-duration).Unix()
	
	// Build query to check if condition has been true for entire duration
	// This uses a window function to check continuous satisfaction
	query := builder.
		With("alert_values", fmt.Sprintf(`
			SELECT 
				timestamp,
				value,
				labels,
				(%s) as condition_met
			FROM metrics
			WHERE timestamp >= toDateTime(%d)
			  AND timestamp <= toDateTime(%d)
			ORDER BY timestamp
		`, expr, startTime, endTime)).
		Select(`
			anyIf(1, min_condition = 1) as should_fire,
			max(timestamp) as last_check_time,
			labels
		`).
		From(`(
			SELECT 
				labels,
				min(condition_met) as min_condition,
				max(timestamp) as timestamp
			FROM alert_values
			GROUP BY labels
		)`).
		GroupBy("labels").
		Build()
	
	return query, nil
}

// GetAlertState evaluates the current state of an alert
func (t *DefaultAlertQueryTranspiler) GetAlertState(
	expr string,
	ctx *AlertEvaluationContext,
) (AlertState, error) {
	// This would execute the query and determine state
	// For now, return a placeholder
	return AlertStateInactive, nil
}

// transpileSimpleAlert transpiles an alert without FOR duration
func (t *DefaultAlertQueryTranspiler) transpileSimpleAlert(
	rule *AlertRule,
	ctx *AlertEvaluationContext,
) (string, error) {
	builder := ctx.Builder
	builder.Reset()
	
	evalTime := ctx.EvalTime.Unix()
	lookback := ctx.EvalTime.Add(-ctx.LookbackDelta).Unix()
	
	// Build simple evaluation query
	query := builder.
		Select("labels", "value", "timestamp").
		SelectExpr(fmt.Sprintf("(%s) as condition_met", rule.Expr), "").
		From("metrics").
		Where(fmt.Sprintf("timestamp >= toDateTime(%d)", lookback)).
		And(fmt.Sprintf("timestamp <= toDateTime(%d)", evalTime)).
		GroupBy("labels").
		Having("condition_met = 1").
		Build()
	
	return query, nil
}

// AlertStateTracker tracks alert states over time
// Following State Pattern
type AlertStateTracker interface {
	UpdateState(alertName string, labels map[string]string, state AlertState) error
	GetState(alertName string, labels map[string]string) (AlertState, error)
	GetStateHistory(alertName string, labels map[string]string, duration time.Duration) ([]StateChange, error)
	GetActiveAlerts() ([]ActiveAlert, error)
}

// StateChange represents a state change event
type StateChange struct {
	Timestamp   time.Time
	PreviousState AlertState
	NewState    AlertState
	Value       float64
}

// ActiveAlert represents an currently active alert
type ActiveAlert struct {
	Name        string
	Labels      map[string]string
	State       AlertState
	Value       float64
	ActiveSince time.Time
	Annotations map[string]string
}

// ClickHouseAlertStateTracker implements AlertStateTracker using ClickHouse
type ClickHouseAlertStateTracker struct {
	sqlBuilder builder.SQLBuilder
	tableName  string
}

// NewClickHouseAlertStateTracker creates a new state tracker
func NewClickHouseAlertStateTracker(
	sqlBuilder builder.SQLBuilder,
	tableName string,
) AlertStateTracker {
	return &ClickHouseAlertStateTracker{
		sqlBuilder: sqlBuilder,
		tableName:  tableName,
	}
}

// UpdateState updates the state of an alert
func (t *ClickHouseAlertStateTracker) UpdateState(
	alertName string,
	labels map[string]string,
	state AlertState,
) error {
	builder := t.sqlBuilder
	builder.Reset()
	
	// Build INSERT query for state change
	labelsJSON := formatLabelsAsJSON(labels)
	
	query := fmt.Sprintf(`
		INSERT INTO %s (timestamp, alert_name, labels, state, value)
		VALUES (now(), '%s', '%s', '%s', 0)
	`, t.tableName, alertName, labelsJSON, state)
	
	_ = query
	// Would execute this query
	return nil
}

// GetState gets the current state of an alert
func (t *ClickHouseAlertStateTracker) GetState(
	alertName string,
	labels map[string]string,
) (AlertState, error) {
	builder := t.sqlBuilder
	builder.Reset()
	
	labelsJSON := formatLabelsAsJSON(labels)
	
	query := builder.
		Select("state").
		From(t.tableName).
		Where(fmt.Sprintf("alert_name = '%s'", alertName)).
		And(fmt.Sprintf("labels = '%s'", labelsJSON)).
		OrderByDesc("timestamp").
		Limit(1).
		Build()
	
	_ = query
	// Would execute and return state
	return AlertStateInactive, nil
}

// GetStateHistory gets the history of state changes
func (t *ClickHouseAlertStateTracker) GetStateHistory(
	alertName string,
	labels map[string]string,
	duration time.Duration,
) ([]StateChange, error) {
	builder := t.sqlBuilder
	builder.Reset()
	
	labelsJSON := formatLabelsAsJSON(labels)
	startTime := time.Now().Add(-duration).Unix()
	
	query := builder.
		Select("timestamp", "state", "value").
		SelectExpr("lagInFrame(state, 1) OVER (ORDER BY timestamp)", "previous_state").
		From(t.tableName).
		Where(fmt.Sprintf("alert_name = '%s'", alertName)).
		And(fmt.Sprintf("labels = '%s'", labelsJSON)).
		And(fmt.Sprintf("timestamp >= toDateTime(%d)", startTime)).
		OrderBy("timestamp").
		Build()
	
	_ = query
	// Would execute and parse results
	return []StateChange{}, nil
}

// GetActiveAlerts returns all currently active (firing or pending) alerts
func (t *ClickHouseAlertStateTracker) GetActiveAlerts() ([]ActiveAlert, error) {
	builder := t.sqlBuilder
	builder.Reset()
	
	query := builder.
		With("latest_states", fmt.Sprintf(`
			SELECT 
				alert_name,
				labels,
				state,
				value,
				timestamp,
				ROW_NUMBER() OVER (PARTITION BY alert_name, labels ORDER BY timestamp DESC) as rn
			FROM %s
		`, t.tableName)).
		Select("alert_name", "labels", "state", "value", "timestamp as active_since").
		From("latest_states").
		Where("rn = 1").
		And("state IN ('pending', 'firing')").
		Build()
	
	_ = query
	// Would execute and parse results
	return []ActiveAlert{}, nil
}

// AlertMetricGenerator generates ALERTS and ALERTS_FOR_STATE metrics
// Following Factory Pattern
type AlertMetricGenerator interface {
	GenerateAlertsMetric(activeAlerts []ActiveAlert) (string, error)
	GenerateAlertsForStateMetric(activeAlerts []ActiveAlert, forDuration time.Duration) (string, error)
}

// DefaultAlertMetricGenerator implements AlertMetricGenerator
type DefaultAlertMetricGenerator struct {
	sqlBuilder builder.SQLBuilder
}

// NewAlertMetricGenerator creates a new alert metric generator
func NewAlertMetricGenerator(sqlBuilder builder.SQLBuilder) AlertMetricGenerator {
	return &DefaultAlertMetricGenerator{
		sqlBuilder: sqlBuilder,
	}
}

// GenerateAlertsMetric generates the ALERTS metric query
// ALERTS{alertname="...", alertstate="...", ...} = 1
func (g *DefaultAlertMetricGenerator) GenerateAlertsMetric(activeAlerts []ActiveAlert) (string, error) {
	builder := g.sqlBuilder
	builder.Reset()
	
	query := builder.
		Select("alert_name as alertname").
		Select("state as alertstate").
		Select("labels").
		Select("value").
		Select("timestamp").
		From("alerts").
		Where("state IN ('pending', 'firing')").
		Build()
	
	return query, nil
}

// GenerateAlertsForStateMetric generates ALERTS_FOR_STATE metric
// Returns alerts that have been in current state for at least 'forDuration'
func (g *DefaultAlertMetricGenerator) GenerateAlertsForStateMetric(
	activeAlerts []ActiveAlert,
	forDuration time.Duration,
) (string, error) {
	builder := g.sqlBuilder
	builder.Reset()
	
	minTimestamp := time.Now().Add(-forDuration).Unix()
	
	query := builder.
		Select("alert_name as alertname").
		Select("state as alertstate").
		Select("labels").
		Select("value").
		Select("timestamp as active_since").
		SelectExpr(fmt.Sprintf("now() - timestamp as duration"), "").
		From("alerts").
		Where("state IN ('pending', 'firing')").
		And(fmt.Sprintf("timestamp <= toDateTime(%d)", minTimestamp)).
		Build()
	
	return query, nil
}

// AlertRuleParser parses Prometheus alert rules
// Following Single Responsibility Principle
type AlertRuleParser interface {
	Parse(ruleContent string) ([]AlertRule, error)
	ParseYAML(yamlContent []byte) ([]AlertRule, error)
}

// PrometheusAlertRuleParser implements AlertRuleParser
type PrometheusAlertRuleParser struct{}

// NewAlertRuleParser creates a new alert rule parser
func NewAlertRuleParser() AlertRuleParser {
	return &PrometheusAlertRuleParser{}
}

// Parse parses alert rules from string
func (p *PrometheusAlertRuleParser) Parse(ruleContent string) ([]AlertRule, error) {
	// This would parse Prometheus alert rule format
	// For now, return empty
	return []AlertRule{}, nil
}

// ParseYAML parses alert rules from YAML
func (p *PrometheusAlertRuleParser) ParseYAML(yamlContent []byte) ([]AlertRule, error) {
	// This would parse YAML format
	// For now, return empty
	return []AlertRule{}, nil
}

// AlertExpressionValidator validates alert expressions
// Following Strategy Pattern
type AlertExpressionValidator interface {
	Validate(expr string) error
	ValidateRule(rule *AlertRule) error
}

// DefaultAlertExpressionValidator implements AlertExpressionValidator
type DefaultAlertExpressionValidator struct{}

// NewAlertExpressionValidator creates a new validator
func NewAlertExpressionValidator() AlertExpressionValidator {
	return &DefaultAlertExpressionValidator{}
}

// Validate validates an alert expression
func (v *DefaultAlertExpressionValidator) Validate(expr string) error {
	// Basic validation
	if expr == "" {
		return fmt.Errorf("alert expression cannot be empty")
	}
	
	// Check for common issues
	if !containsComparison(expr) {
		return fmt.Errorf("alert expression should contain a comparison operator")
	}
	
	return nil
}

// ValidateRule validates an entire alert rule
func (v *DefaultAlertExpressionValidator) ValidateRule(rule *AlertRule) error {
	if rule.Name == "" {
		return fmt.Errorf("alert name cannot be empty")
	}
	
	if err := v.Validate(rule.Expr); err != nil {
		return fmt.Errorf("invalid expression: %w", err)
	}
	
	if rule.Duration < 0 {
		return fmt.Errorf("duration cannot be negative")
	}
	
	return nil
}

// Helper functions

func formatLabelsAsJSON(labels map[string]string) string {
	pairs := make([]string, 0, len(labels))
	for k, v := range labels {
		pairs = append(pairs, fmt.Sprintf(`"%s":"%s"`, k, v))
	}
	return fmt.Sprintf("{%s}", strings.Join(pairs, ","))
}

func containsComparison(expr string) bool {
	comparisons := []string{">", "<", ">=", "<=", "==", "!="}
	for _, comp := range comparisons {
		if strings.Contains(expr, comp) {
			return true
		}
	}
	return false
}

// AlertNotificationBuilder builds notification payloads
// Following Builder Pattern
type AlertNotificationBuilder interface {
	BuildWebhookPayload(alert *ActiveAlert) (map[string]interface{}, error)
	BuildEmailPayload(alert *ActiveAlert) (string, error)
	BuildSlackPayload(alert *ActiveAlert) (map[string]interface{}, error)
}

// DefaultAlertNotificationBuilder implements AlertNotificationBuilder
type DefaultAlertNotificationBuilder struct{}

// NewAlertNotificationBuilder creates a new notification builder
func NewAlertNotificationBuilder() AlertNotificationBuilder {
	return &DefaultAlertNotificationBuilder{}
}

// BuildWebhookPayload builds a webhook notification payload
func (b *DefaultAlertNotificationBuilder) BuildWebhookPayload(
	alert *ActiveAlert,
) (map[string]interface{}, error) {
	payload := map[string]interface{}{
		"alertname":   alert.Name,
		"state":       string(alert.State),
		"value":       alert.Value,
		"labels":      alert.Labels,
		"annotations": alert.Annotations,
		"startsAt":    alert.ActiveSince.Format(time.RFC3339),
	}
	
	return payload, nil
}

// BuildEmailPayload builds an email notification
func (b *DefaultAlertNotificationBuilder) BuildEmailPayload(
	alert *ActiveAlert,
) (string, error) {
	subject := fmt.Sprintf("[%s] Alert: %s", alert.State, alert.Name)
	
	body := fmt.Sprintf(`
Alert: %s
State: %s
Value: %f
Active Since: %s

Labels:
%s

Annotations:
%s
	`, alert.Name, alert.State, alert.Value, alert.ActiveSince.Format(time.RFC3339),
		formatLabelsForEmail(alert.Labels),
		formatLabelsForEmail(alert.Annotations))
	
	return subject + "\n\n" + body, nil
}

// BuildSlackPayload builds a Slack notification payload
func (b *DefaultAlertNotificationBuilder) BuildSlackPayload(
	alert *ActiveAlert,
) (map[string]interface{}, error) {
	color := "danger"
	if alert.State == AlertStatePending {
		color = "warning"
	}
	
	payload := map[string]interface{}{
		"attachments": []map[string]interface{}{
			{
				"color":  color,
				"title":  fmt.Sprintf("Alert: %s", alert.Name),
				"text":   fmt.Sprintf("State: %s\nValue: %f", alert.State, alert.Value),
				"fields": buildSlackFields(alert),
			},
		},
	}
	
	return payload, nil
}

func formatLabelsForEmail(labels map[string]string) string {
	lines := make([]string, 0, len(labels))
	for k, v := range labels {
		lines = append(lines, fmt.Sprintf("  %s: %s", k, v))
	}
	return strings.Join(lines, "\n")
}

func buildSlackFields(alert *ActiveAlert) []map[string]interface{} {
	fields := []map[string]interface{}{}
	
	for k, v := range alert.Labels {
		fields = append(fields, map[string]interface{}{
			"title": k,
			"value": v,
			"short": true,
		})
	}
	
	return fields
}
