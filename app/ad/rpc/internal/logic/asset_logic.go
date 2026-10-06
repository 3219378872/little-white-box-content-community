package logic

import (
	"context"
	"errors"
	"strings"
	"time"

	"esx/app/ad/internal/assets"
	"esx/app/ad/internal/store"
	"esx/app/ad/rpc/internal/svc"
	pb "esx/kitex_gen/ad"
	"esx/pkg/errx"

	"esx/pkg/logging"
)

type UploadAssetLogic struct{ base }

func NewUploadAssetLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UploadAssetLogic {
	return &UploadAssetLogic{newBase(ctx, svcCtx)}
}

// UploadAsset 把素材或证件写入私有存储；未过审素材不经公开地址访问（ADS-015）。
func (l *UploadAssetLogic) UploadAsset(in *pb.UploadAssetReq) (*pb.UploadAssetResp, error) {
	if err := requireUser(in.GetUserId()); err != nil {
		return nil, err
	}
	kind := strings.TrimSpace(in.GetKind())
	if (kind != store.AssetCreative && kind != store.AssetDocument) || len(in.GetIdempotencyKey()) > 128 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	inspected, err := assets.Inspect(kind, in.GetContent(), kind == store.AssetDocument)
	if err != nil {
		return nil, l.mapError(err, "inspect asset")
	}
	id, err := l.svcCtx.Store.NewAssetID()
	if err != nil {
		return nil, l.mapError(err, "allocate asset id")
	}
	key := assets.PrivateKey(id)
	if err := l.svcCtx.Assets.PutPrivate(l.ctx, key, in.GetContent(), inspected.MimeType); err != nil {
		return nil, l.mapError(err, "store private asset")
	}
	asset, err := l.svcCtx.Store.CreateAsset(l.ctx, store.Asset{
		ID: id, UserID: in.GetUserId(), Kind: kind, SHA256: inspected.SHA256, MimeType: inspected.MimeType,
		SizeBytes: inspected.Size, ObjectKey: key,
	}, in.GetIdempotencyKey(), l.svcCtx.Now())
	if err != nil {
		l.discardUnrecorded(id, key)
		return nil, l.mapError(err, "record asset")
	}
	if asset.ObjectKey != key {
		// 幂等重放返回首次上传的记录，本次写入的对象无人引用。
		l.deletePrivate(key)
	}
	return &pb.UploadAssetResp{Asset: &pb.AssetView{
		AssetId: asset.ID, Kind: asset.Kind, Sha256: asset.SHA256, MimeType: asset.MimeType, Size: asset.SizeBytes,
	}}, nil
}

// assetCleanupTimeout 限定回滚删除的耗时；删除不随请求取消，客户端断开也会完成清理。
const assetCleanupTimeout = 10 * time.Second

// discardUnrecorded 在登记失败后删除已写入的私有对象。事务可能在返回错误前已提交，
// 因此只有确认记录不存在时才删除；无法确认时保留对象，宁可遗留也不让已登记的资产丢失内容。
func (l *UploadAssetLogic) discardUnrecorded(id int64, key string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(l.ctx), assetCleanupTimeout)
	defer cancel()
	if _, err := l.svcCtx.Store.GetAsset(ctx, id); !errors.Is(err, store.ErrNotFound) {
		if err != nil {
			l.Errorw("keep private asset: record state unknown", logging.Field("objectKey", key), logging.Field("err", err.Error()))
		}
		return
	}
	l.deletePrivate(key)
}

func (l *UploadAssetLogic) deletePrivate(key string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(l.ctx), assetCleanupTimeout)
	defer cancel()
	if err := l.svcCtx.Assets.DeletePrivate(ctx, key); err != nil {
		l.Errorw("delete orphaned private asset failed", logging.Field("objectKey", key), logging.Field("err", err.Error()))
	}
}

type ReadAssetLogic struct{ base }

func NewReadAssetLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ReadAssetLogic {
	return &ReadAssetLogic{newBase(ctx, svcCtx)}
}

// ReadAsset 只返回给本人，或 Gateway 已经审核平台授权的审核员（ADS-040、RVW-023）；越权统一不存在。
func (l *ReadAssetLogic) ReadAsset(in *pb.ReadAssetReq) (*pb.ReadAssetResp, error) {
	if err := requireUser(in.GetUserId()); err != nil {
		return nil, err
	}
	asset, err := l.svcCtx.Store.GetAsset(l.ctx, in.GetAssetId())
	if err != nil {
		return nil, l.mapError(err, "get asset")
	}
	if asset.UserID != in.GetUserId() && !in.GetReviewerAuthorized() {
		return nil, errx.NewWithCode(errx.NotFound)
	}
	content, err := l.svcCtx.Assets.GetPrivate(l.ctx, asset.ObjectKey)
	if err != nil {
		return nil, l.mapError(err, "read private asset")
	}
	return &pb.ReadAssetResp{Content: content, MimeType: asset.MimeType}, nil
}
