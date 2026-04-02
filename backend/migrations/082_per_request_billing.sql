-- 082_per_request_billing.sql
-- 新增按请求次数计费功能

-- 1. groups 表：新增次数计费配置
ALTER TABLE groups ADD COLUMN IF NOT EXISTS per_request_price DECIMAL(20, 10) DEFAULT NULL;
ALTER TABLE groups ADD COLUMN IF NOT EXISTS daily_limit_requests BIGINT DEFAULT NULL;
ALTER TABLE groups ADD COLUMN IF NOT EXISTS weekly_limit_requests BIGINT DEFAULT NULL;
ALTER TABLE groups ADD COLUMN IF NOT EXISTS monthly_limit_requests BIGINT DEFAULT NULL;

-- 2. user_subscriptions 表：新增次数用量追踪
ALTER TABLE user_subscriptions ADD COLUMN IF NOT EXISTS daily_usage_requests BIGINT NOT NULL DEFAULT 0;
ALTER TABLE user_subscriptions ADD COLUMN IF NOT EXISTS weekly_usage_requests BIGINT NOT NULL DEFAULT 0;
ALTER TABLE user_subscriptions ADD COLUMN IF NOT EXISTS monthly_usage_requests BIGINT NOT NULL DEFAULT 0;