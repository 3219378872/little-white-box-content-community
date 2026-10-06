package logic

import (
	"context"

	"esx/app/review/internal/store"
	"esx/app/review/rpc/internal/svc"
	pb "esx/kitex_gen/review"
	"esx/pkg/event"
)

// AuthorizeEvidenceMediaLogic 承载 AuthorizeEvidenceMedia 接口的业务逻辑；每个请求新建一个实例。
type AuthorizeEvidenceMediaLogic struct{ base }

// NewAuthorizeEvidenceMediaLogic 绑定请求上下文与服务依赖。
func NewAuthorizeEvidenceMediaLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AuthorizeEvidenceMediaLogic {
	return &AuthorizeEvidenceMediaLogic{newBase(ctx, svcCtx)}
}

// AuthorizeEvidenceMedia 判断审核员能否经 Gateway 读取私有素材或证件（RVW-023、ADS-040）：
// 媒体必须属于该任务的冻结快照，且审核员对该任务可见；资质证件只对资质审核员可见。
func (l *AuthorizeEvidenceMediaLogic) AuthorizeEvidenceMedia(in *pb.AuthorizeEvidenceMediaReq) (*pb.AuthorizeEvidenceMediaResp, error) {
	r, err := l.reviewer(in.GetUserId(), claimRoles...)
	if err != nil {
		return nil, err
	}
	task, content, err := l.svcCtx.Store.TaskSnapshotContent(l.ctx, in.GetTaskId())
	if err != nil {
		return nil, l.mapStoreError(err, "load evidence task")
	}
	if !canSeeTask(r, task) {
		return &pb.AuthorizeEvidenceMediaResp{Allowed: false}, nil
	}
	snap, err := snapshotOf(content)
	if err != nil {
		return nil, l.mapStoreError(err, "decode evidence snapshot")
	}
	return &pb.AuthorizeEvidenceMediaResp{Allowed: mediaInSnapshot(snap, in.GetMediaId(), r)}, nil
}

// mediaInSnapshot 判断素材是否属于该任务快照；资质证件只对资质审核员开放。
func mediaInSnapshot(snap event.ReviewSnapshot, mediaID int64, r *store.Reviewer) bool {
	if mediaID <= 0 {
		return false
	}
	for _, media := range snap.Media {
		if media.MediaID == mediaID {
			return true
		}
	}
	if !r.HasRole(store.RoleQualificationReviewer) {
		return false
	}
	for _, qualification := range snap.Qualifications {
		if qualification.DocumentMediaID == mediaID {
			return true
		}
	}
	return false
}
