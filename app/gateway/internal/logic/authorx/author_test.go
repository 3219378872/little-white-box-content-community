package authorx

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"esx/app/content/rpc/contentservice"
	"esx/app/gateway/internal/svc"
	userpb "esx/app/user/rpc/pb/xiaobaihe/user/pb"
	"esx/app/user/rpc/userservice"
	"esx/pkg/errx"

	"google.golang.org/grpc"
)

type fakeUserService struct {
	userservice.UserService
	fn func(context.Context, *userservice.BatchGetUsersReq, ...grpc.CallOption) (*userservice.BatchGetUsersResp, error)
}

func (f *fakeUserService) BatchGetUsers(ctx context.Context, in *userservice.BatchGetUsersReq, opts ...grpc.CallOption) (*userservice.BatchGetUsersResp, error) {
	return f.fn(ctx, in, opts...)
}

func TestDisplayName(t *testing.T) {
	if got := DisplayName("  Alice  ", "alice"); got != "Alice" {
		t.Fatalf("nickname wins, got %q", got)
	}
	if got := DisplayName("  ", " bob "); got != "bob" {
		t.Fatalf("username fallback, got %q", got)
	}
	if got := DisplayName("", ""); got != "" {
		t.Fatalf("empty, got %q", got)
	}
}

func TestUniquePositive(t *testing.T) {
	got := UniquePositive([]int64{0, 7, 7, -1, 9, 0, 7})
	want := []int64{7, 9}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("UniquePositive = %v, want %v", got, want)
	}
}

func TestPostAuthorIDs(t *testing.T) {
	got := PostAuthorIDs([]*contentservice.PostInfo{
		nil,
		{Id: 1, AuthorId: 7},
		{Id: 2, AuthorId: 0},
		{Id: 3, AuthorId: 9},
		{Id: 4, AuthorId: 7},
	})
	want := []int64{7, 9}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("PostAuthorIDs = %v, want %v", got, want)
	}
}

func TestLoad_MapsDisplayFields(t *testing.T) {
	svcCtx := &svc.ServiceContext{UserService: &fakeUserService{
		fn: func(_ context.Context, in *userservice.BatchGetUsersReq, _ ...grpc.CallOption) (*userservice.BatchGetUsersResp, error) {
			if !reflect.DeepEqual(in.UserIds, []int64{7, 9}) {
				t.Fatalf("unexpected ids %v", in.UserIds)
			}
			return &userservice.BatchGetUsersResp{Users: []*userpb.UserInfo{
				nil,
				{Id: 0, Nickname: "skip"},
				{Id: 7, Nickname: " Alice ", Username: "alice", AvatarUrl: " https://a.png "},
				{Id: 9, Nickname: "", Username: "bob", AvatarUrl: "https://b.png"},
			}}, nil
		},
	}}
	got, err := Load(context.Background(), svcCtx, []int64{7, 7, 9})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got[7] != (Author{Name: "Alice", Avatar: "https://a.png"}) {
		t.Fatalf("author 7 = %+v", got[7])
	}
	if got[9] != (Author{Name: "bob", Avatar: "https://b.png"}) {
		t.Fatalf("author 9 = %+v", got[9])
	}
}

func TestLoad_NilServiceIsSystemError(t *testing.T) {
	_, err := Load(context.Background(), &svc.ServiceContext{}, []int64{1})
	if !errx.Is(err, errx.SystemError) {
		t.Fatalf("want SystemError, got %v", err)
	}
}

func TestLoadSoft_RPCErrorDegrades(t *testing.T) {
	svcCtx := &svc.ServiceContext{UserService: &fakeUserService{
		fn: func(context.Context, *userservice.BatchGetUsersReq, ...grpc.CallOption) (*userservice.BatchGetUsersResp, error) {
			return nil, errors.New("timeout")
		},
	}}
	got := LoadSoft(context.Background(), svcCtx, []int64{7})
	if len(got) != 0 {
		t.Fatalf("want empty map, got %+v", got)
	}
}

func TestLoadSoft_NilServiceSilent(t *testing.T) {
	got := LoadSoft(context.Background(), &svc.ServiceContext{}, []int64{7})
	if len(got) != 0 {
		t.Fatalf("want empty map, got %+v", got)
	}
}
