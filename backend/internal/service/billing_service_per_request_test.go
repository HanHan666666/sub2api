//go:build unit

package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestCalculatePerRequestCost_BasicComputation(t *testing.T) {
	svc := NewBillingService(&config.Config{}, nil)

	tests := []struct {
		name           string
		price          float64
		rateMultiplier float64
		expectedTotal  float64
		expectedActual float64
	}{
		{
			name:           "basic calculation with 1x multiplier",
			price:          0.05,
			rateMultiplier: 1.0,
			expectedTotal:  0.05,
			expectedActual: 0.05,
		},
		{
			name:           "2x rate multiplier",
			price:          0.05,
			rateMultiplier: 2.0,
			expectedTotal:  0.05,
			expectedActual: 0.10,
		},
		{
			name:           "0.8x rate multiplier (discount)",
			price:          0.05,
			rateMultiplier: 0.8,
			expectedTotal:  0.05,
			expectedActual: 0.04,
		},
		{
			name:           "zero price",
			price:          0.0,
			rateMultiplier: 1.0,
			expectedTotal:  0.0,
			expectedActual: 0.0,
		},
		{
			name:           "high precision price",
			price:          0.0123456789,
			rateMultiplier: 1.0,
			expectedTotal:  0.0123456789,
			expectedActual: 0.0123456789,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cost := svc.CalculatePerRequestCost(tt.price, tt.rateMultiplier)

			require.InDelta(t, tt.expectedTotal, cost.TotalCost, 1e-10)
			require.InDelta(t, tt.expectedActual, cost.ActualCost, 1e-10)

			// Verify token costs are zero for per-request billing
			require.Equal(t, 0.0, cost.InputCost)
			require.Equal(t, 0.0, cost.OutputCost)
			require.Equal(t, 0.0, cost.CacheCreationCost)
			require.Equal(t, 0.0, cost.CacheReadCost)
		})
	}
}

func TestCalculatePerRequestCost_ZeroMultiplierDefaultsToOne(t *testing.T) {
	svc := NewBillingService(&config.Config{}, nil)

	costZero := svc.CalculatePerRequestCost(0.05, 0)
	costOne := svc.CalculatePerRequestCost(0.05, 1.0)

	require.InDelta(t, costOne.ActualCost, costZero.ActualCost, 1e-10)
}

func TestCalculatePerRequestCost_NegativeMultiplierDefaultsToOne(t *testing.T) {
	svc := NewBillingService(&config.Config{}, nil)

	costNeg := svc.CalculatePerRequestCost(0.05, -1.0)
	costOne := svc.CalculatePerRequestCost(0.05, 1.0)

	require.InDelta(t, costOne.ActualCost, costNeg.ActualCost, 1e-10)
}

func TestCalculatePerRequestCost_ZeroPrice(t *testing.T) {
	svc := NewBillingService(&config.Config{}, nil)

	cost := svc.CalculatePerRequestCost(0.0, 1.0)

	require.Equal(t, 0.0, cost.TotalCost)
	require.Equal(t, 0.0, cost.ActualCost)
}

func TestCalculatePerRequestCost_CostBreakdownFields(t *testing.T) {
	svc := NewBillingService(&config.Config{}, nil)

	cost := svc.CalculatePerRequestCost(0.05, 1.5)

	// Verify the cost breakdown structure
	require.Equal(t, 0.0, cost.InputCost, "input cost should be 0 for per-request billing")
	require.Equal(t, 0.0, cost.OutputCost, "output cost should be 0 for per-request billing")
	require.Equal(t, 0.0, cost.CacheCreationCost, "cache creation cost should be 0 for per-request billing")
	require.Equal(t, 0.0, cost.CacheReadCost, "cache read cost should be 0 for per-request billing")
	require.Equal(t, 0.05, cost.TotalCost, "total cost should equal per request price")
	require.InDelta(t, 0.075, cost.ActualCost, 1e-10, "actual cost should be total * rate multiplier")
}

func TestCalculatePerRequestCost_LargePrice(t *testing.T) {
	svc := NewBillingService(&config.Config{}, nil)

	// Test with a large price value
	cost := svc.CalculatePerRequestCost(100.0, 1.0)

	require.Equal(t, 100.0, cost.TotalCost)
	require.Equal(t, 100.0, cost.ActualCost)
}

func TestCalculatePerRequestCost_SmallPrice(t *testing.T) {
	svc := NewBillingService(&config.Config{}, nil)

	// Test with a very small price value
	cost := svc.CalculatePerRequestCost(0.0001, 1.0)

	require.InDelta(t, 0.0001, cost.TotalCost, 1e-15)
	require.InDelta(t, 0.0001, cost.ActualCost, 1e-15)
}