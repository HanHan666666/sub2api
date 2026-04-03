//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildUsageBillingCommand_PerRequestMode(t *testing.T) {
	userID := int64(1)
	accountID := int64(2)
	apiKeyID := int64(3)
	subscriptionID := int64(10)
	groupID := int64(100)

	tests := []struct {
		name             string
		params           *postUsageBillingParams
		usageLog         *UsageLog
		expectBalanceCost bool
		expectIncrementRequests bool
		expectSubscriptionID bool
	}{
		{
			name: "per_request with price and request limit - both balance and increment",
			params: &postUsageBillingParams{
				IsPerRequestBill: true,
				Cost: &CostBreakdown{
					TotalCost:  0.05,
					ActualCost: 0.05,
				},
				User:     &User{ID: userID},
				Account:  &Account{ID: accountID, Type: AccountTypeOAuth},
				APIKey:   &APIKey{ID: apiKeyID, GroupID: &groupID},
				Group: &Group{
					ID:                   groupID,
					SubscriptionType:     SubscriptionTypePerRequest,
					PerRequestPrice:      float64PtrTest(0.05),
					DailyLimitRequests:   int64Ptr(100),
					WeeklyLimitRequests:  int64Ptr(500),
					MonthlyLimitRequests: int64Ptr(1000),
				},
				Subscription:  &UserSubscription{ID: subscriptionID},
				RequestCount:  1,
			},
			usageLog: &UsageLog{
				Model:        "claude-sonnet-4",
				BillingType:  2, // per_request
			},
			expectBalanceCost:      true,
			expectIncrementRequests: true,
			expectSubscriptionID:    true,
		},
		{
			name: "per_request with price only - balance cost only",
			params: &postUsageBillingParams{
				IsPerRequestBill: true,
				Cost: &CostBreakdown{
					TotalCost:  0.05,
					ActualCost: 0.05,
				},
				User:     &User{ID: userID},
				Account:  &Account{ID: accountID, Type: AccountTypeOAuth},
				APIKey:   &APIKey{ID: apiKeyID, GroupID: &groupID},
				Group: &Group{
					ID:                   groupID,
					SubscriptionType:     SubscriptionTypePerRequest,
					PerRequestPrice:      float64PtrTest(0.05),
					// No request limits
				},
				RequestCount:  1,
			},
			usageLog: &UsageLog{
				Model:        "claude-sonnet-4",
				BillingType:  2,
			},
			expectBalanceCost:      true,
			expectIncrementRequests: false,
			expectSubscriptionID:    false,
		},
		{
			name: "per_request with request limit only - increment only",
			params: &postUsageBillingParams{
				IsPerRequestBill: true,
				Cost: &CostBreakdown{
					TotalCost:  0,
					ActualCost: 0,
				},
				User:     &User{ID: userID},
				Account:  &Account{ID: accountID, Type: AccountTypeOAuth},
				APIKey:   &APIKey{ID: apiKeyID, GroupID: &groupID},
				Group: &Group{
					ID:                   groupID,
					SubscriptionType:     SubscriptionTypePerRequest,
					PerRequestPrice:      nil, // No price
					DailyLimitRequests:   int64Ptr(100),
				},
				Subscription:  &UserSubscription{ID: subscriptionID},
				RequestCount:  1,
			},
			usageLog: &UsageLog{
				Model:        "claude-sonnet-4",
				BillingType:  2,
			},
			expectBalanceCost:      false,
			expectIncrementRequests: true,
			expectSubscriptionID:    true,
		},
		{
			name: "per_request zero cost and no subscription - minimal command",
			params: &postUsageBillingParams{
				IsPerRequestBill: true,
				Cost: &CostBreakdown{
					TotalCost:  0,
					ActualCost: 0,
				},
				User:     &User{ID: userID},
				Account:  &Account{ID: accountID, Type: AccountTypeOAuth},
				APIKey:   &APIKey{ID: apiKeyID, GroupID: &groupID},
				Group: &Group{
					ID:                   groupID,
					SubscriptionType:     SubscriptionTypePerRequest,
				},
				RequestCount:  1,
			},
			usageLog: &UsageLog{
				Model:        "claude-sonnet-4",
				BillingType:  2,
			},
			expectBalanceCost:      false,
			expectIncrementRequests: false,
			expectSubscriptionID:    false,
		},
		{
			name: "standard mode - balance cost only",
			params: &postUsageBillingParams{
				IsPerRequestBill: false,
				Cost: &CostBreakdown{
					TotalCost:  0.10,
					ActualCost: 0.10,
				},
				User:     &User{ID: userID},
				Account:  &Account{ID: accountID, Type: AccountTypeOAuth},
				APIKey:   &APIKey{ID: apiKeyID, GroupID: &groupID},
				Group: &Group{
					ID:                   groupID,
					SubscriptionType:     SubscriptionTypeStandard,
				},
			},
			usageLog: &UsageLog{
				Model:        "claude-sonnet-4",
				BillingType:  0, // standard
			},
			expectBalanceCost:      true,
			expectIncrementRequests: false,
			expectSubscriptionID:    false,
		},
		{
			name: "subscription mode - subscription cost only",
			params: &postUsageBillingParams{
				IsSubscriptionBill: true,
				Cost: &CostBreakdown{
					TotalCost:  0.10,
					ActualCost: 0.10,
				},
				User:     &User{ID: userID},
				Account:  &Account{ID: accountID, Type: AccountTypeOAuth},
				APIKey:   &APIKey{ID: apiKeyID, GroupID: &groupID},
				Group: &Group{
					ID:                   groupID,
					SubscriptionType:     SubscriptionTypeSubscription,
					DailyLimitUSD:        float64PtrTest(10.0),
				},
				Subscription:  &UserSubscription{ID: subscriptionID},
			},
			usageLog: &UsageLog{
				Model:        "claude-sonnet-4",
				BillingType:  1, // subscription
			},
			expectBalanceCost:      false,
			expectIncrementRequests: false,
			expectSubscriptionID:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := buildUsageBillingCommand("test-request-id", tt.usageLog, tt.params)

			require.NotNil(t, cmd)
			require.Equal(t, "test-request-id", cmd.RequestID)
			require.Equal(t, tt.params.User.ID, cmd.UserID)
			require.Equal(t, tt.params.Account.ID, cmd.AccountID)
			require.Equal(t, tt.params.APIKey.ID, cmd.APIKeyID)

			if tt.expectBalanceCost {
				require.Equal(t, tt.params.Cost.ActualCost, cmd.BalanceCost, "should set BalanceCost")
			} else {
				require.Equal(t, 0.0, cmd.BalanceCost, "should not set BalanceCost")
			}

			if tt.expectIncrementRequests {
				require.Equal(t, tt.params.RequestCount, cmd.IncrementRequests, "should set IncrementRequests")
			} else {
				require.Equal(t, int64(0), cmd.IncrementRequests, "should not set IncrementRequests")
			}

			if tt.expectSubscriptionID {
				require.NotNil(t, cmd.SubscriptionID, "should set SubscriptionID")
				require.Equal(t, tt.params.Subscription.ID, *cmd.SubscriptionID)
			}

			if tt.usageLog != nil {
				require.Equal(t, tt.usageLog.Model, cmd.Model)
				require.Equal(t, tt.usageLog.BillingType, cmd.BillingType)
			}
		})
	}
}

func TestBuildUsageBillingCommand_NilCases(t *testing.T) {
	tests := []struct {
		name   string
		params *postUsageBillingParams
	}{
		{
			name:   "nil params returns nil",
			params: nil,
		},
		{
			name: "nil cost returns nil",
			params: &postUsageBillingParams{
				Cost:    nil,
				User:    &User{ID: 1},
				Account: &Account{ID: 2},
				APIKey:  &APIKey{ID: 3},
			},
		},
		{
			name: "nil user returns nil",
			params: &postUsageBillingParams{
				Cost:    &CostBreakdown{},
				User:    nil,
				Account: &Account{ID: 2},
				APIKey:  &APIKey{ID: 3},
			},
		},
		{
			name: "nil account returns nil",
			params: &postUsageBillingParams{
				Cost:    &CostBreakdown{},
				User:    &User{ID: 1},
				Account: nil,
				APIKey:  &APIKey{ID: 3},
			},
		},
		{
			name: "nil apikey returns nil",
			params: &postUsageBillingParams{
				Cost:    &CostBreakdown{},
				User:    &User{ID: 1},
				Account: &Account{ID: 2},
				APIKey:  nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := buildUsageBillingCommand("test-request-id", nil, tt.params)
			require.Nil(t, cmd)
		})
	}
}

func TestBuildUsageBillingCommand_PerRequestBothBalanceAndIncrement(t *testing.T) {
	// This is a critical test for per_request billing correctness
	// It verifies that both BalanceCost and IncrementRequests can be set simultaneously
	userID := int64(1)
	accountID := int64(2)
	apiKeyID := int64(3)
	subscriptionID := int64(10)
	groupID := int64(100)

	params := &postUsageBillingParams{
		IsPerRequestBill: true,
		Cost: &CostBreakdown{
			TotalCost:  0.04,  // $0.05 * 0.8 rate multiplier
			ActualCost: 0.04,
		},
		User:     &User{ID: userID},
		Account:  &Account{ID: accountID, Type: AccountTypeOAuth},
		APIKey:   &APIKey{ID: apiKeyID, GroupID: &groupID},
		Group: &Group{
			ID:                   groupID,
			SubscriptionType:     SubscriptionTypePerRequest,
			PerRequestPrice:      float64PtrTest(0.05),
			RateMultiplier:       0.8,
			DailyLimitRequests:   int64Ptr(100),
			WeeklyLimitRequests:  int64Ptr(500),
			MonthlyLimitRequests: int64Ptr(1000),
		},
		Subscription:  &UserSubscription{ID: subscriptionID},
		RequestCount:  1,
	}

	usageLog := &UsageLog{
		Model:        "claude-sonnet-4",
		BillingType:  2, // per_request
	}

	cmd := buildUsageBillingCommand("test-request-id", usageLog, params)

	require.NotNil(t, cmd)

	// Critical assertion: Both BalanceCost AND IncrementRequests should be set
	require.Equal(t, 0.04, cmd.BalanceCost, "BalanceCost should be set for per_request with price")
	require.Equal(t, int64(1), cmd.IncrementRequests, "IncrementRequests should be set for per_request with limit")
	require.NotNil(t, cmd.SubscriptionID, "SubscriptionID should be set for request count increment")
	require.Equal(t, subscriptionID, *cmd.SubscriptionID)

	// Verify billing type
	require.Equal(t, int8(2), cmd.BillingType, "BillingType should be 2 (per_request)")
}

func TestBuildUsageBillingCommand_RequestFingerprint(t *testing.T) {
	userID := int64(1)
	accountID := int64(2)
	apiKeyID := int64(3)

	params := &postUsageBillingParams{
		Cost:    &CostBreakdown{ActualCost: 0.1},
		User:    &User{ID: userID},
		Account: &Account{ID: accountID, Type: AccountTypeOAuth},
		APIKey:  &APIKey{ID: apiKeyID},
	}

	cmd := buildUsageBillingCommand("test-request-id", nil, params)

	require.NotNil(t, cmd)
	require.NotEmpty(t, cmd.RequestFingerprint, "RequestFingerprint should be auto-generated")
}

func TestFinalizePostUsageBilling_NilSafety(t *testing.T) {
	// Test nil params - should not panic
	require.NotPanics(t, func() {
		finalizePostUsageBilling(nil, nil)
	})

	// Test nil cost - should not panic
	require.NotPanics(t, func() {
		finalizePostUsageBilling(&postUsageBillingParams{User: &User{ID: 1}}, nil)
	})

	// Test nil deps - should not panic
	require.NotPanics(t, func() {
		finalizePostUsageBilling(&postUsageBillingParams{Cost: &CostBreakdown{ActualCost: 0.1}}, nil)
	})

	// Test nil user - should not panic
	require.NotPanics(t, func() {
		finalizePostUsageBilling(&postUsageBillingParams{
			Cost:    &CostBreakdown{ActualCost: 0.1},
			User:    nil,
			Account: &Account{ID: 1},
		}, nil)
	})

	// Test per_request mode with nil group - should not panic
	require.NotPanics(t, func() {
		finalizePostUsageBilling(&postUsageBillingParams{
			IsPerRequestBill: true,
			Cost:            &CostBreakdown{ActualCost: 0.05},
			User:            &User{ID: 1},
			Account:         &Account{ID: 1},
			APIKey:          &APIKey{ID: 1},
			Group:           nil,
			RequestCount:    1,
		}, nil)
	})
}