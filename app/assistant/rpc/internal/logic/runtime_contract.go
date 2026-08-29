package logic

import (
	"esx/app/assistant/internal/memory"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"
	"esx/pkg/errx"
)

func requireAgentUser(userID int64) error {
	if userID <= 0 {
		return errx.NewWithCode(errx.LoginRequired)
	}
	return nil
}

func unavailableUntilStore() error {
	return errx.NewWithCode(errx.ServiceUnavailable)
}

func toPBMemory(entry memory.Entry) *pb.MemoryEntry {
	return &pb.MemoryEntry{
		Id: entry.ID, Target: entry.Target, Content: entry.Content, Version: entry.Version,
		CreatedAtMs: entry.CreatedAtMs, UpdatedAtMs: entry.UpdatedAtMs,
	}
}

func toPBCapacities(caps []memory.Capacity) []*pb.MemoryCapacity {
	out := make([]*pb.MemoryCapacity, 0, len(caps))
	for _, cap := range caps {
		out = append(out, &pb.MemoryCapacity{Target: cap.Target, Used: int32(cap.Used), Limit: int32(cap.Limit)})
	}
	return out
}
