//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUserSubscription_CheckDailyLimitRequests(t *testing.T) {
	now := time.Now()
	windowStart := now.Add(-12 * time.Hour) // 12 hours ago

	tests := []struct {
		name               string
		usageRequests      int64
		limit              *int64
		additionalRequests int64
		expectError        bool
		errorWindow        string
	}{
		{
			name:               "no limit configured - always passes",
			usageRequests:      100,
			limit:              nil,
			additionalRequests: 1,
			expectError:        false,
		},
		{
			name:               "under limit - passes",
			usageRequests:      50,
			limit:              int64Ptr(100),
			additionalRequests: 1,
			expectError:        false,
		},
		{
			name:               "at limit - passes (check is usage + additional > limit)",
			usageRequests:      99,
			limit:              int64Ptr(100),
			additionalRequests: 1,
			expectError:        false,
		},
		{
			name:               "over limit - fails",
			usageRequests:      100,
			limit:              int64Ptr(100),
			additionalRequests: 1,
			expectError:        true,
			errorWindow:        "daily",
		},
		{
			name:               "way over limit - fails",
			usageRequests:      150,
			limit:              int64Ptr(100),
			additionalRequests: 1,
			expectError:        true,
			errorWindow:        "daily",
		},
		{
			name:               "zero additional requests - passes if under limit",
			usageRequests:      100,
			limit:              int64Ptr(100),
			additionalRequests: 0,
			expectError:        false,
		},
		{
			name:               "zero limit means no limit - passes",
			usageRequests:      0,
			limit:              int64Ptr(0),
			additionalRequests: 1,
			expectError:        false, // limit=0 means "no limit configured", not "block all"
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sub := &UserSubscription{
				DailyUsageRequests: tt.usageRequests,
				DailyWindowStart:   &windowStart,
			}
			group := &Group{
				DailyLimitRequests: tt.limit,
			}

			err := sub.CheckDailyLimitRequests(group, tt.additionalRequests)

			if tt.expectError {
				require.Error(t, err)
				limitErr, ok := err.(*RequestLimitExceededError)
				require.True(t, ok, "error should be RequestLimitExceededError")
				require.Equal(t, tt.errorWindow, limitErr.Window)
				require.Equal(t, *tt.limit, limitErr.Limit)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestUserSubscription_CheckWeeklyLimitRequests(t *testing.T) {
	now := time.Now()
	windowStart := now.Add(-3 * 24 * time.Hour) // 3 days ago

	tests := []struct {
		name               string
		usageRequests      int64
		limit              *int64
		additionalRequests int64
		expectError        bool
	}{
		{
			name:               "no limit configured - always passes",
			usageRequests:      500,
			limit:              nil,
			additionalRequests: 1,
			expectError:        false,
		},
		{
			name:               "under limit - passes",
			usageRequests:      400,
			limit:              int64Ptr(500),
			additionalRequests: 1,
			expectError:        false,
		},
		{
			name:               "over limit - fails",
			usageRequests:      500,
			limit:              int64Ptr(500),
			additionalRequests: 1,
			expectError:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sub := &UserSubscription{
				WeeklyUsageRequests: tt.usageRequests,
				WeeklyWindowStart:   &windowStart,
			}
			group := &Group{
				WeeklyLimitRequests: tt.limit,
			}

			err := sub.CheckWeeklyLimitRequests(group, tt.additionalRequests)

			if tt.expectError {
				require.Error(t, err)
				limitErr, ok := err.(*RequestLimitExceededError)
				require.True(t, ok)
				require.Equal(t, "weekly", limitErr.Window)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestUserSubscription_CheckMonthlyLimitRequests(t *testing.T) {
	now := time.Now()
	windowStart := now.Add(-15 * 24 * time.Hour) // 15 days ago

	tests := []struct {
		name               string
		usageRequests      int64
		limit              *int64
		additionalRequests int64
		expectError        bool
	}{
		{
			name:               "no limit configured - always passes",
			usageRequests:      1000,
			limit:              nil,
			additionalRequests: 1,
			expectError:        false,
		},
		{
			name:               "under limit - passes",
			usageRequests:      900,
			limit:              int64Ptr(1000),
			additionalRequests: 1,
			expectError:        false,
		},
		{
			name:               "over limit - fails",
			usageRequests:      1000,
			limit:              int64Ptr(1000),
			additionalRequests: 1,
			expectError:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sub := &UserSubscription{
				MonthlyUsageRequests: tt.usageRequests,
				MonthlyWindowStart:   &windowStart,
			}
			group := &Group{
				MonthlyLimitRequests: tt.limit,
			}

			err := sub.CheckMonthlyLimitRequests(group, tt.additionalRequests)

			if tt.expectError {
				require.Error(t, err)
				limitErr, ok := err.(*RequestLimitExceededError)
				require.True(t, ok)
				require.Equal(t, "monthly", limitErr.Window)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestRequestLimitExceededError_Code(t *testing.T) {
	tests := []struct {
		window      string
		expectedCode string
	}{
		{"daily", "DAILY_REQUEST_LIMIT_EXCEEDED"},
		{"weekly", "WEEKLY_REQUEST_LIMIT_EXCEEDED"},
		{"monthly", "MONTHLY_REQUEST_LIMIT_EXCEEDED"},
		{"unknown", "REQUEST_LIMIT_EXCEEDED"},
	}

	for _, tt := range tests {
		t.Run(tt.window, func(t *testing.T) {
			err := &RequestLimitExceededError{
				Window: tt.window,
				Used:   100,
				Limit:  50,
			}
			require.Equal(t, tt.expectedCode, err.Code())
		})
	}
}

func TestRequestLimitExceededError_Error(t *testing.T) {
	err := &RequestLimitExceededError{
		Window: "daily",
		Used:   101,
		Limit:  100,
	}
	require.Contains(t, err.Error(), "daily request limit exceeded")
	require.Contains(t, err.Error(), "101/100")
}

func TestUserSubscription_GetDailyResetInSeconds(t *testing.T) {
	t.Run("nil window start returns 0", func(t *testing.T) {
		sub := &UserSubscription{DailyWindowStart: nil}
		require.Equal(t, int64(0), sub.GetDailyResetInSeconds())
	})

	t.Run("window in future returns positive seconds", func(t *testing.T) {
		now := time.Now()
		futureStart := now.Add(-12 * time.Hour) // started 12 hours ago
		sub := &UserSubscription{DailyWindowStart: &futureStart}

		resetSeconds := sub.GetDailyResetInSeconds()
		// Should be around 12 hours = 43200 seconds
		require.Greater(t, resetSeconds, int64(40000))
		require.Less(t, resetSeconds, int64(44000))
	})

	t.Run("expired window returns 0", func(t *testing.T) {
		now := time.Now()
		pastStart := now.Add(-25 * time.Hour) // started 25 hours ago (expired)
		sub := &UserSubscription{DailyWindowStart: &pastStart}
		require.Equal(t, int64(0), sub.GetDailyResetInSeconds())
	})
}

func TestUserSubscription_GetWeeklyResetInSeconds(t *testing.T) {
	t.Run("nil window start returns 0", func(t *testing.T) {
		sub := &UserSubscription{WeeklyWindowStart: nil}
		require.Equal(t, int64(0), sub.GetWeeklyResetInSeconds())
	})

	t.Run("window in future returns positive seconds", func(t *testing.T) {
		now := time.Now()
		futureStart := now.Add(-3 * 24 * time.Hour) // started 3 days ago
		sub := &UserSubscription{WeeklyWindowStart: &futureStart}

		resetSeconds := sub.GetWeeklyResetInSeconds()
		// Should be around 4 days = 345600 seconds
		require.Greater(t, resetSeconds, int64(340000))
		require.Less(t, resetSeconds, int64(350000))
	})
}

func TestUserSubscription_GetMonthlyResetInSeconds(t *testing.T) {
	t.Run("nil window start returns 0", func(t *testing.T) {
		sub := &UserSubscription{MonthlyWindowStart: nil}
		require.Equal(t, int64(0), sub.GetMonthlyResetInSeconds())
	})

	t.Run("window in future returns positive seconds", func(t *testing.T) {
		now := time.Now()
		futureStart := now.Add(-15 * 24 * time.Hour) // started 15 days ago
		sub := &UserSubscription{MonthlyWindowStart: &futureStart}

		resetSeconds := sub.GetMonthlyResetInSeconds()
		// Should be around 15 days = 1296000 seconds
		require.Greater(t, resetSeconds, int64(1200000))
		require.Less(t, resetSeconds, int64(1400000))
	})
}

// ============================================
// 边界条件测试：负值、零值
// ============================================

func TestUserSubscription_CheckDailyLimitRequests_NegativeValues(t *testing.T) {
	now := time.Now()
	windowStart := now.Add(-12 * time.Hour)

	tests := []struct {
		name               string
		usageRequests      int64
		limit              *int64
		additionalRequests int64
		expectError        bool
		description        string
	}{
		{
			name:               "negative limit treated as no limit",
			usageRequests:      100,
			limit:              int64Ptr(-1),
			additionalRequests: 1,
			expectError:        false,
			description:        "负限制值应该被视为无限制",
		},
		{
			name:               "negative usage still checked against positive limit",
			usageRequests:      -10,
			limit:              int64Ptr(100),
			additionalRequests: 1,
			expectError:        false,
			description:        "负用量值仍应对正限制进行检查",
		},
		{
			name:               "negative additional requests passes",
			usageRequests:      50,
			limit:              int64Ptr(100),
			additionalRequests: -5,
			expectError:        false,
			description:        "负增量请求应该通过检查",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sub := &UserSubscription{
				DailyUsageRequests: tt.usageRequests,
				DailyWindowStart:   &windowStart,
			}
			group := &Group{
				DailyLimitRequests: tt.limit,
			}

			err := sub.CheckDailyLimitRequests(group, tt.additionalRequests)

			if tt.expectError {
				require.Error(t, err, tt.description)
			} else {
				require.NoError(t, err, tt.description)
			}
		})
	}
}

func TestUserSubscription_CheckWeeklyLimitRequests_NegativeLimit(t *testing.T) {
	now := time.Now()
	windowStart := now.Add(-3 * 24 * time.Hour)

	sub := &UserSubscription{
		WeeklyUsageRequests: 500,
		WeeklyWindowStart:   &windowStart,
	}
	group := &Group{
		WeeklyLimitRequests: int64Ptr(-100),
	}

	err := sub.CheckWeeklyLimitRequests(group, 1)
	require.NoError(t, err, "负周限制值应该被视为无限制")
}

func TestUserSubscription_CheckMonthlyLimitRequests_NegativeLimit(t *testing.T) {
	now := time.Now()
	windowStart := now.Add(-15 * 24 * time.Hour)

	sub := &UserSubscription{
		MonthlyUsageRequests: 1000,
		MonthlyWindowStart:   &windowStart,
	}
	group := &Group{
		MonthlyLimitRequests: int64Ptr(-100),
	}

	err := sub.CheckMonthlyLimitRequests(group, 1)
	require.NoError(t, err, "负月限制值应该被视为无限制")
}

func TestUserSubscription_ZeroLimits(t *testing.T) {
	now := time.Now()
	windowStart := now.Add(-12 * time.Hour)

	sub := &UserSubscription{
		DailyUsageRequests:   100,
		DailyWindowStart:     &windowStart,
		WeeklyUsageRequests:  500,
		WeeklyWindowStart:    &windowStart,
		MonthlyUsageRequests: 1000,
		MonthlyWindowStart:   &windowStart,
	}
	group := &Group{
		DailyLimitRequests:   int64Ptr(0),
		WeeklyLimitRequests:  int64Ptr(0),
		MonthlyLimitRequests: int64Ptr(0),
	}

	// 零限制值意味着无限制（不是阻止所有请求）
	require.NoError(t, sub.CheckDailyLimitRequests(group, 1), "零日限制应该通过")
	require.NoError(t, sub.CheckWeeklyLimitRequests(group, 1), "零周限制应该通过")
	require.NoError(t, sub.CheckMonthlyLimitRequests(group, 1), "零月限制应该通过")
}

func TestUserSubscription_NilLimits(t *testing.T) {
	now := time.Now()
	windowStart := now.Add(-12 * time.Hour)

	sub := &UserSubscription{
		DailyUsageRequests:   100,
		DailyWindowStart:     &windowStart,
		WeeklyUsageRequests:  500,
		WeeklyWindowStart:    &windowStart,
		MonthlyUsageRequests: 1000,
		MonthlyWindowStart:   &windowStart,
	}
	group := &Group{
		DailyLimitRequests:   nil,
		WeeklyLimitRequests:  nil,
		MonthlyLimitRequests: nil,
	}

	// nil 限制值意味着无限制
	require.NoError(t, sub.CheckDailyLimitRequests(group, 1), "nil日限制应该通过")
	require.NoError(t, sub.CheckWeeklyLimitRequests(group, 1), "nil周限制应该通过")
	require.NoError(t, sub.CheckMonthlyLimitRequests(group, 1), "nil月限制应该通过")
}