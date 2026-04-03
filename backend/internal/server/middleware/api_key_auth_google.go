package middleware

import (
	"errors"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/googleapi"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// APIKeyAuthGoogle is a Google-style error wrapper for API key auth.
func APIKeyAuthGoogle(apiKeyService *service.APIKeyService, cfg *config.Config) gin.HandlerFunc {
	return APIKeyAuthWithSubscriptionGoogle(apiKeyService, nil, cfg)
}

// APIKeyAuthWithSubscriptionGoogle behaves like ApiKeyAuthWithSubscription but returns Google-style errors:
// {"error":{"code":401,"message":"...","status":"UNAUTHENTICATED"}}
//
// It is intended for Gemini native endpoints (/v1beta) to match Gemini SDK expectations.
func APIKeyAuthWithSubscriptionGoogle(apiKeyService *service.APIKeyService, subscriptionService *service.SubscriptionService, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		if v := strings.TrimSpace(c.Query("api_key")); v != "" {
			abortWithGoogleError(c, 400, "Query parameter api_key is deprecated. Use Authorization header or key instead.")
			return
		}
		apiKeyString := extractAPIKeyForGoogle(c)
		if apiKeyString == "" {
			abortWithGoogleError(c, 401, "API key is required")
			return
		}

		apiKey, err := apiKeyService.GetByKey(c.Request.Context(), apiKeyString)
		if err != nil {
			if errors.Is(err, service.ErrAPIKeyNotFound) {
				abortWithGoogleError(c, 401, "Invalid API key")
				return
			}
			abortWithGoogleError(c, 500, "Failed to validate API key")
			return
		}

		if !apiKey.IsActive() {
			abortWithGoogleError(c, 401, "API key is disabled")
			return
		}
		if apiKey.User == nil {
			abortWithGoogleError(c, 401, "User associated with API key not found")
			return
		}
		if !apiKey.User.IsActive() {
			abortWithGoogleError(c, 401, "User account is not active")
			return
		}

		// 简易模式：跳过余额和订阅检查
		if cfg.RunMode == config.RunModeSimple {
			c.Set(string(ContextKeyAPIKey), apiKey)
			c.Set(string(ContextKeyUser), AuthSubject{
				UserID:      apiKey.User.ID,
				Concurrency: apiKey.User.Concurrency,
			})
			c.Set(string(ContextKeyUserRole), apiKey.User.Role)
			setGroupContext(c, apiKey.Group)
			_ = apiKeyService.TouchLastUsed(c.Request.Context(), apiKey.ID)
			c.Next()
			return
		}

		// 加载订阅（订阅模式或按次计费模式有限额时）
		var subscription *service.UserSubscription
		isSubscriptionType := apiKey.Group != nil && apiKey.Group.IsSubscriptionType()
		isPerRequestWithLimits := apiKey.Group != nil && apiKey.Group.IsPerRequestType() && apiKey.Group.HasAnyRequestLimit()

		if (isSubscriptionType || isPerRequestWithLimits) && subscriptionService != nil {
			sub, subErr := subscriptionService.GetActiveSubscription(
				c.Request.Context(),
				apiKey.User.ID,
				apiKey.Group.ID,
			)
			if subErr != nil {
				abortWithGoogleError(c, 403, "No active subscription found for this group")
				return
			}
			subscription = sub
		}

		// 计费模式检查
		if apiKey.Group != nil && apiKey.Group.IsPerRequestType() {
			// 按次计费模式预检
			hasPrice := apiKey.Group.HasPerRequestPrice()
			hasLimit := apiKey.Group.HasAnyRequestLimit()

			if !hasPrice && !hasLimit {
				abortWithGoogleError(c, 403, "per_request group must have either per_request_price or at least one request limit configured")
				return
			}

			// 1. 余额检查（如果有单价）
			if hasPrice {
				if apiKey.User.Balance < *apiKey.Group.PerRequestPrice {
					abortWithGoogleError(c, 403, "Insufficient account balance")
					return
				}
			}

			// 2. 次数限额检查（如果有限额配置且有订阅）
			if hasLimit {
				if subscription == nil {
					abortWithGoogleError(c, 403, "subscription is required for per_request mode with request limits")
					return
				}
				needsMaintenance, validateErr := subscriptionService.ValidateAndCheckLimits(subscription, apiKey.Group)
				if validateErr != nil {
					status := mapBillingErrorToStatus(validateErr)
					abortWithGoogleError(c, status, validateErr.Error())
					return
				}

				c.Set(string(ContextKeySubscription), subscription)

				if needsMaintenance {
					maintenanceCopy := *subscription
					subscriptionService.DoWindowMaintenance(&maintenanceCopy)
				}
			}
		} else if subscription != nil {
			// 订阅模式
			needsMaintenance, err := subscriptionService.ValidateAndCheckLimits(subscription, apiKey.Group)
			if err != nil {
				status := mapBillingErrorToStatus(err)
				abortWithGoogleError(c, status, err.Error())
				return
			}

			c.Set(string(ContextKeySubscription), subscription)

			if needsMaintenance {
				maintenanceCopy := *subscription
				subscriptionService.DoWindowMaintenance(&maintenanceCopy)
			}
		} else {
			// 余额模式
			if apiKey.User.Balance <= 0 {
				abortWithGoogleError(c, 403, "Insufficient account balance")
				return
			}
		}

		c.Set(string(ContextKeyAPIKey), apiKey)
		c.Set(string(ContextKeyUser), AuthSubject{
			UserID:      apiKey.User.ID,
			Concurrency: apiKey.User.Concurrency,
		})
		c.Set(string(ContextKeyUserRole), apiKey.User.Role)
		setGroupContext(c, apiKey.Group)
		_ = apiKeyService.TouchLastUsed(c.Request.Context(), apiKey.ID)
		c.Next()
	}
}

// mapBillingErrorToStatus 映射计费错误到 HTTP 状态码
// 统一处理 USD 限额错误和请求次数限额错误
func mapBillingErrorToStatus(err error) int {
	var reqLimitErr *service.RequestLimitExceededError

	switch {
	// USD 限额错误
	case errors.Is(err, service.ErrDailyLimitExceeded),
		errors.Is(err, service.ErrWeeklyLimitExceeded),
		errors.Is(err, service.ErrMonthlyLimitExceeded),
		// 请求次数限额错误
		errors.Is(err, service.ErrDailyRequestLimitExceeded),
		errors.Is(err, service.ErrWeeklyRequestLimitExceeded),
		errors.Is(err, service.ErrMonthlyRequestLimitExceeded),
		// 结构化请求次数错误
		errors.As(err, &reqLimitErr):
		return 429
	default:
		return 403
	}
}

// mapPerRequestErrorToStatus 映射按次计费错误到 HTTP 状态码
// Deprecated: 使用 mapBillingErrorToStatus 代替
func mapPerRequestErrorToStatus(err error) int {
	return mapBillingErrorToStatus(err)
}

// extractAPIKeyForGoogle extracts API key for Google/Gemini endpoints.
// Priority: x-goog-api-key > Authorization: Bearer > x-api-key > query key
// This allows OpenClaw and other clients using Bearer auth to work with Gemini endpoints.
func extractAPIKeyForGoogle(c *gin.Context) string {
	// 1) preferred: Gemini native header
	if k := strings.TrimSpace(c.GetHeader("x-goog-api-key")); k != "" {
		return k
	}

	// 2) fallback: Authorization: Bearer <key>
	auth := strings.TrimSpace(c.GetHeader("Authorization"))
	if auth != "" {
		parts := strings.SplitN(auth, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			if k := strings.TrimSpace(parts[1]); k != "" {
				return k
			}
		}
	}

	// 3) x-api-key header (backward compatibility)
	if k := strings.TrimSpace(c.GetHeader("x-api-key")); k != "" {
		return k
	}

	// 4) query parameter key (for specific paths)
	if allowGoogleQueryKey(c.Request.URL.Path) {
		if v := strings.TrimSpace(c.Query("key")); v != "" {
			return v
		}
	}

	return ""
}

func allowGoogleQueryKey(path string) bool {
	return strings.HasPrefix(path, "/v1beta") || strings.HasPrefix(path, "/antigravity/v1beta")
}

func abortWithGoogleError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"code":    status,
			"message": message,
			"status":  googleapi.HTTPStatusToGoogleStatus(status),
		},
	})
	c.Abort()
}