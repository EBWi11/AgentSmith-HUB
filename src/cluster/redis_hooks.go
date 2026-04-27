package cluster

import "AgentSmith-HUB/common"

var (
	clusterRedisGet                    = common.RedisGet
	clusterRedisSet                    = common.RedisSet
	clusterRedisDel                    = common.RedisDel
	clusterRedisKeys                   = common.RedisKeys
	clusterRedisPublish                = common.RedisPublish
	clusterRetryWithExponentialBackoff = common.RetryWithExponentialBackoff
	clusterRecordInstruction           = common.RecordClusterInstruction
	clusterRecordComponentAdd          = common.RecordComponentAdd
	clusterRecordComponentUpdate       = common.RecordComponentUpdate
	clusterRecordLocalPush             = common.RecordLocalPush
	clusterRecordChangePush            = common.RecordChangePush
)

func resetClusterRedisHooks() {
	clusterRedisGet = common.RedisGet
	clusterRedisSet = common.RedisSet
	clusterRedisDel = common.RedisDel
	clusterRedisKeys = common.RedisKeys
	clusterRedisPublish = common.RedisPublish
	clusterRetryWithExponentialBackoff = common.RetryWithExponentialBackoff
	clusterRecordInstruction = common.RecordClusterInstruction
	clusterRecordComponentAdd = common.RecordComponentAdd
	clusterRecordComponentUpdate = common.RecordComponentUpdate
	clusterRecordLocalPush = common.RecordLocalPush
	clusterRecordChangePush = common.RecordChangePush
}
