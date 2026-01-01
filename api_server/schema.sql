-- MySQL schema for tiktok_play_api
-- 默认库名：tiktok_go_play
--
-- 说明：
-- 1) api_keys：存放 API_KEY 与播放额度（credit）
-- 2) orders：下单记录（action=add），会在创建订单时扣减 credit

CREATE TABLE IF NOT EXISTS api_keys (
  api_key VARCHAR(128) NOT NULL COMMENT 'Client API key (action requests use this)',
  merchant_name VARCHAR(128) NOT NULL DEFAULT '' COMMENT 'Merchant name',
  is_active TINYINT(1) NOT NULL DEFAULT 1 COMMENT '1=active,0=disabled',
  credit BIGINT NOT NULL DEFAULT 0 COMMENT 'Play credits (quantity deducted on add)',
  total_credit BIGINT NOT NULL DEFAULT 0 COMMENT 'Total credited amount (lifetime, not deducted on add)',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT 'Create time',
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT 'Update time',
  PRIMARY KEY (api_key),
  KEY idx_is_active (is_active) COMMENT 'Quick filter active keys'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='API keys + play credits';

CREATE TABLE IF NOT EXISTS orders (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT 'Order ID',
  order_id CHAR(36) NOT NULL COMMENT 'External order id (UUID), returned to client',
  api_key VARCHAR(128) NOT NULL COMMENT 'Which API key created this order',
  aweme_id VARCHAR(64) NOT NULL COMMENT 'Parsed aweme/video id',
  link TEXT NOT NULL COMMENT 'Original tiktok link',
  quantity BIGINT NOT NULL COMMENT 'Requested quantity',
  delivered BIGINT NOT NULL DEFAULT 0 COMMENT 'Delivered quantity (worker updates)',
  start_count BIGINT NOT NULL DEFAULT 0 COMMENT 'Start view count snapshot',
  status VARCHAR(32) NOT NULL DEFAULT 'Pending' COMMENT 'Pending/In progress/Completed/Partial/Canceled',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT 'Create time',
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT 'Update time',
  PRIMARY KEY (id),
  UNIQUE KEY uk_order_id (order_id) COMMENT 'Unique external order id',
  KEY idx_api_key (api_key) COMMENT 'List orders by API key',
  KEY idx_aweme_id (aweme_id) COMMENT 'Search orders by aweme id',
  KEY idx_status (status) COMMENT 'Filter by status',
  KEY idx_created_at (created_at) COMMENT 'Time range queries'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Orders created via /api?action=add';

-- cookies 池（MySQL 版）：替代 Redis startup_cookie_pool
-- signup(dgemail) 写入，stats(dgmain3) 读取（device + cookies 同源：account_json 内含 cookies 字段）
CREATE TABLE IF NOT EXISTS startup_cookie_accounts (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT 'Auto sequence',
  shard_id INT NOT NULL DEFAULT 0 COMMENT 'Shard index (0..N-1)',
  device_key VARCHAR(128) NOT NULL COMMENT 'Unique key (prefer device_id; fallback cdid)',
  account_json MEDIUMTEXT NOT NULL COMMENT 'Account JSON (device fields + cookies + create_time)',
  use_count BIGINT NOT NULL DEFAULT 0 COMMENT 'use_count for rotation/evict',
  fail_count BIGINT NOT NULL DEFAULT 0 COMMENT 'fail_count (optional)',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_device_key (device_key),
  KEY idx_shard_id_id (shard_id, id),
  KEY idx_shard_use (shard_id, use_count)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Startup cookie accounts (DB backend)';

-- 设备池（MySQL 版）：替代 Redis device_pool（用于 mwzzzh_spider 写入，Go signup/stats 读取）
-- 分库/分片策略：
-- - shard_id：由写入端按 device_id hash%N 计算（N=DB_DEVICE_POOL_SHARDS）
-- - 读端可指定 shard_id（多进程/多机并行消费时用）
CREATE TABLE IF NOT EXISTS device_pool_devices (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT 'Auto sequence (FIFO/order)',
  shard_id INT NOT NULL DEFAULT 0 COMMENT 'Shard index (0..N-1)',
  device_id VARCHAR(128) NOT NULL COMMENT 'Unique device id (cdid/device_id/...)',
  device_json MEDIUMTEXT NOT NULL COMMENT 'Raw device JSON',
  device_create_time TIMESTAMP NULL DEFAULT NULL COMMENT 'Device registration time (extracted from device_json.create_time)',
  use_count BIGINT NOT NULL DEFAULT 0 COMMENT 'use_count (compat with redis :use)',
  fail_count BIGINT NOT NULL DEFAULT 0 COMMENT 'fail_count (compat with redis :fail)',
  play_count BIGINT NOT NULL DEFAULT 0 COMMENT 'play_count (compat with redis :play)',
  attempt_count BIGINT NOT NULL DEFAULT 0 COMMENT 'attempt_count (compat with redis :attempt)',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT 'Insert time',
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT 'Update time',
  PRIMARY KEY (id),
  UNIQUE KEY uk_device_id (device_id),
  KEY idx_shard_id_id (shard_id, id),
  KEY idx_shard_use (shard_id, use_count),
  KEY idx_shard_play (shard_id, play_count),
  KEY idx_shard_attempt (shard_id, attempt_count),
  KEY idx_shard_create_time (shard_id, device_create_time) COMMENT 'Filter devices by age'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Device pool devices (DB backend)';

-- cookies 池（MySQL 版）：signup(dgemail) 注册成功后写入，stats 从这里读取（device+cookies 同源）
CREATE TABLE IF NOT EXISTS startup_cookie_accounts (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  shard_id INT NOT NULL DEFAULT 0,
  device_key VARCHAR(128) NOT NULL COMMENT 'device_id (preferred) or cdid (fallback)',
  account_json MEDIUMTEXT NOT NULL COMMENT 'Full account JSON including cookies field',
  device_create_time TIMESTAMP NULL DEFAULT NULL COMMENT 'Device registration time (extracted from account_json.create_time)',
  use_count BIGINT NOT NULL DEFAULT 0,
  fail_count BIGINT NOT NULL DEFAULT 0,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_device_key (device_key),
  KEY idx_shard_id_id (shard_id, id),
  KEY idx_shard_use (shard_id, use_count),
  KEY idx_shard_create_time (shard_id, device_create_time)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Startup cookie accounts (DB backend)';

-- 通用计数器（用于 dgemail 生成跨进程不重复邮箱序号等）
CREATE TABLE IF NOT EXISTS counters (
  name VARCHAR(128) NOT NULL,
  val BIGINT NOT NULL DEFAULT 0,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Simple counters for sequences';


