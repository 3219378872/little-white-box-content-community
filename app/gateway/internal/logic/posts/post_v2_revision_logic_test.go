// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package posts

import (
	"context"
	"testing"

	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/pkg/errx"

	"github.com/stretchr/testify/assert"
)

// CORE-013：v2 写接口强制乐观锁，缺失或为 0 的 expectedRevision 必须被拒绝。
func TestUpdatePostV2RejectsMissingOrZeroRevision(t *testing.T) {
	l := NewUpdatePostV2Logic(context.Background(), &svc.ServiceContext{})
	for name, req := range map[string]*types.UpdatePostV2Req{
		"missing": {PostId: 11},
		"zero":    {PostId: 11, ExpectedRevision: 0},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := l.UpdatePostV2(req)
			assert.True(t, errx.Is(err, errx.ParamError), "want ParamError, got %v", err)
		})
	}
}

func TestDeletePostV2RejectsMissingOrZeroRevision(t *testing.T) {
	l := NewDeletePostV2Logic(context.Background(), &svc.ServiceContext{})
	for name, req := range map[string]*types.DeletePostV2Req{
		"missing": {PostId: 11},
		"zero":    {PostId: 11, ExpectedRevision: 0},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := l.DeletePostV2(req)
			assert.True(t, errx.Is(err, errx.ParamError), "want ParamError, got %v", err)
		})
	}
}
