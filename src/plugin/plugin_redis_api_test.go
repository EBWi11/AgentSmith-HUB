package plugin

import (
	"fmt"
	"strings"
	"testing"
)

func TestPluginRejectsDirectCommonImport(t *testing.T) {
	raw := `package plugin

import "AgentSmith-HUB/common"

func Eval(key string) (interface{}, bool, error) {
	value, err := common.RedisGet(key)
	return value, err == nil, err
}`

	err := Verify("", raw, "direct-common")
	if err == nil {
		t.Fatal("expected direct common import to be rejected")
	}
	if !strings.Contains(err.Error(), "AgentSmith-HUB/common") {
		t.Fatalf("expected error to mention rejected import, got %v", err)
	}
}

func TestYaegiPluginCanUseScopedHubRedisAPI(t *testing.T) {
	store := make(map[string]string)
	deletedKeys := make([]string, 0)
	calls := make(map[string][]string)
	recordCall := func(op, key string) {
		calls[op] = append(calls[op], key)
	}

	oldGet := pluginRedisGet
	oldSet := pluginRedisSet
	oldSetNX := pluginRedisSetNX
	oldIncrBy := pluginRedisIncrBy
	oldDel := pluginRedisDel
	oldExpire := pluginRedisExpire
	oldHSet := pluginRedisHSet
	oldHGet := pluginRedisHGet
	oldHGetAll := pluginRedisHGetAll
	oldHDel := pluginRedisHDel
	oldLPush := pluginRedisLPush
	oldLRange := pluginRedisLRange
	oldSAdd := pluginRedisSAdd
	oldSRem := pluginRedisSRem
	oldSMembers := pluginRedisSMembers
	oldZAdd := pluginRedisZAdd
	oldZRevRange := pluginRedisZRevRange
	oldZRemRangeByRank := pluginRedisZRemRangeByRank
	oldZRemRangeByScore := pluginRedisZRemRangeByScore
	t.Cleanup(func() {
		pluginRedisGet = oldGet
		pluginRedisSet = oldSet
		pluginRedisSetNX = oldSetNX
		pluginRedisIncrBy = oldIncrBy
		pluginRedisDel = oldDel
		pluginRedisExpire = oldExpire
		pluginRedisHSet = oldHSet
		pluginRedisHGet = oldHGet
		pluginRedisHGetAll = oldHGetAll
		pluginRedisHDel = oldHDel
		pluginRedisLPush = oldLPush
		pluginRedisLRange = oldLRange
		pluginRedisSAdd = oldSAdd
		pluginRedisSRem = oldSRem
		pluginRedisSMembers = oldSMembers
		pluginRedisZAdd = oldZAdd
		pluginRedisZRevRange = oldZRevRange
		pluginRedisZRemRangeByRank = oldZRemRangeByRank
		pluginRedisZRemRangeByScore = oldZRemRangeByScore
	})

	pluginRedisGet = func(key string) (string, error) {
		recordCall("Get", key)
		value, ok := store[key]
		if !ok {
			return "", fmt.Errorf("missing key %s", key)
		}
		return value, nil
	}
	pluginRedisSet = func(key string, value interface{}, expiration int) (string, error) {
		recordCall("Set", key)
		store[key] = fmt.Sprintf("%v", value)
		return "OK", nil
	}
	pluginRedisSetNX = func(key string, value interface{}, expiration int) (bool, error) {
		recordCall("SetNX", key)
		if _, exists := store[key]; exists {
			return false, nil
		}
		store[key] = fmt.Sprintf("%v", value)
		return true, nil
	}
	pluginRedisIncrBy = func(key string, value int64) (int64, error) {
		recordCall("IncrBy", key)
		return value, nil
	}
	pluginRedisDel = func(key string) error {
		recordCall("Del", key)
		deletedKeys = append(deletedKeys, key)
		delete(store, key)
		return nil
	}
	pluginRedisExpire = func(key string, expiration int) error {
		recordCall("Expire", key)
		return nil
	}
	pluginRedisHSet = func(hash string, field string, value interface{}) error {
		recordCall("HSet", hash)
		return nil
	}
	pluginRedisHGet = func(hash string, field string) (string, error) {
		recordCall("HGet", hash)
		return "hash-value", nil
	}
	pluginRedisHGetAll = func(hash string) (map[string]string, error) {
		recordCall("HGetAll", hash)
		return map[string]string{"field": "hash-value"}, nil
	}
	pluginRedisHDel = func(key string, field string) error {
		recordCall("HDel", key)
		return nil
	}
	pluginRedisLPush = func(key string, value interface{}, maxLen int64) error {
		recordCall("LPush", key)
		return nil
	}
	pluginRedisLRange = func(key string, start, stop int64) ([]string, error) {
		recordCall("LRange", key)
		return []string{"list-value"}, nil
	}
	pluginRedisSAdd = func(key string, member interface{}) (int64, error) {
		recordCall("SAdd", key)
		return 1, nil
	}
	pluginRedisSRem = func(key string, member interface{}) (int64, error) {
		recordCall("SRem", key)
		return 1, nil
	}
	pluginRedisSMembers = func(key string) ([]string, error) {
		recordCall("SMembers", key)
		return []string{"set-value"}, nil
	}
	pluginRedisZAdd = func(key string, score float64, member interface{}) (int64, error) {
		recordCall("ZAdd", key)
		return 1, nil
	}
	pluginRedisZRevRange = func(key string, start, stop int64) ([]string, error) {
		recordCall("ZRevRange", key)
		return []string{"zset-value"}, nil
	}
	pluginRedisZRemRangeByRank = func(key string, start, stop int64) (int64, error) {
		recordCall("ZRemRangeByRank", key)
		return 1, nil
	}
	pluginRedisZRemRangeByScore = func(key string, min, max string) (int64, error) {
		recordCall("ZRemRangeByScore", key)
		return 1, nil
	}

	raw := `package plugin

import (
	"fmt"
	hubredis "AgentSmith-HUB/pluginapi/redis"
)

func Eval(key string) (interface{}, bool, error) {
	ok, err := hubredis.SetNX(key, "first", 30)
	if err != nil {
		return nil, false, err
	}
	value, err := hubredis.Get(key)
	if err != nil {
		return nil, false, err
	}
	if _, err := hubredis.Set("summary", fmt.Sprintf("%v:%s", ok, value), 5); err != nil {
		return nil, false, err
	}
	if err := hubredis.Del("cleanup"); err != nil {
		return nil, false, err
	}
	if _, err := hubredis.Incr("counter"); err != nil {
		return nil, false, err
	}
	if _, err := hubredis.IncrBy("counter", int64(4)); err != nil {
		return nil, false, err
	}
	if err := hubredis.Expire("counter", 60); err != nil {
		return nil, false, err
	}
	if err := hubredis.HSet("hash", "field", "value"); err != nil {
		return nil, false, err
	}
	if _, err := hubredis.HGet("hash", "field"); err != nil {
		return nil, false, err
	}
	if _, err := hubredis.HGetAll("hash"); err != nil {
		return nil, false, err
	}
	if err := hubredis.HDel("hash", "field"); err != nil {
		return nil, false, err
	}
	if err := hubredis.LPush("list", "value", int64(10)); err != nil {
		return nil, false, err
	}
	if _, err := hubredis.LRange("list", int64(0), int64(-1)); err != nil {
		return nil, false, err
	}
	if _, err := hubredis.SAdd("set", "member"); err != nil {
		return nil, false, err
	}
	if _, err := hubredis.SRem("set", "member"); err != nil {
		return nil, false, err
	}
	if _, err := hubredis.SMembers("set"); err != nil {
		return nil, false, err
	}
	if _, err := hubredis.ZAdd("zset", 1.5, "member"); err != nil {
		return nil, false, err
	}
	if _, err := hubredis.ZRevRange("zset", int64(0), int64(-1)); err != nil {
		return nil, false, err
	}
	if _, err := hubredis.ZRemRangeByRank("zset", int64(0), int64(1)); err != nil {
		return nil, false, err
	}
	if _, err := hubredis.ZRemRangeByScore("zset", "-inf", "+inf"); err != nil {
		return nil, false, err
	}
	return value, ok, nil
}`

	p, err := NewTestPlugin("", raw, "redis-demo", YAEGI_PLUGIN)
	if err != nil {
		t.Fatalf("expected plugin to load with redis API import: %v", err)
	}

	result, success, err := p.FuncEvalOther("dedupe")
	if err != nil {
		t.Fatalf("expected plugin redis API call to succeed: %v", err)
	}
	if !success {
		t.Fatal("expected SetNX success")
	}
	if result != "first" {
		t.Fatalf("expected result from scoped Redis key, got %#v", result)
	}
	if got := store["plugin:redis-demo:dedupe"]; got != "first" {
		t.Fatalf("expected scoped dedupe key to be set, got %q", got)
	}
	if got := store["plugin:redis-demo:summary"]; got != "true:first" {
		t.Fatalf("expected scoped summary key to be set, got %q", got)
	}
	if len(deletedKeys) != 1 || deletedKeys[0] != "plugin:redis-demo:cleanup" {
		t.Fatalf("expected scoped delete key, got %#v", deletedKeys)
	}
	if _, exists := store["dedupe"]; exists {
		t.Fatal("unexpected unscoped Redis key")
	}

	expectedCalls := map[string]string{
		"Get":              "plugin:redis-demo:dedupe",
		"Set":              "plugin:redis-demo:summary",
		"SetNX":            "plugin:redis-demo:dedupe",
		"Del":              "plugin:redis-demo:cleanup",
		"Expire":           "plugin:redis-demo:counter",
		"HSet":             "plugin:redis-demo:hash",
		"HGet":             "plugin:redis-demo:hash",
		"HGetAll":          "plugin:redis-demo:hash",
		"HDel":             "plugin:redis-demo:hash",
		"LPush":            "plugin:redis-demo:list",
		"LRange":           "plugin:redis-demo:list",
		"SAdd":             "plugin:redis-demo:set",
		"SRem":             "plugin:redis-demo:set",
		"SMembers":         "plugin:redis-demo:set",
		"ZAdd":             "plugin:redis-demo:zset",
		"ZRevRange":        "plugin:redis-demo:zset",
		"ZRemRangeByRank":  "plugin:redis-demo:zset",
		"ZRemRangeByScore": "plugin:redis-demo:zset",
	}
	for op, expectedKey := range expectedCalls {
		if len(calls[op]) == 0 {
			t.Fatalf("expected %s to be called", op)
		}
		for _, key := range calls[op] {
			if key != expectedKey {
				t.Fatalf("expected %s to use scoped key %q, got %q", op, expectedKey, key)
			}
		}
	}
	if got := calls["IncrBy"]; len(got) != 2 || got[0] != "plugin:redis-demo:counter" || got[1] != "plugin:redis-demo:counter" {
		t.Fatalf("expected Incr and IncrBy to use scoped counter key, got %#v", got)
	}
}

func TestYaegiPluginRedisAPIRejectsEmptyKey(t *testing.T) {
	raw := `package plugin

import hubredis "AgentSmith-HUB/pluginapi/redis"

func Eval() (interface{}, bool, error) {
	value, err := hubredis.Get("")
	return value, err == nil, err
}`

	p, err := NewTestPlugin("", raw, "redis-empty", YAEGI_PLUGIN)
	if err != nil {
		t.Fatalf("expected plugin to load: %v", err)
	}

	_, _, err = p.FuncEvalOther()
	if err == nil {
		t.Fatal("expected empty Redis key to be rejected")
	}
	if !strings.Contains(err.Error(), "plugin redis key cannot be empty") {
		t.Fatalf("expected empty key error, got %v", err)
	}
}
