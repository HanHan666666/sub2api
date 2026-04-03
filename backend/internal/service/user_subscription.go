package service

import (
	"fmt"
	"time"
)

type UserSubscription struct {
	ID      int64
	UserID  int64
	GroupID int64

	StartsAt  time.Time
	ExpiresAt time.Time
	Status    string

	DailyWindowStart   *time.Time
	WeeklyWindowStart  *time.Time
	MonthlyWindowStart *time.Time

	DailyUsageUSD   float64
	WeeklyUsageUSD  float64
	MonthlyUsageUSD float64

	// 按次用量追踪（新增）
	DailyUsageRequests   int64 // 当前日窗口已用请求次数
	WeeklyUsageRequests  int64 // 当前周窗口已用请求次数
	MonthlyUsageRequests int64 // 当前月窗口已用请求次数

	AssignedBy *int64
	AssignedAt time.Time
	Notes      string

	CreatedAt time.Time
	UpdatedAt time.Time

	User           *User
	Group          *Group
	AssignedByUser *User
}

func (s *UserSubscription) IsActive() bool {
	return s.Status == SubscriptionStatusActive && time.Now().Before(s.ExpiresAt)
}

func (s *UserSubscription) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

func (s *UserSubscription) DaysRemaining() int {
	if s.IsExpired() {
		return 0
	}
	return int(time.Until(s.ExpiresAt).Hours() / 24)
}

func (s *UserSubscription) IsWindowActivated() bool {
	return s.DailyWindowStart != nil || s.WeeklyWindowStart != nil || s.MonthlyWindowStart != nil
}

func (s *UserSubscription) NeedsDailyReset() bool {
	if s.DailyWindowStart == nil {
		return false
	}
	return time.Since(*s.DailyWindowStart) >= 24*time.Hour
}

func (s *UserSubscription) NeedsWeeklyReset() bool {
	if s.WeeklyWindowStart == nil {
		return false
	}
	return time.Since(*s.WeeklyWindowStart) >= 7*24*time.Hour
}

func (s *UserSubscription) NeedsMonthlyReset() bool {
	if s.MonthlyWindowStart == nil {
		return false
	}
	return time.Since(*s.MonthlyWindowStart) >= 30*24*time.Hour
}

func (s *UserSubscription) DailyResetTime() *time.Time {
	if s.DailyWindowStart == nil {
		return nil
	}
	t := s.DailyWindowStart.Add(24 * time.Hour)
	return &t
}

func (s *UserSubscription) WeeklyResetTime() *time.Time {
	if s.WeeklyWindowStart == nil {
		return nil
	}
	t := s.WeeklyWindowStart.Add(7 * 24 * time.Hour)
	return &t
}

func (s *UserSubscription) MonthlyResetTime() *time.Time {
	if s.MonthlyWindowStart == nil {
		return nil
	}
	t := s.MonthlyWindowStart.Add(30 * 24 * time.Hour)
	return &t
}

func (s *UserSubscription) CheckDailyLimit(group *Group, additionalCost float64) bool {
	if !group.HasDailyLimit() {
		return true
	}
	return s.DailyUsageUSD+additionalCost <= *group.DailyLimitUSD
}

func (s *UserSubscription) CheckWeeklyLimit(group *Group, additionalCost float64) bool {
	if !group.HasWeeklyLimit() {
		return true
	}
	return s.WeeklyUsageUSD+additionalCost <= *group.WeeklyLimitUSD
}

func (s *UserSubscription) CheckMonthlyLimit(group *Group, additionalCost float64) bool {
	if !group.HasMonthlyLimit() {
		return true
	}
	return s.MonthlyUsageUSD+additionalCost <= *group.MonthlyLimitUSD
}

func (s *UserSubscription) CheckAllLimits(group *Group, additionalCost float64) (daily, weekly, monthly bool) {
	daily = s.CheckDailyLimit(group, additionalCost)
	weekly = s.CheckWeeklyLimit(group, additionalCost)
	monthly = s.CheckMonthlyLimit(group, additionalCost)
	return
}

// GetDailyResetInSeconds 返回日窗口重置剩余秒数
func (s *UserSubscription) GetDailyResetInSeconds() int64 {
	if s.DailyWindowStart == nil {
		return 0
	}
	resetTime := s.DailyWindowStart.Add(24 * time.Hour)
	remaining := time.Until(resetTime).Seconds()
	if remaining < 0 {
		return 0
	}
	return int64(remaining)
}

// GetWeeklyResetInSeconds 返回周窗口重置剩余秒数
func (s *UserSubscription) GetWeeklyResetInSeconds() int64 {
	if s.WeeklyWindowStart == nil {
		return 0
	}
	resetTime := s.WeeklyWindowStart.Add(7 * 24 * time.Hour)
	remaining := time.Until(resetTime).Seconds()
	if remaining < 0 {
		return 0
	}
	return int64(remaining)
}

// GetMonthlyResetInSeconds 返回月窗口重置剩余秒数
func (s *UserSubscription) GetMonthlyResetInSeconds() int64 {
	if s.MonthlyWindowStart == nil {
		return 0
	}
	resetTime := s.MonthlyWindowStart.Add(30 * 24 * time.Hour)
	remaining := time.Until(resetTime).Seconds()
	if remaining < 0 {
		return 0
	}
	return int64(remaining)
}

// CheckDailyLimitRequests 检查是否超过每日请求次数限额
// additionalRequests 通常为 1（一次请求）
func (s *UserSubscription) CheckDailyLimitRequests(group *Group, additionalRequests int64) error {
	if !group.HasDailyLimitRequests() {
		return nil // 无限制
	}
	if s.DailyUsageRequests+additionalRequests > *group.DailyLimitRequests {
		return NewRequestLimitExceededError(
			"daily",
			s.DailyUsageRequests+additionalRequests,
			*group.DailyLimitRequests,
			s.GetDailyResetInSeconds(),
		)
	}
	return nil
}

// CheckWeeklyLimitRequests 检查是否超过每周请求次数限额
func (s *UserSubscription) CheckWeeklyLimitRequests(group *Group, additionalRequests int64) error {
	if !group.HasWeeklyLimitRequests() {
		return nil // 无限制
	}
	if s.WeeklyUsageRequests+additionalRequests > *group.WeeklyLimitRequests {
		return NewRequestLimitExceededError(
			"weekly",
			s.WeeklyUsageRequests+additionalRequests,
			*group.WeeklyLimitRequests,
			s.GetWeeklyResetInSeconds(),
		)
	}
	return nil
}

// CheckMonthlyLimitRequests 检查是否超过每月请求次数限额
func (s *UserSubscription) CheckMonthlyLimitRequests(group *Group, additionalRequests int64) error {
	if !group.HasMonthlyLimitRequests() {
		return nil // 无限制
	}
	if s.MonthlyUsageRequests+additionalRequests > *group.MonthlyLimitRequests {
		return NewRequestLimitExceededError(
			"monthly",
			s.MonthlyUsageRequests+additionalRequests,
			*group.MonthlyLimitRequests,
			s.GetMonthlyResetInSeconds(),
		)
	}
	return nil
}

// RequestLimitExceededError 用于向 API 层传递结构化限额信息
type RequestLimitExceededError struct {
	Window         string
	Used           int64
	Limit          int64
	ResetInSeconds int64
}

func (e *RequestLimitExceededError) Error() string {
	return fmt.Sprintf("%s request limit exceeded (%d/%d)", e.Window, e.Used, e.Limit)
}

func NewRequestLimitExceededError(window string, used, limit, resetInSeconds int64) error {
	return &RequestLimitExceededError{
		Window:         window,
		Used:           used,
		Limit:          limit,
		ResetInSeconds: resetInSeconds,
	}
}

func (e *RequestLimitExceededError) Code() string {
	switch e.Window {
	case "daily":
		return "DAILY_REQUEST_LIMIT_EXCEEDED"
	case "weekly":
		return "WEEKLY_REQUEST_LIMIT_EXCEEDED"
	case "monthly":
		return "MONTHLY_REQUEST_LIMIT_EXCEEDED"
	default:
		return "REQUEST_LIMIT_EXCEEDED"
	}
}
