package cache

import "time"

// MemoryCacheOptions 内存缓存选项
type MemoryCacheOptions struct {
	MaxSize    int           // 最大缓存条目数
	DefaultTTL time.Duration // 默认过期时间
	Permanent  bool          // 是否永久存储
}

// RedisCacheOptions Redis缓存选项
type RedisCacheOptions struct {
	Host       string
	Port       int
	Password   string
	DB         int
	TLS        bool
	DefaultTTL time.Duration // 默认过期时间
	Permanent  bool          // 是否永久存储
}
