package logic

import "esx/pkg/util"

// init 为依赖 Snowflake 的单测（如 outbox 事件构造）初始化 ID 生成器；
// InitSnowflake 幂等，与 integration 模式的 TestMain 兼容。
func init() {
	if err := util.InitSnowflake(1, 1); err != nil {
		panic(err)
	}
}
