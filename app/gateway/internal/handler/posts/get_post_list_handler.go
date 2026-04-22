// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package posts

import (
	"net/http"

	"gateway/internal/logic/posts"
	"gateway/internal/svc"
	"gateway/internal/types"
	"jwtx"
	"middleware"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// 获取帖子列表
func GetPostListHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req types.GetPostListReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := posts.NewGetPostListLogic(r.Context(), svcCtx)
		resp, err := l.GetPostList(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	})

	return middleware.OptionalAuthMiddleware(jwtx.JwtConfig{
		AccessSecret: svcCtx.Config.Auth.AccessSecret,
		AccessExpire: svcCtx.Config.Auth.AccessExpire,
	})(inner).ServeHTTP
}
