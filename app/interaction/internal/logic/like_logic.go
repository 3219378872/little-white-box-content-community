package logic

import (
	"context"

	"errx"
	"esx/app/interaction/internal/model"
	"esx/app/interaction/internal/svc"
	"esx/app/interaction/pb/xiaobaihe/interaction/pb"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

type LikeLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewLikeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *LikeLogic {
	return &LikeLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *LikeLogic) Like(in *pb.LikeReq) (*pb.LikeResp, error) {
	if in.UserId <= 0 || in.TargetId <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if l.svcCtx.Conn == nil || l.svcCtx.LikeRecordModel == nil || l.svcCtx.ActionCountModel == nil {
		l.Errorw("like dependencies are not configured")
		return nil, errx.NewWithCode(errx.SystemError)
	}

	var likeRecordID int64
	err := l.svcCtx.Conn.TransactCtx(l.ctx, func(ctx context.Context, session sqlx.Session) error {
		txConn := sqlx.NewSqlConnFromSession(session)
		result, id, err := l.svcCtx.LikeRecordModel.UpsertLikeStatusTx(ctx, txConn, in.UserId, in.TargetId, int64(in.TargetType), model.StatusActive)
		if err != nil {
			return err
		}
		if result == nil {
			return errx.NewWithCode(errx.SystemError)
		}
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if rowsAffected == 0 {
			return errx.NewWithCode(errx.AlreadyLiked)
		}
		likeRecordID = id
		return l.svcCtx.ActionCountModel.IncrLikeCountTx(ctx, txConn, in.TargetId, int64(in.TargetType))
	})
	if err != nil {
		if errx.Is(err, errx.AlreadyLiked) {
			return nil, err
		}
		l.Errorw("local like transaction failed",
			logx.Field("userId", in.UserId),
			logx.Field("targetId", in.TargetId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	if err := l.svcCtx.LikeRecordModel.InvalidateLikeRecordCache(l.ctx, likeRecordID, in.UserId, in.TargetId, int64(in.TargetType)); err != nil {
		l.Errorw("InvalidateLikeRecordCache failed", logx.Field("err", err.Error()))
	}

	return &pb.LikeResp{}, nil
}
