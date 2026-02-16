package strategy

import (
	"fmt"
	"sort"
	"sync"

	"github.com/shinro/promql-transpiler/pkg/ast"
)

// DefaultStrategyRegistry implements StrategyRegistry using Chain of Responsibility
// Following Single Responsibility Principle - only manages strategy registration and selection
type DefaultStrategyRegistry struct {
	strategies []TranspilationStrategy
	mu         sync.RWMutex
}

// NewStrategyRegistry creates a new strategy registry
func NewStrategyRegistry() StrategyRegistry {
	return &DefaultStrategyRegistry{
		strategies: make([]TranspilationStrategy, 0),
	}
}

// Register adds a strategy to the registry
// Following Open/Closed Principle - open for extension
func (r *DefaultStrategyRegistry) Register(strategy TranspilationStrategy) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	r.strategies = append(r.strategies, strategy)
	
	// Sort by priority (highest first)
	sort.Slice(r.strategies, func(i, j int) bool {
		return r.strategies[i].GetPriority() > r.strategies[j].GetPriority()
	})
}

// GetStrategy finds the best strategy for an expression using Chain of Responsibility
func (r *DefaultStrategyRegistry) GetStrategy(expr ast.Expr) (TranspilationStrategy, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	// Chain of Responsibility: try each strategy in priority order
	for _, strategy := range r.strategies {
		if strategy.CanHandle(expr) {
			return strategy, nil
		}
	}
	
	return nil, fmt.Errorf("no strategy found for expression type: %T", expr)
}

// GetStrategiesByType returns strategies of a specific type.
// Only strategies that implement TypedStrategy and match the given type
// are returned. Strategies without type information are excluded.
func (r *DefaultStrategyRegistry) GetStrategiesByType(strategyType StrategyType) []TranspilationStrategy {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	var result []TranspilationStrategy
	for _, strategy := range r.strategies {
		if typed, ok := strategy.(TypedStrategy); ok && typed.GetType() == strategyType {
			result = append(result, strategy)
		}
	}
	
	return result
}

// BaseStrategy provides common functionality for all strategies
// Following Template Method Pattern
type BaseStrategy struct {
	priority int
	context  *Context
}

// NewBaseStrategy creates a base strategy
func NewBaseStrategy(priority int, context *Context) *BaseStrategy {
	return &BaseStrategy{
		priority: priority,
		context:  context,
	}
}

// GetPriority returns the strategy priority
func (b *BaseStrategy) GetPriority() int {
	return b.priority
}

// GetContext returns the transpilation context
func (b *BaseStrategy) GetContext() *Context {
	return b.context
}

// SetContext updates the transpilation context
func (b *BaseStrategy) SetContext(ctx *Context) {
	b.context = ctx
}
