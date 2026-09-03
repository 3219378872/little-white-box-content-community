package util

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"
)

const (
	// 起始时间戳 (2024-01-01 00:00:00)
	twepoch          = int64(1704038400000)
	workerIDBits     = uint(5)  // worker ID 位数
	datacenterIDBits = uint(5)  // datacenter ID 位数
	sequenceBits     = uint(12) // 序列号位数

	workerIDMax     = int64(-1 ^ (-1 << workerIDBits))
	datacenterIDMax = int64(-1 ^ (-1 << datacenterIDBits))
	sequenceMask    = int64(-1 ^ (-1 << sequenceBits))

	workerIDShift      = sequenceBits
	datacenterIDShift  = sequenceBits + workerIDBits
	timestampLeftShift = sequenceBits + workerIDBits + datacenterIDBits
)

// Snowflake 分布式 ID 生成器
type Snowflake struct {
	mu           sync.Mutex
	timestamp    int64
	workerID     int64
	datacenterID int64
	sequence     int64
}

var snowflake *Snowflake

// InitSnowflake 初始化 Snowflake
func InitSnowflake(workerID, datacenterID int64) error {
	if workerID < 0 || workerID > workerIDMax {
		return ErrInvalidWorkerID
	}
	if datacenterID < 0 || datacenterID > datacenterIDMax {
		return ErrInvalidDatacenterID
	}
	snowflake = &Snowflake{
		workerID:     workerID,
		datacenterID: datacenterID,
	}
	return nil
}

// NextID 生成下一个 ID
func NextID() (int64, error) {
	if snowflake == nil {
		return 0, ErrSnowflakeNotInit
	}
	return snowflake.NextID()
}

// 环境变量名：多副本部署时用于区分各实例，避免生成重复 ID。
const (
	EnvSnowflakeWorkerID     = "SNOWFLAKE_WORKER_ID"
	EnvSnowflakeDatacenterID = "SNOWFLAKE_DATACENTER_ID"
)

// InitSnowflakeFromEnv 是各服务统一的 Snowflake 初始化入口：
// 优先从环境变量读取 worker/datacenter ID，未设置时使用传入默认值；
// 格式非法时返回携带变量名的错误。默认值按服务区分
// （user=0、content=1、interaction=3、media=4），多副本部署必须显式配置。
func InitSnowflakeFromEnv(defaultWorkerID, defaultDatacenterID int64) error {
	parse := func(name string, def int64) (int64, error) {
		raw := os.Getenv(name)
		if raw == "" {
			return def, nil
		}
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("%s 格式无效: %w", name, err)
		}
		return id, nil
	}
	workerID, err := parse(EnvSnowflakeWorkerID, defaultWorkerID)
	if err != nil {
		return err
	}
	datacenterID, err := parse(EnvSnowflakeDatacenterID, defaultDatacenterID)
	if err != nil {
		return err
	}
	return InitSnowflake(workerID, datacenterID)
}

// NextID 生成下一个 ID
func (s *Snowflake) NextID() (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UnixMilli()
	if now < s.timestamp {
		return 0, ErrClockMovedBackwards
	}

	if s.timestamp == now {
		s.sequence = (s.sequence + 1) & sequenceMask
		if s.sequence == 0 {
			// 等待下一毫秒
			for now <= s.timestamp {
				now = time.Now().UnixMilli()
			}
		}
	} else {
		s.sequence = 0
	}

	s.timestamp = now

	id := ((now - twepoch) << timestampLeftShift) |
		(s.datacenterID << datacenterIDShift) |
		(s.workerID << workerIDShift) |
		s.sequence

	return id, nil
}

var (
	ErrInvalidWorkerID     = errors.New("invalid worker ID")
	ErrInvalidDatacenterID = errors.New("invalid datacenter ID")
	ErrSnowflakeNotInit    = errors.New("snowflake not initialized")
	ErrClockMovedBackwards = errors.New("snowflake clock moved backwards")
)
