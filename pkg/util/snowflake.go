package util

import (
	"errors"
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

// NextID 生成下一个 ID
func (s *Snowflake) NextID() (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UnixMilli()

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
)
