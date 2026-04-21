package errx

import (
	"fmt"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestBizError_GRPCCode(t *testing.T) {
	tests := []struct {
		name     string
		bizCode  int
		wantCode codes.Code
	}{
		{"SUCCESS maps to OK", SUCCESS, codes.OK},
		{"ParamError maps to InvalidArgument", ParamError, codes.InvalidArgument},
		{"SystemError maps to Internal", SystemError, codes.Internal},
		{"UserNotFound maps to NotFound", UserNotFound, codes.NotFound},
		{"ContentNotFound maps to NotFound", ContentNotFound, codes.NotFound},
		{"MediaNotFound maps to NotFound", MediaNotFound, codes.NotFound},
		{"LoginRequired maps to Unauthenticated", LoginRequired, codes.Unauthenticated},
		{"TokenExpired maps to Unauthenticated", TokenExpired, codes.Unauthenticated},
		{"TokenInvalid maps to Unauthenticated", TokenInvalid, codes.Unauthenticated},
		{"PermissionDenied maps to PermissionDenied", PermissionDenied, codes.PermissionDenied},
		{"ContentForbidden maps to PermissionDenied", ContentForbidden, codes.PermissionDenied},
		{"FavoritesPrivate maps to PermissionDenied", FavoritesPrivate, codes.PermissionDenied},
		{"UserAlreadyExist maps to AlreadyExists", UserAlreadyExist, codes.AlreadyExists},
		{"TooManyReq maps to ResourceExhausted", TooManyReq, codes.ResourceExhausted},
		{"ServiceUnavailable maps to Unavailable", ServiceUnavailable, codes.Unavailable},
		{"PostAlreadyDeleted maps to NotFound", PostAlreadyDeleted, codes.NotFound},
		{"TitleEmpty maps to InvalidArgument", TitleEmpty, codes.InvalidArgument},
		{"FileTooLarge maps to InvalidArgument", FileTooLarge, codes.InvalidArgument},
		{"AlreadyLiked maps to InvalidArgument", AlreadyLiked, codes.InvalidArgument},
		{"CannotFollowSelf maps to InvalidArgument", CannotFollowSelf, codes.InvalidArgument},
		{"NotLikedYet maps to FailedPrecondition", NotLikedYet, codes.FailedPrecondition},
		{"NotFavoritedYet maps to FailedPrecondition", NotFavoritedYet, codes.FailedPrecondition},
		{"UnknownError maps to Internal", UnknownError, codes.Internal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bizErr := &BizError{Code: tt.bizCode, Message: GetMsg(tt.bizCode)}
			if got := bizErr.GRPCCode(); got != tt.wantCode {
				t.Errorf("GRPCCode() = %v, want %v", got, tt.wantCode)
			}
		})
	}
}

func TestBizError_GRPCStatus(t *testing.T) {
	bizErr := &BizError{Code: UserNotFound, Message: "用户不存在"}

	st := bizErr.GRPCStatus()

	// 1. gRPC code should be NotFound
	if st.Code() != codes.NotFound {
		t.Errorf("GRPCStatus().Code() = %v, want %v", st.Code(), codes.NotFound)
	}

	// 2. Message should be preserved
	if st.Message() != "用户不存在" {
		t.Errorf("GRPCStatus().Message() = %q, want %q", st.Message(), "用户不存在")
	}

	// 3. Detail should contain business code as Int32Value
	details := st.Details()
	if len(details) != 1 {
		t.Fatalf("expected 1 detail, got %d", len(details))
	}
	v, ok := details[0].(*wrapperspb.Int32Value)
	if !ok {
		t.Fatalf("detail type = %T, want *wrapperspb.Int32Value", details[0])
	}
	if int(v.Value) != UserNotFound {
		t.Errorf("detail value = %d, want %d", v.Value, UserNotFound)
	}
}

func TestBizError_GRPCStatus_SuccessReturnsNil(t *testing.T) {
	bizErr := &BizError{Code: SUCCESS, Message: "成功"}
	st := bizErr.GRPCStatus()

	// codes.OK status — Err() should return nil per gRPC contract
	if st.Err() != nil {
		t.Errorf("SUCCESS BizError should produce nil Err(), got %v", st.Err())
	}
}

func TestFromGRPCError_WithBizDetail(t *testing.T) {
	// Simulate: RPC server returned BizError, serialized through GRPCStatus()
	original := &BizError{Code: UserNotFound, Message: "用户不存在"}
	grpcErr := original.GRPCStatus().Err()

	// Act: convert back
	result := FromGRPCError(grpcErr)

	// Assert: should be a BizError with the same code and message
	bizErr, ok := result.(*BizError)
	if !ok {
		t.Fatalf("FromGRPCError returned %T, want *BizError", result)
	}
	if bizErr.Code != UserNotFound {
		t.Errorf("Code = %d, want %d", bizErr.Code, UserNotFound)
	}
	if bizErr.Message != "用户不存在" {
		t.Errorf("Message = %q, want %q", bizErr.Message, "用户不存在")
	}
}

func TestFromGRPCError_Nil(t *testing.T) {
	if err := FromGRPCError(nil); err != nil {
		t.Errorf("FromGRPCError(nil) = %v, want nil", err)
	}
}

func TestFromGRPCError_FrameworkErrors(t *testing.T) {
	tests := []struct {
		name     string
		grpcCode codes.Code
		grpcMsg  string
		wantBiz  int
	}{
		{"DeadlineExceeded → SystemError", codes.DeadlineExceeded, "context deadline exceeded", SystemError},
		{"Unavailable → ServiceUnavailable", codes.Unavailable, "service unavailable", ServiceUnavailable},
		{"Internal → SystemError", codes.Internal, "panic: something", SystemError},
		{"ResourceExhausted → TooManyReq", codes.ResourceExhausted, "cpu overloaded", TooManyReq},
		{"Unknown → SystemError", codes.Unknown, "unexpected", SystemError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			grpcErr := status.Error(tt.grpcCode, tt.grpcMsg)
			result := FromGRPCError(grpcErr)

			bizErr, ok := result.(*BizError)
			if !ok {
				t.Fatalf("FromGRPCError returned %T, want *BizError", result)
			}
			if bizErr.Code != tt.wantBiz {
				t.Errorf("Code = %d, want %d", bizErr.Code, tt.wantBiz)
			}
			if bizErr.Message != tt.grpcMsg {
				t.Errorf("Message = %q, want %q", bizErr.Message, tt.grpcMsg)
			}
		})
	}
}

func TestFromGRPCError_NonGRPCError(t *testing.T) {
	plainErr := fmt.Errorf("some plain error")
	result := FromGRPCError(plainErr)

	// Non-gRPC error should be returned as-is
	if result != plainErr {
		t.Errorf("expected original error, got %v", result)
	}
}
