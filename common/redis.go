package common

import (
	"context"
	"github.com/redis/go-redis/v9"
	"time"
)

var ctx = context.Background()
var rdb *redis.Client

func init() {
	rdb = redis.NewClient(&redis.Options{
		Addr: "127.0.0.1:6379", // Redis server address
	})
}

func RedisInit(addr string, passwd string) {
	// Initialize Redis client
	rdb = redis.NewClient(&redis.Options{
		Addr:     addr,   // Redis server address
		Password: passwd, // No password set
	})
}

func RedisGet(key string) (string, error) {
	val, err := rdb.Get(ctx, key).Result()
	if err != nil {
		return "", err
	}
	return val, nil
}

func RedisKeys(key string) ([]string, error) {
	return rdb.Keys(ctx, key).Result()
}

func RedisSet(key string, value interface{}, expiration int) (string, error) {
	return rdb.Set(ctx, key, value, time.Duration(expiration)*time.Second).Result()
}

func RedisSetNX(key string, value interface{}, expiration int) (bool, error) {
	return rdb.SetNX(ctx, key, value, time.Duration(expiration)*time.Second).Result()
}

func RedisIncr(key string) (int64, error) {
	return rdb.Incr(ctx, key).Result()
}

func RedisIncrby(key string, value int64) (int64, error) {
	return rdb.IncrBy(ctx, key, value).Result()
}

func RedisDel(key string) error {
	return rdb.Del(ctx, key).Err()
}
