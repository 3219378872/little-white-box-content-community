package svc

import (
	"reflect"
	"testing"

	"esx/app/pipeline/behaviorlog/internal/config"
	"mqx"

	"github.com/stretchr/testify/assert"
	"github.com/zeromicro/go-zero/core/stores/redis"
)

func TestNewServiceContext_PanicsWithMissingConfigList(t *testing.T) {
	assert.PanicsWithError(t,
		"missing behavior-log config: ClickHouseDSN, MQ.NameServer, MQ.GroupName, Redis.Host, BloomBits",
		func() { NewServiceContext(config.Config{}) })
}

func TestValidateBehaviorLogConfig_AllowsMinimalRequiredConfig(t *testing.T) {
	err := validateBehaviorLogConfig(config.Config{
		MQ: mqx.ConsumerConfig{
			NameServer: "127.0.0.1:9876",
			GroupName:  mqx.GroupBehaviorLogService,
		},
		ClickHouseDSN: "clickhouse://localhost:9000/xbh_analytics",
		Redis: redis.RedisConf{
			Host: "127.0.0.1:6379",
			Type: redis.NodeType,
		},
		BloomBits: 1024,
	})

	assert.NoError(t, err)
}

func TestServiceContextDependencyFieldsUseSvcInterfaces(t *testing.T) {
	serviceContextType := reflect.TypeOf(ServiceContext{})
	expectedPkg := reflect.TypeOf((*BehaviorStore)(nil)).Elem().PkgPath()

	for _, name := range []string{"Store", "Dedup"} {
		field, ok := serviceContextType.FieldByName(name)
		assert.True(t, ok)
		assert.Equal(t, reflect.Interface, field.Type.Kind())
		assert.Equal(t, expectedPkg, field.Type.PkgPath())
	}
}
