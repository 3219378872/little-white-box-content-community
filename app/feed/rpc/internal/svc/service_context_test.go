package svc

import "testing"

func TestServiceContext_TypeSurface(t *testing.T) {
	ctx := &ServiceContext{}
	_ = ctx.InboxModel
	_ = ctx.OutboxModel
	_ = ctx.UserService
	_ = ctx.ContentService
	_ = ctx.BigVThreshold
	_ = ctx.FanoutBatchSize
}
