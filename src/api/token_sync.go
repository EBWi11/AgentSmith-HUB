package api

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	tokenRedisKey     = "cluster:leader:token"
	llmAPIKeyRedisKey = "cluster:hub_config:llm_api_key"
	llmBaseURLRedisKey = "cluster:hub_config:llm_base_url"
	llmModelRedisKey   = "cluster:hub_config:llm_model"
)

// WriteTokenToRedis writes the token to Redis (called by leader on startup)
func WriteTokenToRedis(token string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := common.GetRedisClient().Set(ctx, tokenRedisKey, token, 0).Err() // No expiration
	if err != nil {
		logger.Error("Failed to write token to Redis", "error", err)
		return err
	}

	logger.Info("Token written to Redis successfully")
	return nil
}

// WriteLeaderNodeIDToRedis writes the leader's node ID (IP/host) to Redis so followers and plugins can reach the leader API.
// Called by leader on startup.
func WriteLeaderNodeIDToRedis(nodeID string) error {
	if nodeID == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := common.GetRedisClient().Set(ctx, common.LeaderNodeIDRedisKey, nodeID, 0).Err()
	if err != nil {
		logger.Error("Failed to write leader node ID to Redis", "error", err)
		return err
	}
	logger.Info("Leader node ID written to Redis", "node_id", nodeID)
	return nil
}

// ReadTokenFromRedis reads the token from Redis (called by follower on startup)
func ReadTokenFromRedis() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	token, err := common.GetRedisClient().Get(ctx, tokenRedisKey).Result()
	if err != nil {
		logger.Error("Failed to read token from Redis", "error", err)
		return "", err
	}

	logger.Info("Token read from Redis successfully")
	return token, nil
}

// WriteLLMConfigToRedis writes LLM API key/base URL/model to Redis (called by leader when config is set).
// Ensures all nodes in the cluster use the same LLM config.
func WriteLLMConfigToRedis(apiKey, baseURL, model string) error {
	if apiKey == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := common.GetRedisClient()
	if err := client.Set(ctx, llmAPIKeyRedisKey, apiKey, 0).Err(); err != nil {
		logger.Error("Failed to write LLM API key to Redis", "error", err)
		return err
	}
	if baseURL != "" {
		if err := client.Set(ctx, llmBaseURLRedisKey, baseURL, 0).Err(); err != nil {
			logger.Error("Failed to write LLM base URL to Redis", "error", err)
			return err
		}
	}
	if model != "" {
		if err := client.Set(ctx, llmModelRedisKey, model, 0).Err(); err != nil {
			logger.Error("Failed to write LLM model to Redis", "error", err)
			return err
		}
	}
	logger.Info("LLM config written to Redis successfully")
	return nil
}

// ReadLLMConfigFromRedis reads LLM API key/base URL/model from Redis (called by followers).
// Overrides local config so cluster-wide LLM config is consistent.
func ReadLLMConfigFromRedis() (apiKey, baseURL, model string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := common.GetRedisClient()
	apiKey, err = client.Get(ctx, llmAPIKeyRedisKey).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", "", "", nil
		}
		return "", "", "", err
	}
	baseURL, _ = client.Get(ctx, llmBaseURLRedisKey).Result()
	model, _ = client.Get(ctx, llmModelRedisKey).Result()
	return apiKey, baseURL, model, nil
}
