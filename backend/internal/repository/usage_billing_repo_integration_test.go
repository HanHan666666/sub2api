//go:build integration

package repository

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestUsageBillingRepositoryApply_DeduplicatesBalanceBilling(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("usage-billing-user-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Balance:      100,
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-usage-billing-" + uuid.NewString(),
		Name:   "billing",
		Quota:  1,
	})
	account := mustCreateAccount(t, client, &service.Account{
		Name: "usage-billing-account-" + uuid.NewString(),
		Type: service.AccountTypeAPIKey,
	})

	requestID := uuid.NewString()
	cmd := &service.UsageBillingCommand{
		RequestID:           requestID,
		APIKeyID:            apiKey.ID,
		UserID:              user.ID,
		AccountID:           account.ID,
		AccountType:         service.AccountTypeAPIKey,
		BalanceCost:         1.25,
		APIKeyQuotaCost:     1.25,
		APIKeyRateLimitCost: 1.25,
	}

	result1, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.NotNil(t, result1)
	require.True(t, result1.Applied)
	require.True(t, result1.APIKeyQuotaExhausted)

	result2, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.NotNil(t, result2)
	require.False(t, result2.Applied)

	var balance float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance FROM users WHERE id = $1", user.ID).Scan(&balance))
	require.InDelta(t, 98.75, balance, 0.000001)

	var quotaUsed float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT quota_used FROM api_keys WHERE id = $1", apiKey.ID).Scan(&quotaUsed))
	require.InDelta(t, 1.25, quotaUsed, 0.000001)

	var usage5h float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT usage_5h FROM api_keys WHERE id = $1", apiKey.ID).Scan(&usage5h))
	require.InDelta(t, 1.25, usage5h, 0.000001)

	var status string
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT status FROM api_keys WHERE id = $1", apiKey.ID).Scan(&status))
	require.Equal(t, service.StatusAPIKeyQuotaExhausted, status)

	var dedupCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_billing_dedup WHERE request_id = $1 AND api_key_id = $2", requestID, apiKey.ID).Scan(&dedupCount))
	require.Equal(t, 1, dedupCount)
}

func TestUsageBillingRepositoryApply_DeduplicatesSubscriptionBilling(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("usage-billing-sub-user-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
	})
	group := mustCreateGroup(t, client, &service.Group{
		Name:             "usage-billing-group-" + uuid.NewString(),
		Platform:         service.PlatformAnthropic,
		SubscriptionType: service.SubscriptionTypeSubscription,
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID:  user.ID,
		GroupID: &group.ID,
		Key:     "sk-usage-billing-sub-" + uuid.NewString(),
		Name:    "billing-sub",
	})
	subscription := mustCreateSubscription(t, client, &service.UserSubscription{
		UserID:  user.ID,
		GroupID: group.ID,
	})

	requestID := uuid.NewString()
	cmd := &service.UsageBillingCommand{
		RequestID:        requestID,
		APIKeyID:         apiKey.ID,
		UserID:           user.ID,
		AccountID:        0,
		SubscriptionID:   &subscription.ID,
		SubscriptionCost: 2.5,
	}

	result1, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.True(t, result1.Applied)

	result2, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.False(t, result2.Applied)

	var dailyUsage float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT daily_usage_usd FROM user_subscriptions WHERE id = $1", subscription.ID).Scan(&dailyUsage))
	require.InDelta(t, 2.5, dailyUsage, 0.000001)
}

func TestUsageBillingRepositoryApply_RequestFingerprintConflict(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("usage-billing-conflict-user-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Balance:      100,
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-usage-billing-conflict-" + uuid.NewString(),
		Name:   "billing-conflict",
	})

	requestID := uuid.NewString()
	_, err := repo.Apply(ctx, &service.UsageBillingCommand{
		RequestID:   requestID,
		APIKeyID:    apiKey.ID,
		UserID:      user.ID,
		BalanceCost: 1.25,
	})
	require.NoError(t, err)

	_, err = repo.Apply(ctx, &service.UsageBillingCommand{
		RequestID:   requestID,
		APIKeyID:    apiKey.ID,
		UserID:      user.ID,
		BalanceCost: 2.50,
	})
	require.ErrorIs(t, err, service.ErrUsageBillingRequestConflict)
}

func TestUsageBillingRepositoryApply_UpdatesAccountQuota(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("usage-billing-account-user-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-usage-billing-account-" + uuid.NewString(),
		Name:   "billing-account",
	})
	account := mustCreateAccount(t, client, &service.Account{
		Name: "usage-billing-account-quota-" + uuid.NewString(),
		Type: service.AccountTypeAPIKey,
		Extra: map[string]any{
			"quota_limit": 100.0,
		},
	})

	_, err := repo.Apply(ctx, &service.UsageBillingCommand{
		RequestID:        uuid.NewString(),
		APIKeyID:         apiKey.ID,
		UserID:           user.ID,
		AccountID:        account.ID,
		AccountType:      service.AccountTypeAPIKey,
		AccountQuotaCost: 3.5,
	})
	require.NoError(t, err)

	var quotaUsed float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COALESCE((extra->>'quota_used')::numeric, 0) FROM accounts WHERE id = $1", account.ID).Scan(&quotaUsed))
	require.InDelta(t, 3.5, quotaUsed, 0.000001)
}

func TestDashboardAggregationRepositoryCleanupUsageBillingDedup_BatchDeletesOldRows(t *testing.T) {
	ctx := context.Background()
	repo := newDashboardAggregationRepositoryWithSQL(integrationDB)

	oldRequestID := "dedup-old-" + uuid.NewString()
	newRequestID := "dedup-new-" + uuid.NewString()
	oldCreatedAt := time.Now().UTC().AddDate(0, 0, -400)
	newCreatedAt := time.Now().UTC().Add(-time.Hour)

	_, err := integrationDB.ExecContext(ctx, `
		INSERT INTO usage_billing_dedup (request_id, api_key_id, request_fingerprint, created_at)
		VALUES ($1, 1, $2, $3), ($4, 1, $5, $6)
	`,
		oldRequestID, strings.Repeat("a", 64), oldCreatedAt,
		newRequestID, strings.Repeat("b", 64), newCreatedAt,
	)
	require.NoError(t, err)

	require.NoError(t, repo.CleanupUsageBillingDedup(ctx, time.Now().UTC().AddDate(0, 0, -365)))

	var oldCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_billing_dedup WHERE request_id = $1", oldRequestID).Scan(&oldCount))
	require.Equal(t, 0, oldCount)

	var newCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_billing_dedup WHERE request_id = $1", newRequestID).Scan(&newCount))
	require.Equal(t, 1, newCount)

	var archivedCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_billing_dedup_archive WHERE request_id = $1", oldRequestID).Scan(&archivedCount))
	require.Equal(t, 1, archivedCount)
}

func TestUsageBillingRepositoryApply_DeduplicatesAgainstArchivedKey(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)
	aggRepo := newDashboardAggregationRepositoryWithSQL(integrationDB)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("usage-billing-archive-user-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Balance:      100,
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-usage-billing-archive-" + uuid.NewString(),
		Name:   "billing-archive",
	})

	requestID := uuid.NewString()
	cmd := &service.UsageBillingCommand{
		RequestID:   requestID,
		APIKeyID:    apiKey.ID,
		UserID:      user.ID,
		BalanceCost: 1.25,
	}

	result1, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.True(t, result1.Applied)

	_, err = integrationDB.ExecContext(ctx, `
		UPDATE usage_billing_dedup
		SET created_at = $1
		WHERE request_id = $2 AND api_key_id = $3
	`, time.Now().UTC().AddDate(0, 0, -400), requestID, apiKey.ID)
	require.NoError(t, err)
	require.NoError(t, aggRepo.CleanupUsageBillingDedup(ctx, time.Now().UTC().AddDate(0, 0, -365)))

	result2, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.False(t, result2.Applied)

	var balance float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance FROM users WHERE id = $1", user.ID).Scan(&balance))
	require.InDelta(t, 98.75, balance, 0.000001)
}

// TestUsageBillingRepositoryApply_PerRequestIncrementRequests verifies that IncrementRequests
// atomically increments all three request count fields (daily, weekly, monthly) in a single transaction.
func TestUsageBillingRepositoryApply_PerRequestIncrementRequests(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("per-request-user-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
	})
	group := mustCreateGroup(t, client, &service.Group{
		Name:             "per-request-group-" + uuid.NewString(),
		Platform:         service.PlatformAnthropic,
		SubscriptionType: service.SubscriptionTypePerRequest,
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID:  user.ID,
		GroupID: &group.ID,
		Key:     "sk-per-request-" + uuid.NewString(),
		Name:    "per-request-test",
	})
	subscription := mustCreateSubscription(t, client, &service.UserSubscription{
		UserID:              user.ID,
		GroupID:             group.ID,
		DailyUsageRequests:  10,
		WeeklyUsageRequests: 50,
		MonthlyUsageRequests: 200,
	})

	requestID := uuid.NewString()
	cmd := &service.UsageBillingCommand{
		RequestID:        requestID,
		APIKeyID:         apiKey.ID,
		UserID:           user.ID,
		AccountID:        0,
		SubscriptionID:   &subscription.ID,
		IncrementRequests: 1,
	}

	result, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Applied)

	// Verify all three request count fields were incremented atomically
	var dailyRequests, weeklyRequests, monthlyRequests int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT daily_usage_requests, weekly_usage_requests, monthly_usage_requests
		FROM user_subscriptions WHERE id = $1
	`, subscription.ID).Scan(&dailyRequests, &weeklyRequests, &monthlyRequests))

	require.Equal(t, int64(11), dailyRequests, "daily_usage_requests should be incremented by 1")
	require.Equal(t, int64(51), weeklyRequests, "weekly_usage_requests should be incremented by 1")
	require.Equal(t, int64(201), monthlyRequests, "monthly_usage_requests should be incremented by 1")

	// Verify deduplication prevents double-counting
	result2, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.False(t, result2.Applied, "duplicate request should not be applied")

	// Verify counts remain unchanged after duplicate attempt
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT daily_usage_requests, weekly_usage_requests, monthly_usage_requests
		FROM user_subscriptions WHERE id = $1
	`, subscription.ID).Scan(&dailyRequests, &weeklyRequests, &monthlyRequests))

	require.Equal(t, int64(11), dailyRequests, "daily count should remain unchanged")
	require.Equal(t, int64(51), weeklyRequests, "weekly count should remain unchanged")
	require.Equal(t, int64(201), monthlyRequests, "monthly count should remain unchanged")
}

// TestUsageBillingRepositoryApply_PerRequestBothBalanceAndIncrement verifies the critical
// per_request billing behavior where both BalanceCost and IncrementRequests are applied
// atomically in the same transaction.
func TestUsageBillingRepositoryApply_PerRequestBothBalanceAndIncrement(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("per-request-both-user-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Balance:      10.0,
	})
	group := mustCreateGroup(t, client, &service.Group{
		Name:             "per-request-both-group-" + uuid.NewString(),
		Platform:         service.PlatformAnthropic,
		SubscriptionType: service.SubscriptionTypePerRequest,
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID:  user.ID,
		GroupID: &group.ID,
		Key:     "sk-per-request-both-" + uuid.NewString(),
		Name:    "per-request-both-test",
	})
	subscription := mustCreateSubscription(t, client, &service.UserSubscription{
		UserID:              user.ID,
		GroupID:             group.ID,
		DailyUsageRequests:  0,
		WeeklyUsageRequests: 0,
		MonthlyUsageRequests: 0,
	})

	requestID := uuid.NewString()
	cmd := &service.UsageBillingCommand{
		RequestID:          requestID,
		APIKeyID:           apiKey.ID,
		UserID:             user.ID,
		AccountID:          0,
		SubscriptionID:     &subscription.ID,
		BalanceCost:        0.05,  // per_request price deduction
		IncrementRequests:  1,     // request count increment
	}

	result, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Applied)

	// Verify balance was deducted
	var balance float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance FROM users WHERE id = $1", user.ID).Scan(&balance))
	require.InDelta(t, 9.95, balance, 0.000001, "balance should be deducted by 0.05")

	// Verify request counts were incremented
	var dailyRequests, weeklyRequests, monthlyRequests int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT daily_usage_requests, weekly_usage_requests, monthly_usage_requests
		FROM user_subscriptions WHERE id = $1
	`, subscription.ID).Scan(&dailyRequests, &weeklyRequests, &monthlyRequests))

	require.Equal(t, int64(1), dailyRequests, "daily_usage_requests should be 1")
	require.Equal(t, int64(1), weeklyRequests, "weekly_usage_requests should be 1")
	require.Equal(t, int64(1), monthlyRequests, "monthly_usage_requests should be 1")

	// Verify deduplication prevents double application
	result2, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.False(t, result2.Applied, "duplicate request should not apply both balance and increment")
}

// TestUsageBillingRepositoryApply_PerRequestIncrementMultiple verifies that IncrementRequests
// can be greater than 1 and all three fields are incremented by the same amount atomically.
func TestUsageBillingRepositoryApply_PerRequestIncrementMultiple(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("per-request-multi-user-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
	})
	group := mustCreateGroup(t, client, &service.Group{
		Name:             "per-request-multi-group-" + uuid.NewString(),
		Platform:         service.PlatformAnthropic,
		SubscriptionType: service.SubscriptionTypePerRequest,
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID:  user.ID,
		GroupID: &group.ID,
		Key:     "sk-per-request-multi-" + uuid.NewString(),
		Name:    "per-request-multi-test",
	})
	subscription := mustCreateSubscription(t, client, &service.UserSubscription{
		UserID:              user.ID,
		GroupID:             group.ID,
		DailyUsageRequests:  5,
		WeeklyUsageRequests: 25,
		MonthlyUsageRequests: 100,
	})

	requestID := uuid.NewString()
	cmd := &service.UsageBillingCommand{
		RequestID:         requestID,
		APIKeyID:          apiKey.ID,
		UserID:            user.ID,
		AccountID:         0,
		SubscriptionID:    &subscription.ID,
		IncrementRequests: 3, // increment by 3 requests
	}

	result, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.True(t, result.Applied)

	var dailyRequests, weeklyRequests, monthlyRequests int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT daily_usage_requests, weekly_usage_requests, monthly_usage_requests
		FROM user_subscriptions WHERE id = $1
	`, subscription.ID).Scan(&dailyRequests, &weeklyRequests, &monthlyRequests))

	require.Equal(t, int64(8), dailyRequests, "daily should be 5+3")
	require.Equal(t, int64(28), weeklyRequests, "weekly should be 25+3")
	require.Equal(t, int64(103), monthlyRequests, "monthly should be 100+3")
}

// TestUsageBillingRepositoryApply_PerRequestMissingSubscriptionID verifies that
// IncrementRequests is skipped when SubscriptionID is nil, even if IncrementRequests > 0.
func TestUsageBillingRepositoryApply_PerRequestMissingSubscriptionID(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("per-request-nosub-user-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-per-request-nosub-" + uuid.NewString(),
		Name:   "per-request-nosub-test",
	})

	requestID := uuid.NewString()
	cmd := &service.UsageBillingCommand{
		RequestID:         requestID,
		APIKeyID:          apiKey.ID,
		UserID:            user.ID,
		AccountID:         0,
		SubscriptionID:    nil, // no subscription
		IncrementRequests: 1,   // this should be skipped
		BalanceCost:       0.10, // this should still apply
	}

	result, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.True(t, result.Applied)

	// Balance should be deducted
	var balance float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance FROM users WHERE id = $1", user.ID).Scan(&balance))
	require.InDelta(t, -0.10, balance, 0.000001, "balance should be deducted even without subscription")

	// No subscription to check request counts - operation succeeds without increment
}

// TestUsageBillingRepositoryApply_PerRequestConcurrent verifies that 50 concurrent
// billing requests result in accurate request count increments. This is a critical
// test for per_request billing correctness under concurrent load.
func TestUsageBillingRepositoryApply_PerRequestConcurrent(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("per-request-concurrent-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Balance:      100.0,
	})
	group := mustCreateGroup(t, client, &service.Group{
		Name:             "per-request-concurrent-group-" + uuid.NewString(),
		Platform:         service.PlatformAnthropic,
		SubscriptionType: service.SubscriptionTypePerRequest,
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID:  user.ID,
		GroupID: &group.ID,
		Key:     "sk-per-request-concurrent-" + uuid.NewString(),
		Name:    "per-request-concurrent-test",
	})
	subscription := mustCreateSubscription(t, client, &service.UserSubscription{
		UserID:              user.ID,
		GroupID:             group.ID,
		DailyUsageRequests:  0,
		WeeklyUsageRequests: 0,
		MonthlyUsageRequests: 0,
	})

	// 50 concurrent requests, each incrementing by 1
	const numRequests = 50
	const perRequestCost = 0.05

	var wg sync.WaitGroup
	errs := make([]error, numRequests)
	results := make([]*service.UsageBillingApplyResult, numRequests)
	requestIDs := make([]string, numRequests) // Store request IDs for duplicate test

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			requestIDs[idx] = fmt.Sprintf("concurrent-req-%d-%s", idx, uuid.NewString())
			cmd := &service.UsageBillingCommand{
				RequestID:         requestIDs[idx],
				APIKeyID:          apiKey.ID,
				UserID:            user.ID,
				AccountID:         0,
				SubscriptionID:    &subscription.ID,
				BalanceCost:       perRequestCost,
				IncrementRequests: 1,
			}
			results[idx], errs[idx] = repo.Apply(ctx, cmd)
		}(i)
	}
	wg.Wait()

	// Verify all requests succeeded without errors
	for i, e := range errs {
		require.NoError(t, e, "goroutine %d should not have error", i)
	}

	// Count how many requests were actually applied (should be exactly 50)
	appliedCount := 0
	for i, r := range results {
		require.NotNil(t, r, "goroutine %d should have result", i)
		if r.Applied {
			appliedCount++
		}
	}
	require.Equal(t, numRequests, appliedCount, "all %d requests should be applied", numRequests)

	// Verify final request counts are exactly 50 for all three windows
	var dailyRequests, weeklyRequests, monthlyRequests int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT daily_usage_requests, weekly_usage_requests, monthly_usage_requests
		FROM user_subscriptions WHERE id = $1
	`, subscription.ID).Scan(&dailyRequests, &weeklyRequests, &monthlyRequests))

	require.Equal(t, int64(numRequests), dailyRequests, "daily_usage_requests should be exactly %d", numRequests)
	require.Equal(t, int64(numRequests), weeklyRequests, "weekly_usage_requests should be exactly %d", numRequests)
	require.Equal(t, int64(numRequests), monthlyRequests, "monthly_usage_requests should be exactly %d", numRequests)

	// Verify balance was deducted exactly numRequests * perRequestCost
	var balance float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance FROM users WHERE id = $1", user.ID).Scan(&balance))
	expectedBalance := 100.0 - (float64(numRequests) * perRequestCost)
	require.InDelta(t, expectedBalance, balance, 0.000001, "balance should be exactly %.2f after %d deductions", expectedBalance, numRequests)

	// Verify deduplication: re-applying existing request should fail
	for i := 0; i < 5; i++ {
		cmd := &service.UsageBillingCommand{
			RequestID:         requestIDs[i], // Use saved request ID (real duplicate)
			APIKeyID:          apiKey.ID,
			UserID:            user.ID,
			SubscriptionID:    &subscription.ID,
			BalanceCost:       perRequestCost,
			IncrementRequests: 1,
		}
		result, err := repo.Apply(ctx, cmd)
		require.NoError(t, err)
		require.False(t, result.Applied, "duplicate request %d should not be applied", i)
	}
}
