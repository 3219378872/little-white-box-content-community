package errx

import (
	"net/http"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// GRPCCode maps BizError to a standard gRPC status code.
// Derived from HTTPStatus() to keep HTTP/gRPC semantics consistent.
func (e *BizError) GRPCCode() codes.Code {
	switch e.HTTPStatus() {
	case http.StatusOK:
		return codes.OK
	case http.StatusBadRequest:
		if e.Code == NotLikedYet || e.Code == NotFavoritedYet {
			return codes.FailedPrecondition
		}
		return codes.InvalidArgument
	case http.StatusUnauthorized:
		return codes.Unauthenticated
	case http.StatusForbidden:
		return codes.PermissionDenied
	case http.StatusNotFound, http.StatusGone:
		return codes.NotFound
	case http.StatusConflict:
		return codes.AlreadyExists
	case http.StatusTooManyRequests:
		return codes.ResourceExhausted
	case http.StatusServiceUnavailable:
		return codes.Unavailable
	default:
		return codes.Internal
	}
}

// GRPCStatus implements the grpcstatus interface recognized by grpc/status.FromError.
// Business code is carried as a wrapperspb.Int32Value detail so the client can reconstruct BizError.
func (e *BizError) GRPCStatus() *status.Status {
	st := status.New(e.GRPCCode(), e.Message)
	detailed, err := st.WithDetails(wrapperspb.Int32(int32(e.Code)))
	if err != nil {
		return st
	}
	return detailed
}

// FromGRPCError converts a gRPC status error back to a BizError.
// If the status carries a BizError detail (Int32Value), the original business code and message are restored.
// If not (e.g. framework-generated timeout/breaker errors), a BizError is synthesized from the gRPC code.
// Non-gRPC errors and nil are returned as-is.
func FromGRPCError(err error) error {
	if err == nil {
		return nil
	}

	s, ok := status.FromError(err)
	if !ok {
		return err
	}

	for _, detail := range s.Details() {
		if v, ok := detail.(*wrapperspb.Int32Value); ok {
			return &BizError{
				Code:    int(v.Value),
				Message: s.Message(),
			}
		}
	}

	return &BizError{
		Code:    grpcCodeToBizCode(s.Code()),
		Message: s.Message(),
	}
}

func grpcCodeToBizCode(c codes.Code) int {
	switch c {
	case codes.OK:
		return SUCCESS
	case codes.InvalidArgument:
		return ParamError
	case codes.NotFound:
		return NotFound
	case codes.Unauthenticated:
		return LoginRequired
	case codes.PermissionDenied:
		return PermissionDenied
	case codes.AlreadyExists:
		return UserAlreadyExist
	case codes.ResourceExhausted:
		return TooManyReq
	case codes.Unavailable:
		return ServiceUnavailable
	case codes.DeadlineExceeded:
		return SystemError
	default:
		return SystemError
	}
}
