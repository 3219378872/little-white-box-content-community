package rpcx

import (
	"context"
	"errors"
	"testing"

	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"github.com/zeromicro/go-zero/core/logx"
)

func TestRequireUser_FromClaims(t *testing.T) {
	ctx := jwtx.WithClaimsContext(context.Background(), &jwtx.Claims{UserId: 42})
	id, err := RequireUser(ctx)
	if err != nil || id != 42 {
		t.Fatalf("RequireUser = %d, %v", id, err)
	}
}

func TestRequireUser_MissingIsLoginRequired(t *testing.T) {
	_, err := RequireUser(context.Background())
	if !errx.Is(err, errx.LoginRequired) {
		t.Fatalf("want LoginRequired, got %v", err)
	}
}

func TestRequireUser_NonPositive(t *testing.T) {
	ctx := jwtx.WithUserIdContext(context.Background(), 0)
	_, err := RequireUser(ctx)
	if !errx.Is(err, errx.LoginRequired) {
		t.Fatalf("want LoginRequired, got %v", err)
	}
}

func TestError_WrapsNonBizAsSystemError(t *testing.T) {
	err := Error(logx.WithContext(context.Background()), "UserService.Login", errors.New("rpc timeout"))
	if !errx.Is(err, errx.SystemError) {
		t.Fatalf("want SystemError, got %v", err)
	}
}

func TestError_PreservesBizError(t *testing.T) {
	in := errx.NewWithCode(errx.ParamError)
	err := Error(logx.WithContext(context.Background()), "UserService.Login", in)
	if !errx.Is(err, errx.ParamError) {
		t.Fatalf("want ParamError, got %v", err)
	}
}

func TestError_Nil(t *testing.T) {
	if Error(logx.WithContext(context.Background()), "op", nil) != nil {
		t.Fatal("nil err should stay nil")
	}
}
