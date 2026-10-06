package logic

import (
	"esx/app/assistant/internal/memory"
	pb "esx/kitex_gen/assistant"
	"esx/pkg/errx"
)

// requireAgentUser 拒绝未登录的调用。
func requireAgentUser(userID int64) error {
	if userID <= 0 {
		return errx.NewWithCode(errx.LoginRequired)
	}
	return nil
}

// unavailableUntilStore 是存储未配置时的统一错误。
func unavailableUntilStore() error {
	return errx.NewWithCode(errx.ServiceUnavailable)
}

// toPBMemory 把记忆条目映射为 RPC 结构。
func toPBMemory(entry memory.Entry) *pb.MemoryEntry {
	return &pb.MemoryEntry{
		Id: entry.ID, Target: entry.Target, Content: entry.Content, Version: entry.Version,
		CreatedAtMs: entry.CreatedAtMs, UpdatedAtMs: entry.UpdatedAtMs,
	}
}

// toPBCapacities 把容量占用映射为 RPC 结构。
func toPBCapacities(caps []memory.Capacity) []*pb.MemoryCapacity {
	out := make([]*pb.MemoryCapacity, 0, len(caps))
	for _, cap := range caps {
		out = append(out, &pb.MemoryCapacity{Target: cap.Target, Used: int32(cap.Used), Limit: int32(cap.Limit)})
	}
	return out
}
