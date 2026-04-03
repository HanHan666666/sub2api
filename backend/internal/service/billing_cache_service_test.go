package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type billingCacheWorkerStub struct {
	balanceUpdates      int64
	subscriptionUpdates int64
}

func (b *billingCacheWorkerStub) GetUserBalance(ctx context.Context, userID int64) (float64, error) {
	return 0, errors.New("not implemented")
}

func (b *billingCacheWorkerStub) SetUserBalance(ctx context.Context, userID int64, balance float64) error {
	atomic.AddInt64(&b.balanceUpdates, 1)
	return nil
}

func (b *billingCacheWorkerStub) DeductUserBalance(ctx context.Context, userID int64, amount float64) error {
	atomic.AddInt64(&b.balanceUpdates, 1)
	return nil
}

func (b *billingCacheWorkerStub) InvalidateUserBalance(ctx context.Context, userID int64) error {
	return nil
}

func (b *billingCacheWorkerStub) GetSubscriptionCache(ctx context.Context, userID, groupID int64) (*SubscriptionCacheData, error) {
	return nil, errors.New("not implemented")
}

func (b *billingCacheWorkerStub) SetSubscriptionCache(ctx context.Context, userID, groupID int64, data *SubscriptionCacheData) error {
	atomic.AddInt64(&b.subscriptionUpdates, 1)
	return nil
}

func (b *billingCacheWorkerStub) UpdateSubscriptionUsage(ctx context.Context, userID, groupID int64, cost float64) error {
	atomic.AddInt64(&b.subscriptionUpdates, 1)
	return nil
}

func (b *billingCacheWorkerStub) IncrementSubscriptionRequestUsage(ctx context.Context, userID, groupID int64, count int64) error {
	atomic.AddInt64(&b.subscriptionUpdates, 1)
	return nil
}

func (b *billingCacheWorkerStub) InvalidateSubscriptionCache(ctx context.Context, userID, groupID int64) error {
	return nil
}

func (b *billingCacheWorkerStub) GetAPIKeyRateLimit(ctx context.Context, keyID int64) (*APIKeyRateLimitCacheData, error) {
	return nil, errors.New("not implemented")
}

func (b *billingCacheWorkerStub) SetAPIKeyRateLimit(ctx context.Context, keyID int64, data *APIKeyRateLimitCacheData) error {
	return nil
}

func (b *billingCacheWorkerStub) UpdateAPIKeyRateLimitUsage(ctx context.Context, keyID int64, cost float64) error {
	return nil
}

func (b *billingCacheWorkerStub) InvalidateAPIKeyRateLimit(ctx context.Context, keyID int64) error {
	return nil
}

func TestBillingCacheServiceQueueHighLoad(t *testing.T) {
	cache := &billingCacheWorkerStub{}
	svc := NewBillingCacheService(cache, nil, nil, nil, &config.Config{})
	t.Cleanup(svc.Stop)

	start := time.Now()
	for i := 0; i < cacheWriteBufferSize*2; i++ {
		svc.QueueDeductBalance(1, 1)
	}
	require.Less(t, time.Since(start), 2*time.Second)

	svc.QueueUpdateSubscriptionUsage(1, 2, 1.5)

	require.Eventually(t, func() bool {
		return atomic.LoadInt64(&cache.balanceUpdates) > 0
	}, 2*time.Second, 10*time.Millisecond)

	require.Eventually(t, func() bool {
		return atomic.LoadInt64(&cache.subscriptionUpdates) > 0
	}, 2*time.Second, 10*time.Millisecond)
}

func TestBillingCacheServiceEnqueueAfterStopReturnsFalse(t *testing.T) {
	cache := &billingCacheWorkerStub{}
	svc := NewBillingCacheService(cache, nil, nil, nil, &config.Config{})
	svc.Stop()

	enqueued := svc.enqueueCacheWrite(cacheWriteTask{
		kind:   cacheWriteDeductBalance,
		userID: 1,
		amount: 1,
	})
	require.False(t, enqueued)
}

// ============================================
// CheckBillingEligibility per_request 模式测试
// ============================================

type perRequestMockCache struct {
	balance             float64
	subscriptionData    *SubscriptionCacheData
	balanceErr          error
	subscriptionErr     error
	rateLimitData       *APIKeyRateLimitCacheData
	rateLimitErr        error
	incrementReqCalls   int64
	incrementReqUserID  int64
	incrementReqGroupID int64
	incrementReqCount   int64
}

func (m *perRequestMockCache) GetUserBalance(ctx context.Context, userID int64) (float64, error) {
	return m.balance, m.balanceErr
}

func (m *perRequestMockCache) SetUserBalance(ctx context.Context, userID int64, balance float64) error {
	m.balance = balance
	return nil
}

func (m *perRequestMockCache) DeductUserBalance(ctx context.Context, userID int64, amount float64) error {
	m.balance -= amount
	return nil
}

func (m *perRequestMockCache) InvalidateUserBalance(ctx context.Context, userID int64) error {
	return nil
}

func (m *perRequestMockCache) GetSubscriptionCache(ctx context.Context, userID, groupID int64) (*SubscriptionCacheData, error) {
	return m.subscriptionData, m.subscriptionErr
}

func (m *perRequestMockCache) SetSubscriptionCache(ctx context.Context, userID, groupID int64, data *SubscriptionCacheData) error {
	m.subscriptionData = data
	return nil
}

func (m *perRequestMockCache) UpdateSubscriptionUsage(ctx context.Context, userID, groupID int64, cost float64) error {
	return nil
}

func (m *perRequestMockCache) IncrementSubscriptionRequestUsage(ctx context.Context, userID, groupID int64, count int64) error {
	atomic.AddInt64(&m.incrementReqCalls, 1)
	m.incrementReqUserID = userID
	m.incrementReqGroupID = groupID
	m.incrementReqCount = count
	return nil
}

func (m *perRequestMockCache) InvalidateSubscriptionCache(ctx context.Context, userID, groupID int64) error {
	return nil
}

func (m *perRequestMockCache) GetAPIKeyRateLimit(ctx context.Context, keyID int64) (*APIKeyRateLimitCacheData, error) {
	return m.rateLimitData, m.rateLimitErr
}

func (m *perRequestMockCache) SetAPIKeyRateLimit(ctx context.Context, keyID int64, data *APIKeyRateLimitCacheData) error {
	m.rateLimitData = data
	return nil
}

func (m *perRequestMockCache) UpdateAPIKeyRateLimitUsage(ctx context.Context, keyID int64, cost float64) error {
	return nil
}

func (m *perRequestMockCache) InvalidateAPIKeyRateLimit(ctx context.Context, keyID int64) error {
	return nil
}

func TestCheckBillingEligibility_PerRequest_PriceOnly_Pass(t *testing.T) {
	// 固定价格模式（无次数限制），余额充足应该通过
	price := 0.01
	cache := &perRequestMockCache{balance: 1.0}
	svc := NewBillingCacheService(cache, nil, nil, nil, &config.Config{})

	user := &User{ID: 1}
	group := &Group{
		ID:                1,
		SubscriptionType:  SubscriptionTypePerRequest,
		PerRequestPrice:   &price,
		Status:            StatusActive,
		DailyLimitRequests: nil, // 无次数限制
	}

	err := svc.CheckBillingEligibility(context.Background(), user, nil, group, nil)
	require.NoError(t, err, "per_request with price only and sufficient balance should pass")
}

func TestCheckBillingEligibility_PerRequest_PriceOnly_InsufficientBalance(t *testing.T) {
	// 固定价格模式，余额不足应该失败
	price := 0.01
	cache := &perRequestMockCache{balance: 0.005}
	svc := NewBillingCacheService(cache, nil, nil, nil, &config.Config{})

	user := &User{ID: 1}
	group := &Group{
		ID:               1,
		SubscriptionType: SubscriptionTypePerRequest,
		PerRequestPrice:  &price,
		Status:           StatusActive,
	}

	err := svc.CheckBillingEligibility(context.Background(), user, nil, group, nil)
	require.Error(t, err, "per_request with insufficient balance should fail")
	require.ErrorIs(t, err, ErrInsufficientBalance)
}

func TestCheckBillingEligibility_PerRequest_LimitOnly_NoSubscription(t *testing.T) {
	// 仅次数限制模式，没有订阅应该失败
	limit := int64(100)
	cache := &perRequestMockCache{}
	svc := NewBillingCacheService(cache, nil, nil, nil, &config.Config{})

	user := &User{ID: 1}
	group := &Group{
		ID:                 1,
		SubscriptionType:   SubscriptionTypePerRequest,
		DailyLimitRequests: &limit,
		Status:             StatusActive,
	}

	err := svc.CheckBillingEligibility(context.Background(), user, nil, group, nil)
	require.Error(t, err, "per_request with request limit but no subscription should fail")
	require.ErrorIs(t, err, ErrSubscriptionRequired)
}

func TestCheckBillingEligibility_PerRequest_LimitOnly_WithinLimit(t *testing.T) {
	// 仅次数限制模式，次数未超出应该通过
	limit := int64(100)
	cache := &perRequestMockCache{
		subscriptionData: &SubscriptionCacheData{
			Status:             SubscriptionStatusActive,
			ExpiresAt:          time.Now().Add(24 * time.Hour),
			DailyUsageRequests: 50,
		},
	}
	svc := NewBillingCacheService(cache, nil, nil, nil, &config.Config{})

	user := &User{ID: 1}
	group := &Group{
		ID:                 1,
		SubscriptionType:   SubscriptionTypePerRequest,
		DailyLimitRequests: &limit,
		Status:             StatusActive,
	}
	subscription := &UserSubscription{ID: 1, UserID: 1, GroupID: 1}

	err := svc.CheckBillingEligibility(context.Background(), user, nil, group, subscription)
	require.NoError(t, err, "per_request within request limit should pass")
}

func TestCheckBillingEligibility_PerRequest_LimitOnly_DailyExceeded(t *testing.T) {
	// 仅次数限制模式，日次数超出应该失败
	limit := int64(100)
	cache := &perRequestMockCache{
		subscriptionData: &SubscriptionCacheData{
			Status:             SubscriptionStatusActive,
			ExpiresAt:          time.Now().Add(24 * time.Hour),
			DailyUsageRequests: 100, // 已达限额
		},
	}
	svc := NewBillingCacheService(cache, nil, nil, nil, &config.Config{})

	user := &User{ID: 1}
	group := &Group{
		ID:                 1,
		SubscriptionType:   SubscriptionTypePerRequest,
		DailyLimitRequests: &limit,
		Status:             StatusActive,
	}
	subscription := &UserSubscription{ID: 1, UserID: 1, GroupID: 1}

	err := svc.CheckBillingEligibility(context.Background(), user, nil, group, subscription)
	require.Error(t, err, "per_request exceeding daily request limit should fail")
	require.ErrorIs(t, err, ErrDailyRequestLimitExceeded)
}

func TestCheckBillingEligibility_PerRequest_LimitOnly_WeeklyExceeded(t *testing.T) {
	// 仅次数限制模式，周次数超出应该失败
	limit := int64(500)
	cache := &perRequestMockCache{
		subscriptionData: &SubscriptionCacheData{
			Status:              SubscriptionStatusActive,
			ExpiresAt:           time.Now().Add(24 * time.Hour),
			WeeklyUsageRequests: 500, // 已达限额
		},
	}
	svc := NewBillingCacheService(cache, nil, nil, nil, &config.Config{})

	user := &User{ID: 1}
	group := &Group{
		ID:                  1,
		SubscriptionType:    SubscriptionTypePerRequest,
		WeeklyLimitRequests: &limit,
		Status:              StatusActive,
	}
	subscription := &UserSubscription{ID: 1, UserID: 1, GroupID: 1}

	err := svc.CheckBillingEligibility(context.Background(), user, nil, group, subscription)
	require.Error(t, err, "per_request exceeding weekly request limit should fail")
	require.ErrorIs(t, err, ErrWeeklyRequestLimitExceeded)
}

func TestCheckBillingEligibility_PerRequest_LimitOnly_MonthlyExceeded(t *testing.T) {
	// 仅次数限制模式，月次数超出应该失败
	limit := int64(1000)
	cache := &perRequestMockCache{
		subscriptionData: &SubscriptionCacheData{
			Status:               SubscriptionStatusActive,
			ExpiresAt:            time.Now().Add(24 * time.Hour),
			MonthlyUsageRequests: 1000, // 已达限额
		},
	}
	svc := NewBillingCacheService(cache, nil, nil, nil, &config.Config{})

	user := &User{ID: 1}
	group := &Group{
		ID:                   1,
		SubscriptionType:     SubscriptionTypePerRequest,
		MonthlyLimitRequests: &limit,
		Status:               StatusActive,
	}
	subscription := &UserSubscription{ID: 1, UserID: 1, GroupID: 1}

	err := svc.CheckBillingEligibility(context.Background(), user, nil, group, subscription)
	require.Error(t, err, "per_request exceeding monthly request limit should fail")
	require.ErrorIs(t, err, ErrMonthlyRequestLimitExceeded)
}

func TestCheckBillingEligibility_PerRequest_PriceAndLimit_Pass(t *testing.T) {
	// 同时配置单价和次数限制，都满足应该通过
	price := 0.01
	dailyLimit := int64(100)
	cache := &perRequestMockCache{
		balance: 1.0,
		subscriptionData: &SubscriptionCacheData{
			Status:             SubscriptionStatusActive,
			ExpiresAt:          time.Now().Add(24 * time.Hour),
			DailyUsageRequests: 50,
		},
	}
	svc := NewBillingCacheService(cache, nil, nil, nil, &config.Config{})

	user := &User{ID: 1}
	group := &Group{
		ID:                 1,
		SubscriptionType:   SubscriptionTypePerRequest,
		PerRequestPrice:    &price,
		DailyLimitRequests: &dailyLimit,
		Status:             StatusActive,
	}
	subscription := &UserSubscription{ID: 1, UserID: 1, GroupID: 1}

	err := svc.CheckBillingEligibility(context.Background(), user, nil, group, subscription)
	require.NoError(t, err, "per_request with price and limit both satisfied should pass")
}

func TestCheckBillingEligibility_PerRequest_PriceAndLimit_InsufficientBalance(t *testing.T) {
	// 同时配置单价和次数限制，余额不足应该失败（价格检查优先）
	price := 0.01
	dailyLimit := int64(100)
	cache := &perRequestMockCache{
		balance: 0.005, // 余额不足
		subscriptionData: &SubscriptionCacheData{
			Status:             SubscriptionStatusActive,
			ExpiresAt:          time.Now().Add(24 * time.Hour),
			DailyUsageRequests: 50,
		},
	}
	svc := NewBillingCacheService(cache, nil, nil, nil, &config.Config{})

	user := &User{ID: 1}
	group := &Group{
		ID:                 1,
		SubscriptionType:   SubscriptionTypePerRequest,
		PerRequestPrice:    &price,
		DailyLimitRequests: &dailyLimit,
		Status:             StatusActive,
	}
	subscription := &UserSubscription{ID: 1, UserID: 1, GroupID: 1}

	err := svc.CheckBillingEligibility(context.Background(), user, nil, group, subscription)
	require.Error(t, err, "per_request with insufficient balance should fail")
	require.ErrorIs(t, err, ErrInsufficientBalance)
}

func TestCheckBillingEligibility_PerRequest_PriceAndLimit_LimitExceeded(t *testing.T) {
	// 同时配置单价和次数限制，次数超出应该失败
	price := 0.01
	dailyLimit := int64(100)
	cache := &perRequestMockCache{
		balance: 1.0,
		subscriptionData: &SubscriptionCacheData{
			Status:             SubscriptionStatusActive,
			ExpiresAt:          time.Now().Add(24 * time.Hour),
			DailyUsageRequests: 100, // 已达限额
		},
	}
	svc := NewBillingCacheService(cache, nil, nil, nil, &config.Config{})

	user := &User{ID: 1}
	group := &Group{
		ID:                 1,
		SubscriptionType:   SubscriptionTypePerRequest,
		PerRequestPrice:    &price,
		DailyLimitRequests: &dailyLimit,
		Status:             StatusActive,
	}
	subscription := &UserSubscription{ID: 1, UserID: 1, GroupID: 1}

	err := svc.CheckBillingEligibility(context.Background(), user, nil, group, subscription)
	require.Error(t, err, "per_request exceeding request limit should fail")
	require.ErrorIs(t, err, ErrDailyRequestLimitExceeded)
}

func TestCheckBillingEligibility_PerRequest_InvalidConfig(t *testing.T) {
	// per_request 模式没有配置单价也没有配置次数限制，应该失败
	cache := &perRequestMockCache{}
	svc := NewBillingCacheService(cache, nil, nil, nil, &config.Config{})

	user := &User{ID: 1}
	group := &Group{
		ID:               1,
		SubscriptionType: SubscriptionTypePerRequest,
		Status:           StatusActive,
		// 无 PerRequestPrice，无 *LimitRequests
	}

	err := svc.CheckBillingEligibility(context.Background(), user, nil, group, nil)
	require.Error(t, err, "per_request with no price and no limit should fail")
	require.ErrorIs(t, err, ErrGroupBillingConfigInvalid)
}

func TestCheckBillingEligibility_PerRequest_ExpiredSubscription(t *testing.T) {
	// 订阅已过期应该失败
	limit := int64(100)
	cache := &perRequestMockCache{
		subscriptionData: &SubscriptionCacheData{
			Status:             SubscriptionStatusActive,
			ExpiresAt:          time.Now().Add(-1 * time.Hour), // 已过期
			DailyUsageRequests: 50,
		},
	}
	svc := NewBillingCacheService(cache, nil, nil, nil, &config.Config{})

	user := &User{ID: 1}
	group := &Group{
		ID:                 1,
		SubscriptionType:   SubscriptionTypePerRequest,
		DailyLimitRequests: &limit,
		Status:             StatusActive,
	}
	subscription := &UserSubscription{ID: 1, UserID: 1, GroupID: 1}

	err := svc.CheckBillingEligibility(context.Background(), user, nil, group, subscription)
	require.Error(t, err, "per_request with expired subscription should fail")
	require.ErrorIs(t, err, ErrSubscriptionInvalid)
}

func TestCheckBillingEligibility_PerRequest_InactiveSubscription(t *testing.T) {
	// 订阅状态非 active 应该失败
	limit := int64(100)
	cache := &perRequestMockCache{
		subscriptionData: &SubscriptionCacheData{
			Status:             SubscriptionStatusExpired, // 非活跃状态
			ExpiresAt:          time.Now().Add(24 * time.Hour),
			DailyUsageRequests: 50,
		},
	}
	svc := NewBillingCacheService(cache, nil, nil, nil, &config.Config{})

	user := &User{ID: 1}
	group := &Group{
		ID:                 1,
		SubscriptionType:   SubscriptionTypePerRequest,
		DailyLimitRequests: &limit,
		Status:             StatusActive,
	}
	subscription := &UserSubscription{ID: 1, UserID: 1, GroupID: 1}

	err := svc.CheckBillingEligibility(context.Background(), user, nil, group, subscription)
	require.Error(t, err, "per_request with inactive subscription should fail")
	require.ErrorIs(t, err, ErrSubscriptionInvalid)
}

func TestQueueIncrementSubscriptionRequestUsage(t *testing.T) {
	cache := &perRequestMockCache{}
	svc := NewBillingCacheService(cache, nil, nil, nil, &config.Config{})
	t.Cleanup(svc.Stop)

	svc.QueueIncrementSubscriptionRequestUsage(1, 2, 5)

	// 等待异步写入完成
	require.Eventually(t, func() bool {
		return atomic.LoadInt64(&cache.incrementReqCalls) > 0
	}, 2*time.Second, 10*time.Millisecond)

	require.Equal(t, int64(1), cache.incrementReqUserID)
	require.Equal(t, int64(2), cache.incrementReqGroupID)
	require.Equal(t, int64(5), cache.incrementReqCount)
}
