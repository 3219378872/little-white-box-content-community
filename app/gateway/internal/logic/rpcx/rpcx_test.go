package rpcx

import (
	"context"
	"errors"
	"testing"

	"esx/pkg/errx"

	logx "esx/pkg/logging"
)

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
