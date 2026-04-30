package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"cleanupx"
	"esx/app/pipeline/behaviorlog/internal/config"
	behaviorconsumer "esx/app/pipeline/behaviorlog/internal/consumer"
	"esx/app/pipeline/behaviorlog/internal/svc"
	"mqx"
	"util"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
)

var configFile = flag.String("f", "etc/behavior-log.yaml", "config file")

func main() {
	flag.Parse()
	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())

	if err := util.InitSnowflake(c.WorkerID, c.DatacenterID); err != nil {
		logx.Must(err)
	}

	svcCtx := svc.NewServiceContext(c)
	handler := behaviorconsumer.MakeBehaviorHandler(svcCtx.Store, svcCtx.Dedup)

	mq, err := mqx.NewConsumer(c.MQ)
	if err != nil {
		logx.Must(err)
	}

	topics := []string{
		mqx.TopicLike, mqx.TopicUnlike,
		mqx.TopicFavorite, mqx.TopicUnfavorite,
		mqx.TopicCommentCreate,
		mqx.TopicUserFollow, mqx.TopicUserUnfollow,
	}
	for _, topic := range topics {
		if err := mq.SubscribeWithTopic(topic, mqx.TagDefault, handler); err != nil {
			logx.Must(fmt.Errorf("subscribe %s: %w", topic, err))
		}
	}

	if err := mq.Start(); err != nil {
		logx.Must(err)
	}
	logger := logx.WithContext(context.Background())
	defer cleanupx.Shutdown(logger, "behavior-log clickhouse", svcCtx.Close)
	defer cleanupx.Shutdown(logger, "behavior-log consumer", mq.Shutdown)

	shutdownCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Println("Behavior-log consumer started, subscribing: like/unlike/favorite/unfavorite/comment-create/user-follow/user-unfollow")
	<-shutdownCtx.Done()
}
