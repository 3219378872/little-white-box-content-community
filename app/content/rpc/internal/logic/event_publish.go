package logic

import (
	"context"
	"encoding/json"
	"time"

	"esx/app/content/rpc/internal/svc"
	"esx/pkg/event"
	"mqx"
	"util"

	"github.com/zeromicro/go-zero/core/logx"
)

// publishPostEvent 把帖子生命周期事件发到 RocketMQ。
// 失败不阻塞主链路：DB 已提交，事件只用于 search/embedding/cleanup 等
// 异步下游，错过的事件可通过对账重发或在线 reindex 补齐。
//
// topic 应为 mqx.TopicPostCreate / TopicPostUpdate / TopicPostDelete 之一。
func publishPostEvent(ctx context.Context, p svc.MQProducer, topic string, e event.PostEvent) {
	if p == nil {
		return
	}
	if e.EventID == 0 {
		if id, err := util.NextID(); err == nil {
			e.EventID = id
		}
	}
	if e.EventTime == 0 {
		e.EventTime = time.Now().UnixMilli()
	}
	if err := e.Validate(); err != nil {
		logx.WithContext(ctx).Errorw("publishPostEvent: invalid event, skipping",
			logx.Field("topic", topic), logx.Field("err", err.Error()))
		return
	}
	body, err := json.Marshal(e)
	if err != nil {
		logx.WithContext(ctx).Errorw("publishPostEvent: marshal failed",
			logx.Field("topic", topic), logx.Field("err", err.Error()))
		return
	}
	if _, err := p.SendSyncWithTag(ctx, topic, mqx.TagDefault, body); err != nil {
		logx.WithContext(ctx).Errorw("publishPostEvent: send failed",
			logx.Field("topic", topic), logx.Field("post_id", e.PostID),
			logx.Field("err", err.Error()))
		return
	}
	logx.WithContext(ctx).Infow("publishPostEvent: sent",
		logx.Field("topic", topic), logx.Field("post_id", e.PostID),
		logx.Field("type", string(e.Type)))
}
