package cache

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
)

type RedisCache struct {
	client     *redis.Client
	defaultTTL time.Duration
	permanent  bool
}

// NewRedisCache 创建一个新的Redis缓存
func NewRedisCache(opts RedisCacheOptions) *RedisCache {
	redisOptions := &redis.Options{
		Addr:     fmt.Sprintf("%s:%d", opts.Host, opts.Port),
		Password: opts.Password,
		DB:       opts.DB,
	}
	if opts.TLS {
		redisOptions.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}

	client := redis.NewClient(redisOptions)

	// 设置默认TTL
	defaultTTL := opts.DefaultTTL
	if defaultTTL <= 0 && !opts.Permanent {
		defaultTTL = 24 * time.Hour // 默认1天
	}

	return &RedisCache{
		client:     client,
		defaultTTL: defaultTTL,
		permanent:  opts.Permanent,
	}
}

// Get 从缓存中获取值
func (c *RedisCache) Get(ctx context.Context, key string) (string, error) {
	val, err := c.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", ErrCacheMiss
	}
	return val, err
}

// Set 将值存入缓存
func (c *RedisCache) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	var expiration time.Duration

	// 处理过期时间
	if ttl < 0 || c.permanent {
		// 永久存储，使用Redis的0表示永不过期
		expiration = 0
	} else if ttl == 0 {
		// 使用默认过期时间
		if c.defaultTTL > 0 {
			expiration = c.defaultTTL
		} else {
			expiration = 0 // 默认过期时间为0也表示永久
		}
	} else {
		// 使用指定的过期时间
		expiration = ttl
	}

	return c.client.Set(ctx, key, value, expiration).Err()
}

// Clear 清空缓存
func (c *RedisCache) Clear(ctx context.Context) error {
	return c.client.FlushDB(ctx).Err()
}

// Close 关闭Redis连接
func (c *RedisCache) Close(ctx context.Context) error {
	return c.client.Close()
}
