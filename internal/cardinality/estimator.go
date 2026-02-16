package cardinality

import (
	"fmt"
	"math"
	"sync"
)

// Estimator estimates and manages query cardinality
type Estimator struct {
	// Thresholds for cardinality management
	HighCardinalityThreshold  uint64
	VeryHighCardinalityThreshold uint64
	
	// Sampling ratios
	DefaultSampleRatio float64
	HighCardinalitySampleRatio float64
}

// NewEstimator creates a new cardinality estimator
func NewEstimator() *Estimator {
	return &Estimator{
		HighCardinalityThreshold:     100000,
		VeryHighCardinalityThreshold: 1000000,
		DefaultSampleRatio:           1.0,
		HighCardinalitySampleRatio:   0.1,
	}
}

// EstimateResult represents the cardinality estimation result
type EstimateResult struct {
	EstimatedCardinality uint64
	IsHighCardinality    bool
	SuggestedSampleRatio float64
	SuggestedStrategy    Strategy
	Warnings             []string
}

// Strategy represents query execution strategies
type Strategy int

const (
	StrategyDirect Strategy = iota
	StrategySampling
	StrategyAggregateFirst
	StrategyPruneLabels
)

func (s Strategy) String() string {
	switch s {
	case StrategyDirect:
		return "direct"
	case StrategySampling:
		return "sampling"
	case StrategyAggregateFirst:
		return "aggregate_first"
	case StrategyPruneLabels:
		return "prune_labels"
	default:
		return "unknown"
	}
}

// Estimate estimates the cardinality of a query
func (e *Estimator) Estimate(metricName string, labelCount int, timeRangeSeconds int64) *EstimateResult {
	result := &EstimateResult{
		SuggestedSampleRatio: e.DefaultSampleRatio,
		SuggestedStrategy:    StrategyDirect,
		Warnings:             []string{},
	}
	
	// Base cardinality estimation using heuristics
	// This is a simplified model; in production, you'd query actual cardinality data
	baseCardinality := e.estimateBaseCardinality(metricName, labelCount)
	
	// Factor in time range (more time = more data points)
	timeMultiplier := float64(timeRangeSeconds) / 3600.0 // Normalize to hours
	estimatedPoints := uint64(float64(baseCardinality) * math.Max(1.0, timeMultiplier))
	
	result.EstimatedCardinality = estimatedPoints
	
	// Determine strategy based on cardinality
	if estimatedPoints > e.VeryHighCardinalityThreshold {
		result.IsHighCardinality = true
		result.SuggestedStrategy = StrategySampling
		result.SuggestedSampleRatio = e.HighCardinalitySampleRatio
		result.Warnings = append(result.Warnings, 
			fmt.Sprintf("Very high cardinality detected (%d points). Using sampling strategy.", estimatedPoints))
	} else if estimatedPoints > e.HighCardinalityThreshold {
		result.IsHighCardinality = true
		result.SuggestedStrategy = StrategyAggregateFirst
		result.Warnings = append(result.Warnings, 
			fmt.Sprintf("High cardinality detected (%d points). Consider aggregating first.", estimatedPoints))
	}
	
	// Additional warnings based on label count
	if labelCount > 10 {
		result.Warnings = append(result.Warnings, 
			fmt.Sprintf("High label count (%d). Consider label pruning.", labelCount))
		if result.SuggestedStrategy == StrategyDirect {
			result.SuggestedStrategy = StrategyPruneLabels
		}
	}
	
	return result
}

// estimateBaseCardinality estimates base cardinality based on metric and label count
func (e *Estimator) estimateBaseCardinality(metricName string, labelCount int) uint64 {
	// Simple heuristic: more labels = higher cardinality
	// In production, this would query actual cardinality statistics
	baseCard := uint64(1000)
	
	// Each additional label can multiply cardinality
	labelMultiplier := math.Pow(10, float64(labelCount)/3.0)
	
	return uint64(float64(baseCard) * labelMultiplier)
}

// CardinalityTracker tracks cardinality metrics (thread-safe)
type CardinalityTracker struct {
	mu                sync.RWMutex
	metricCardinality map[string]map[string]uint64 // metric -> label -> cardinality
}

// NewCardinalityTracker creates a new cardinality tracker
func NewCardinalityTracker() *CardinalityTracker {
	return &CardinalityTracker{
		metricCardinality: make(map[string]map[string]uint64),
	}
}

// Track records cardinality information
func (ct *CardinalityTracker) Track(metricName, labelName string, cardinality uint64) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	if _, exists := ct.metricCardinality[metricName]; !exists {
		ct.metricCardinality[metricName] = make(map[string]uint64)
	}
	ct.metricCardinality[metricName][labelName] = cardinality
}

// GetCardinality retrieves cardinality for a metric/label combination
func (ct *CardinalityTracker) GetCardinality(metricName, labelName string) uint64 {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	if labels, exists := ct.metricCardinality[metricName]; exists {
		if card, found := labels[labelName]; found {
			return card
		}
	}
	return 0
}

// GetTotalCardinality calculates total cardinality for a metric
func (ct *CardinalityTracker) GetTotalCardinality(metricName string) uint64 {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	if labels, exists := ct.metricCardinality[metricName]; exists {
		var total uint64
		for _, card := range labels {
			total += card
		}
		return total
	}
	return 0
}

// GetHighCardinalityLabels returns labels with high cardinality
func (ct *CardinalityTracker) GetHighCardinalityLabels(metricName string, threshold uint64) []string {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	var highCardLabels []string
	
	if labels, exists := ct.metricCardinality[metricName]; exists {
		for label, card := range labels {
			if card > threshold {
				highCardLabels = append(highCardLabels, label)
			}
		}
	}
	
	return highCardLabels
}

// OptimizationHint provides hints for query optimization
type OptimizationHint struct {
	Type        HintType
	Description string
	Impact      ImpactLevel
}

// HintType represents types of optimization hints
type HintType int

const (
	HintSampling HintType = iota
	HintAggregation
	HintLabelPruning
	HintTimeReduction
	HintIndexUsage
)

func (ht HintType) String() string {
	switch ht {
	case HintSampling:
		return "sampling"
	case HintAggregation:
		return "aggregation"
	case HintLabelPruning:
		return "label_pruning"
	case HintTimeReduction:
		return "time_reduction"
	case HintIndexUsage:
		return "index_usage"
	default:
		return "unknown"
	}
}

// ImpactLevel represents the impact of an optimization
type ImpactLevel int

const (
	ImpactLow ImpactLevel = iota
	ImpactMedium
	ImpactHigh
)

func (il ImpactLevel) String() string {
	switch il {
	case ImpactLow:
		return "low"
	case ImpactMedium:
		return "medium"
	case ImpactHigh:
		return "high"
	default:
		return "unknown"
	}
}

// Optimizer provides query optimization suggestions
type Optimizer struct {
	estimator *Estimator
	tracker   *CardinalityTracker
}

// NewOptimizer creates a new optimizer
func NewOptimizer() *Optimizer {
	return &Optimizer{
		estimator: NewEstimator(),
		tracker:   NewCardinalityTracker(),
	}
}

// Optimize provides optimization hints for a query
func (o *Optimizer) Optimize(metricName string, labels []string, timeRangeSeconds int64) []OptimizationHint {
	var hints []OptimizationHint
	
	estimate := o.estimator.Estimate(metricName, len(labels), timeRangeSeconds)
	
	if estimate.IsHighCardinality {
		if estimate.SuggestedStrategy == StrategySampling {
			hints = append(hints, OptimizationHint{
				Type:        HintSampling,
				Description: fmt.Sprintf("Use sampling with ratio %.2f to reduce data volume", estimate.SuggestedSampleRatio),
				Impact:      ImpactHigh,
			})
		}
		
		if estimate.SuggestedStrategy == StrategyAggregateFirst {
			hints = append(hints, OptimizationHint{
				Type:        HintAggregation,
				Description: "Aggregate data before further processing to reduce cardinality",
				Impact:      ImpactHigh,
			})
		}
		
		if estimate.SuggestedStrategy == StrategyPruneLabels {
			hints = append(hints, OptimizationHint{
				Type:        HintLabelPruning,
				Description: "Remove unnecessary labels to reduce cardinality",
				Impact:      ImpactMedium,
			})
		}
	}
	
	// Check for long time ranges
	if timeRangeSeconds > 86400 { // More than 1 day
		hints = append(hints, OptimizationHint{
			Type:        HintTimeReduction,
			Description: "Consider reducing time range or using pre-aggregated data",
			Impact:      ImpactMedium,
		})
	}
	
	return hints
}

// CalculateSampleSize calculates an appropriate sample size
func CalculateSampleSize(totalCardinality uint64, desiredPrecision float64) uint64 {
	// Using statistical sampling formula
	// n = (Z^2 * p * (1-p)) / E^2
	// Where Z = 1.96 for 95% confidence, p = 0.5 for maximum variance, E = desired error
	
	z := 1.96
	p := 0.5
	e := desiredPrecision
	
	sampleSize := (z * z * p * (1 - p)) / (e * e)
	
	// Ensure we don't sample more than available
	if uint64(sampleSize) > totalCardinality {
		return totalCardinality
	}
	
	// Minimum sample size
	if sampleSize < 100 {
		return 100
	}
	
	return uint64(sampleSize)
}

// EstimateQueryCost estimates the computational cost of a query
func EstimateQueryCost(cardinality uint64, operations int, complexity float64) float64 {
	// Simple cost model: cost = cardinality * operations * complexity
	return float64(cardinality) * float64(operations) * complexity
}
