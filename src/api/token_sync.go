package api

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"context"
	"time"
)

const (
	tokenRedisKey = "cluster:leader:token"
)

// WriteTokenToRedis writes the token to Redis (called by leader on startup)
func WriteTokenToRedis(token string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := common.GetRedisClient().Set(ctx, tokenRedisKey, token, 0).Err() // No expiration
	if err != nil {
		logger.Error("Failed to write token to Redis: %v", err)
		return err
	}

	logger.Info("Token written to Redis successfully")
	return nil
}

// ReadTokenFromRedis reads the token from Redis (called by follower on startup)
func ReadTokenFromRedis() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	token, err := common.GetRedisClient().Get(ctx, tokenRedisKey).Result()
	if err != nil {
		logger.Error("Failed to read token from Redis: %v", err)
		return "", err
	}

	logger.Info("Token read from Redis successfully")
	return token, nil
}
