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
