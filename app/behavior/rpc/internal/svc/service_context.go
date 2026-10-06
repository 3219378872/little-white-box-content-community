package svc

import (
	"fmt"
	"strings"
	"time"

	"esx/app/behavior/rpc/internal/config"
	"esx/app/behavior/rpc/internal/publisher"
	"esx/pkg/mqx"
)

// ServiceContext 持有行为上报的配置与投递器；Now 可在测试中替换以固定时钟窗口。
type ServiceContext struct {
	Config    config.Config
	Publisher publisher.Publisher
	Now       func() time.Time
	producer  *mqx.Producer
}

// NewServiceContext 校验配置并创建 MQ 生产者，失败时终止启动。
func NewServiceContext(c config.Config) *ServiceContext {
	if err := validateConfig(c); err != nil {
		panic(err)
	}
	producer, err := mqx.NewProducer(c.MQ)
	if err != nil {
		panic(fmt.Errorf("behavior-rpc: initialize producer: %w", err))
	}
	return &ServiceContext{
		Config:    c,
		Publisher: publisher.NewMQPublisher(producer),
		Now:       time.Now,
		producer:  producer,
	}
}

// Close 关闭 MQ 生产者。
func (s *ServiceContext) Close() error {
	if s.producer == nil {
		return nil
	}
	return s.producer.Shutdown()
}

// validateConfig 一次列出所有缺失或越界的配置项，便于部署时一次修正。
func validateConfig(c config.Config) error {
	missing := make([]string, 0, 4)
	if c.MQ.NameServer == "" {
		missing = append(missing, "MQ.NameServer")
	}
	if c.MQ.GroupName == "" {
		missing = append(missing, "MQ.GroupName")
	}
	if c.MaxBatchSize <= 0 || c.MaxBatchSize > 100 {
		missing = append(missing, "MaxBatchSize(1..100)")
	}
	if c.MaxPastAgeHours <= 0 || c.MaxFutureSkewSeconds <= 0 {
		missing = append(missing, "event clock windows")
	}
	if len(missing) > 0 {
		return fmt.Errorf("behavior-rpc: invalid config: %s", strings.Join(missing, ", "))
	}
	return nil
}
