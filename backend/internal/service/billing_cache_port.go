package service

import (
	"time"
)

// SubscriptionCacheData represents cached subscription data
type SubscriptionCacheData struct {
	Status               string
	ExpiresAt            time.Time
	DailyUsage           float64
	WeeklyUsage          float64
	MonthlyUsage         float64
	DailyUsageRequests   int64 // 按次计费：日请求次数用量
	WeeklyUsageRequests  int64 // 按次计费：周请求次数用量
	MonthlyUsageRequests int64 // 按次计费：月请求次数用量
	Version              int64
}
