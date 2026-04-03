//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestValidateAndCheckLimits_PerRequestMode(t *testing.T) {
	now := time.Now()
	activeWindowStart := now.Add(-12 * time.Hour)
	expiredWindowStart := now.Add(-25 * time.Hour)

	tests := []struct {
		name                    string
		sub                     *UserSubscription
		group                   *Group
		expectError             bool
		errorType               error
		expectNeedsMaintenance  bool
	}{
		{
			name: "per_request with no limits - passes",
			sub: &UserSubscription{
				Status:              SubscriptionStatusActive,
				ExpiresAt:           now.Add(24 * time.Hour),
				DailyWindowStart:    &activeWindowStart,
				DailyUsageRequests:  50,
				WeeklyUsageRequests: 200,
				MonthlyUsageRequests: 800,
			},
			group: &Group{
				SubscriptionType:    SubscriptionTypePerRequest,
				PerRequestPrice:     float64PtrTest(0.05),
				DailyLimitRequests:  nil, // no limits
				WeeklyLimitRequests: nil,
				MonthlyLimitRequests: nil,
			},
			expectError:            false,
			expectNeedsMaintenance: false,
		},
		{
			name: "per_request under all limits - passes",
			sub: &UserSubscription{
				Status:               SubscriptionStatusActive,
				ExpiresAt:            now.Add(24 * time.Hour),
				DailyWindowStart:     &activeWindowStart,
				WeeklyWindowStart:    &activeWindowStart,
				MonthlyWindowStart:   &activeWindowStart,
				DailyUsageRequests:   50,
				WeeklyUsageRequests:  200,
				MonthlyUsageRequests: 800,
			},
			group: &Group{
				SubscriptionType:     SubscriptionTypePerRequest,
				PerRequestPrice:      float64PtrTest(0.05),
				DailyLimitRequests:   int64Ptr(100),
				WeeklyLimitRequests:  int64Ptr(500),
				MonthlyLimitRequests: int64Ptr(1000),
			},
			expectError:            false,
			expectNeedsMaintenance: false,
		},
		{
			name: "per_request daily limit exceeded - fails",
			sub: &UserSubscription{
				Status:              SubscriptionStatusActive,
				ExpiresAt:           now.Add(24 * time.Hour),
				DailyWindowStart:    &activeWindowStart,
				DailyUsageRequests:  100,
				WeeklyUsageRequests: 200,
				MonthlyUsageRequests: 800,
			},
			group: &Group{
				SubscriptionType:     SubscriptionTypePerRequest,
				PerRequestPrice:      float64PtrTest(0.05),
				DailyLimitRequests:   int64Ptr(100),
				WeeklyLimitRequests:  int64Ptr(500),
				MonthlyLimitRequests: int64Ptr(1000),
			},
			expectError: true,
			errorType:   nil, // Will be RequestLimitExceededError
		},
		{
			name: "per_request weekly limit exceeded - fails",
			sub: &UserSubscription{
				Status:               SubscriptionStatusActive,
				ExpiresAt:            now.Add(24 * time.Hour),
				DailyWindowStart:     &activeWindowStart,
				WeeklyWindowStart:    &activeWindowStart,
				DailyUsageRequests:   50,
				WeeklyUsageRequests:  500,
				MonthlyUsageRequests: 800,
			},
			group: &Group{
				SubscriptionType:     SubscriptionTypePerRequest,
				PerRequestPrice:      float64PtrTest(0.05),
				DailyLimitRequests:   int64Ptr(100),
				WeeklyLimitRequests:  int64Ptr(500),
				MonthlyLimitRequests: int64Ptr(1000),
			},
			expectError: true,
			errorType:   nil, // Will be RequestLimitExceededError
		},
		{
			name: "per_request monthly limit exceeded - fails",
			sub: &UserSubscription{
				Status:               SubscriptionStatusActive,
				ExpiresAt:            now.Add(24 * time.Hour),
				DailyWindowStart:     &activeWindowStart,
				WeeklyWindowStart:    &activeWindowStart,
				MonthlyWindowStart:   &activeWindowStart,
				DailyUsageRequests:   50,
				WeeklyUsageRequests:  200,
				MonthlyUsageRequests: 1000,
			},
			group: &Group{
				SubscriptionType:     SubscriptionTypePerRequest,
				PerRequestPrice:      float64PtrTest(0.05),
				DailyLimitRequests:   int64Ptr(100),
				WeeklyLimitRequests:  int64Ptr(500),
				MonthlyLimitRequests: int64Ptr(1000),
			},
			expectError: true,
			errorType:   nil, // Will be RequestLimitExceededError
		},
		{
			name: "per_request expired window resets usage - passes",
			sub: &UserSubscription{
				Status:               SubscriptionStatusActive,
				ExpiresAt:            now.Add(24 * time.Hour),
				DailyWindowStart:     &expiredWindowStart, // expired 1 hour ago
				DailyUsageRequests:   100, // would be over limit if not reset
				WeeklyWindowStart:    &activeWindowStart,
				MonthlyWindowStart:   &activeWindowStart,
			},
			group: &Group{
				SubscriptionType:     SubscriptionTypePerRequest,
				PerRequestPrice:      float64PtrTest(0.05),
				DailyLimitRequests:   int64Ptr(100),
				WeeklyLimitRequests:  nil,
				MonthlyLimitRequests: nil,
			},
			expectError:            false,
			expectNeedsMaintenance: true, // needs window reset
		},
		{
			name: "per_request no window activated - needs maintenance",
			sub: &UserSubscription{
				Status:               SubscriptionStatusActive,
				ExpiresAt:            now.Add(24 * time.Hour),
				DailyWindowStart:     nil, // not activated
				WeeklyWindowStart:    nil,
				MonthlyWindowStart:   nil,
			},
			group: &Group{
				SubscriptionType:     SubscriptionTypePerRequest,
				PerRequestPrice:      float64PtrTest(0.05),
				DailyLimitRequests:   int64Ptr(100),
			},
			expectError:            false,
			expectNeedsMaintenance: true,
		},
		{
			name: "subscription mode uses USD limits - not affected by request limits",
			sub: &UserSubscription{
				Status:             SubscriptionStatusActive,
				ExpiresAt:          now.Add(24 * time.Hour),
				DailyWindowStart:   &activeWindowStart,
				WeeklyWindowStart:  &activeWindowStart,
				MonthlyWindowStart: &activeWindowStart,
				DailyUsageUSD:      5.0,
				DailyUsageRequests: 100, // would exceed if per_request
			},
			group: &Group{
				SubscriptionType:     SubscriptionTypeSubscription,
				DailyLimitUSD:        float64PtrTest(10.0),
				DailyLimitRequests:   int64Ptr(50), // should be ignored
			},
			expectError:            false,
			expectNeedsMaintenance: false,
		},
		{
			name: "expired subscription - fails",
			sub: &UserSubscription{
				Status:               SubscriptionStatusActive,
				ExpiresAt:            now.Add(-1 * time.Hour), // expired
				DailyWindowStart:     &activeWindowStart,
			},
			group: &Group{
				SubscriptionType:     SubscriptionTypePerRequest,
				PerRequestPrice:      float64PtrTest(0.05),
			},
			expectError: true,
			errorType:   ErrSubscriptionExpired,
		},
		{
			name: "suspended subscription - fails",
			sub: &UserSubscription{
				Status:               SubscriptionStatusSuspended,
				ExpiresAt:            now.Add(24 * time.Hour),
				DailyWindowStart:     &activeWindowStart,
			},
			group: &Group{
				SubscriptionType:     SubscriptionTypePerRequest,
				PerRequestPrice:      float64PtrTest(0.05),
			},
			expectError: true,
			errorType:   ErrSubscriptionSuspended,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &SubscriptionService{}
			needsMaintenance, err := svc.ValidateAndCheckLimits(tt.sub, tt.group)

			require.Equal(t, tt.expectNeedsMaintenance, needsMaintenance, "needsMaintenance mismatch")

			if tt.expectError {
				require.Error(t, err)
				if tt.errorType != nil {
					require.Equal(t, tt.errorType, err)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateAndCheckLimits_PerRequestResetsExpiredWindows(t *testing.T) {
	now := time.Now()
	expiredDailyStart := now.Add(-25 * time.Hour)
	expiredWeeklyStart := now.Add(-8 * 24 * time.Hour)
	expiredMonthlyStart := now.Add(-31 * 24 * time.Hour)

	sub := &UserSubscription{
		Status:               SubscriptionStatusActive,
		ExpiresAt:            now.Add(24 * time.Hour),
		DailyWindowStart:     &expiredDailyStart,
		WeeklyWindowStart:    &expiredWeeklyStart,
		MonthlyWindowStart:   &expiredMonthlyStart,
		DailyUsageRequests:   100,
		WeeklyUsageRequests:  500,
		MonthlyUsageRequests: 1000,
	}

	group := &Group{
		SubscriptionType:     SubscriptionTypePerRequest,
		PerRequestPrice:      float64PtrTest(0.05),
		DailyLimitRequests:   int64Ptr(50),  // would fail if not reset
		WeeklyLimitRequests:  int64Ptr(200), // would fail if not reset
		MonthlyLimitRequests: int64Ptr(500), // would fail if not reset
	}

	svc := &SubscriptionService{}
	needsMaintenance, err := svc.ValidateAndCheckLimits(sub, group)

	require.NoError(t, err, "expired windows should be reset in memory")
	require.True(t, needsMaintenance, "should need maintenance for DB sync")
	require.Equal(t, int64(0), sub.DailyUsageRequests, "daily usage should be reset")
	require.Equal(t, int64(0), sub.WeeklyUsageRequests, "weekly usage should be reset")
	require.Equal(t, int64(0), sub.MonthlyUsageRequests, "monthly usage should be reset")
}

func TestValidateAndCheckLimits_SubscriptionModeUSDLimits(t *testing.T) {
	now := time.Now()
	activeWindowStart := now.Add(-12 * time.Hour)

	tests := []struct {
		name        string
		sub         *UserSubscription
		group       *Group
		expectError bool
		errorType   error
	}{
		{
			name: "subscription mode under USD limit - passes",
			sub: &UserSubscription{
				Status:          SubscriptionStatusActive,
				ExpiresAt:       now.Add(24 * time.Hour),
				DailyWindowStart: &activeWindowStart,
				DailyUsageUSD:   5.0,
			},
			group: &Group{
				SubscriptionType: SubscriptionTypeSubscription,
				DailyLimitUSD:    float64PtrTest(10.0),
			},
			expectError: false,
		},
		{
			name: "subscription mode USD limit exceeded - fails",
			sub: &UserSubscription{
				Status:          SubscriptionStatusActive,
				ExpiresAt:       now.Add(24 * time.Hour),
				DailyWindowStart: &activeWindowStart,
				DailyUsageUSD:   10.01, // exceed limit
			},
			group: &Group{
				SubscriptionType: SubscriptionTypeSubscription,
				DailyLimitUSD:    float64PtrTest(10.0),
			},
			expectError: true, // usage > limit means CheckDailyLimit returns false
			errorType:   ErrDailyLimitExceeded,
		},
		{
			name: "subscription mode USD clearly over limit - fails",
			sub: &UserSubscription{
				Status:          SubscriptionStatusActive,
				ExpiresAt:       now.Add(24 * time.Hour),
				DailyWindowStart: &activeWindowStart,
				DailyUsageUSD:   15.0, // clearly over limit
			},
			group: &Group{
				SubscriptionType: SubscriptionTypeSubscription,
				DailyLimitUSD:    float64PtrTest(10.0),
			},
			expectError: true,
			errorType:   ErrDailyLimitExceeded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &SubscriptionService{}
			_, err := svc.ValidateAndCheckLimits(tt.sub, tt.group)

			if tt.expectError {
				require.Error(t, err)
				require.Equal(t, tt.errorType, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}