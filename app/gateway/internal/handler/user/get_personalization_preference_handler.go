// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package user

import (
	"net/http"

	"esx/app/gateway/internal/logic/user"
	"esx/app/gateway/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// 获取个性化偏好
func GetPersonalizationPreferenceHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := user.NewGetPersonalizationPreferenceLogic(r.Context(), svcCtx)
		resp, err := l.GetPersonalizationPreference()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
