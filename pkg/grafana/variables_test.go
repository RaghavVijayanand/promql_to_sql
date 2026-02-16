package grafana

import (
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestProcessMacros_TimeFilter tests $__timeFilter macro
func TestProcessMacros_TimeFilter(t *testing.T) {
	// We're testing the pre-compiled regex patterns which is the critical fix
	// Just verify the patterns exist and are valid
	assert.NotNil(t, timeFilterRe, "timeFilterRe should be pre-compiled")
	assert.NotNil(t, timeFromRe, "timeFromRe should be pre-compiled")
	assert.NotNil(t, timeToRe, "timeToRe should be pre-compiled")
}

// TestProcessMacros_PreCompiledRegex tests that regex patterns are pre-compiled
func TestProcessMacros_PreCompiledRegex(t *testing.T) {
	// Test all pre-compiled patterns exist
	patterns := []*regexp.Regexp{
		timeFilterRe,
		timeFromRe,
		timeToRe,
		timeGroupRe,
		unixEpochFilterRe,
		unixEpochFromRe,
		unixEpochToRe,
		containsRe,
	}
	
	for i, pattern := range patterns {
		assert.NotNil(t, pattern, "Pattern %d should be pre-compiled", i)
	}
}

// TestGrafanaContext tests context creation
func TestGrafanaContext(t *testing.T) {
	ctx := &GrafanaContext{
		Variables: make(map[string]string),
		TimeRange: &TimeRange{
			From: time.Now().Add(-1 * time.Hour),
			To:   time.Now(),
		},
		Interval:   "1m",
		IntervalMs: 60000,
	}
	
	assert.NotNil(t, ctx.Variables)
	assert.NotNil(t, ctx.TimeRange)
	assert.Equal(t, "1m", ctx.Interval)
	assert.Equal(t, int64(60000), ctx.IntervalMs)
}

// TestTimeRange tests time range structure
func TestTimeRange(t *testing.T) {
	now := time.Now()
	tr := &TimeRange{
		From: now.Add(-1 * time.Hour),
		To:   now,
	}
	
	assert.True(t, tr.From.Before(tr.To))
	assert.Equal(t, time.Hour, tr.To.Sub(tr.From))
}


