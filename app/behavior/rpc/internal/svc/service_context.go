package svc

import (
	"fmt"
	"strings"
	"time"

	"esx/app/behavior/rpc/internal/config"
	"esx/app/behavior/rpc/internal/publisher"
	"esx/pkg/mqx"
)

type ServiceContext struct {
	Config    config.Config
	Publisher publisher.Publisher
	Now       func() time.Time
	producer  *mqx.Producer
}

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

func (s *ServiceContext) Close() error {
	if s.producer == nil {
		return nil
	}
	return s.producer.Shutdown()
}

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
