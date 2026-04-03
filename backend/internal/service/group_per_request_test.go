//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Helper functions for this test file
func float64PtrTest(v float64) *float64 { return &v }

func TestGroup_IsPerRequestType(t *testing.T) {
	tests := []struct {
		name             string
		subscriptionType string
		expected         bool
	}{
		{
			name:             "per_request type returns true",
			subscriptionType: SubscriptionTypePerRequest,
			expected:         true,
		},
		{
			name:             "subscription type returns false",
			subscriptionType: SubscriptionTypeSubscription,
			expected:         false,
		},
		{
			name:             "standard type returns false",
			subscriptionType: SubscriptionTypeStandard,
			expected:         false,
		},
		{
			name:             "empty type returns false",
			subscriptionType: "",
			expected:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			group := &Group{SubscriptionType: tt.subscriptionType}
			require.Equal(t, tt.expected, group.IsPerRequestType())
		})
	}
}

func TestGroup_HasPerRequestPrice(t *testing.T) {
	tests := []struct {
		name     string
		price    *float64
		expected bool
	}{
		{
			name:     "positive price returns true",
			price:    float64PtrTest(0.05),
			expected: true,
		},
		{
			name:     "zero price returns false",
			price:    float64PtrTest(0.0),
			expected: false,
		},
		{
			name:     "negative price returns false",
			price:    float64PtrTest(-0.01),
			expected: false,
		},
		{
			name:     "nil price returns false",
			price:    nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			group := &Group{PerRequestPrice: tt.price}
			require.Equal(t, tt.expected, group.HasPerRequestPrice())
		})
	}
}

func TestGroup_HasDailyLimitRequests(t *testing.T) {
	tests := []struct {
		name     string
		limit    *int64
		expected bool
	}{
		{
			name:     "positive limit returns true",
			limit:    int64Ptr(100),
			expected: true,
		},
		{
			name:     "zero limit returns false",
			limit:    int64Ptr(0),
			expected: false,
		},
		{
			name:     "negative limit returns false",
			limit:    int64Ptr(-10),
			expected: false,
		},
		{
			name:     "nil limit returns false",
			limit:    nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			group := &Group{DailyLimitRequests: tt.limit}
			require.Equal(t, tt.expected, group.HasDailyLimitRequests())
		})
	}
}

func TestGroup_HasWeeklyLimitRequests(t *testing.T) {
	tests := []struct {
		name     string
		limit    *int64
		expected bool
	}{
		{
			name:     "positive limit returns true",
			limit:    int64Ptr(500),
			expected: true,
		},
		{
			name:     "zero limit returns false",
			limit:    int64Ptr(0),
			expected: false,
		},
		{
			name:     "nil limit returns false",
			limit:    nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			group := &Group{WeeklyLimitRequests: tt.limit}
			require.Equal(t, tt.expected, group.HasWeeklyLimitRequests())
		})
	}
}

func TestGroup_HasMonthlyLimitRequests(t *testing.T) {
	tests := []struct {
		name     string
		limit    *int64
		expected bool
	}{
		{
			name:     "positive limit returns true",
			limit:    int64Ptr(1000),
			expected: true,
		},
		{
			name:     "zero limit returns false",
			limit:    int64Ptr(0),
			expected: false,
		},
		{
			name:     "nil limit returns false",
			limit:    nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			group := &Group{MonthlyLimitRequests: tt.limit}
			require.Equal(t, tt.expected, group.HasMonthlyLimitRequests())
		})
	}
}

func TestGroup_HasAnyRequestLimit(t *testing.T) {
	tests := []struct {
		name                string
		dailyLimitRequests  *int64
		weeklyLimitRequests *int64
		monthlyLimitRequests *int64
		expected            bool
	}{
		{
			name:                "no limits returns false",
			dailyLimitRequests:  nil,
			weeklyLimitRequests: nil,
			monthlyLimitRequests: nil,
			expected:            false,
		},
		{
			name:                "only daily limit returns true",
			dailyLimitRequests:  int64Ptr(100),
			weeklyLimitRequests: nil,
			monthlyLimitRequests: nil,
			expected:            true,
		},
		{
			name:                "only weekly limit returns true",
			dailyLimitRequests:  nil,
			weeklyLimitRequests: int64Ptr(500),
			monthlyLimitRequests: nil,
			expected:            true,
		},
		{
			name:                "only monthly limit returns true",
			dailyLimitRequests:  nil,
			weeklyLimitRequests: nil,
			monthlyLimitRequests: int64Ptr(1000),
			expected:            true,
		},
		{
			name:                "multiple limits returns true",
			dailyLimitRequests:  int64Ptr(100),
			weeklyLimitRequests: int64Ptr(500),
			monthlyLimitRequests: int64Ptr(1000),
			expected:            true,
		},
		{
			name:                "zero limits returns false",
			dailyLimitRequests:  int64Ptr(0),
			weeklyLimitRequests: int64Ptr(0),
			monthlyLimitRequests: int64Ptr(0),
			expected:            false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			group := &Group{
				DailyLimitRequests:   tt.dailyLimitRequests,
				WeeklyLimitRequests:  tt.weeklyLimitRequests,
				MonthlyLimitRequests: tt.monthlyLimitRequests,
			}
			require.Equal(t, tt.expected, group.HasAnyRequestLimit())
		})
	}
}

// Note: floatPtr and int64Ptr helpers are defined in other test files in this package