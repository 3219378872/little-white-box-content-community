package logic

import (
	"errors"
	"errx"
	"esx/app/media/rpc/internal/mediautil"
	"esx/app/media/rpc/pb/xiaobaihe/media/pb"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestNullStringOr_EmptyMapsToInvalid(t *testing.T) {
	got := nullStringOr("")
	if got.Valid {
		t.Fatalf("expected Valid=false for empty, got %+v", got)
	}
	if got.String != "" {
		t.Fatalf("expected zero string, got %q", got.String)
	}
}

func TestNullStringOr_NonEmptyMapsToValid(t *testing.T) {
	got := nullStringOr("hello")
	if !got.Valid {
		t.Fatalf("expected Valid=true, got %+v", got)
	}
	if got.String != "hello" {
		t.Fatalf("expected 'hello', got %q", got.String)
	}
}

func TestNullInt_ZeroMapsToInvalid(t *testing.T) {
	got := nullInt(0)
	if got.Valid {
		t.Fatalf("expected Valid=false for zero, got %+v", got)
	}
}

func TestNullInt_NonZeroMapsToValid(t *testing.T) {
	got := nullInt(42)
	if !got.Valid || got.Int64 != 42 {
		t.Fatalf("expected Valid=true Int64=42, got %+v", got)
	}
}

func TestBuildObjectKey_MatchesPrefixMonthUuidExt(t *testing.T) {
	ym := time.Now().Format("200601")
	got := buildObjectKey("original", "jpg")

	// pattern: original/YYYYMM/<uuid>.jpg
	pat := regexp.MustCompile(`^original/` + regexp.QuoteMeta(ym) +
		`/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\.jpg$`)
	if !pat.MatchString(got) {
		t.Fatalf("key %q does not match pattern %s", got, pat)
	}
}

func TestBuildObjectKey_UniqueAcrossCalls(t *testing.T) {
	a := buildObjectKey("thumb", "jpg")
	b := buildObjectKey("thumb", "jpg")
	if a == b {
		t.Fatalf("expected unique keys, both were %q", a)
	}
}

// --- receiveUploadStream tests ---

// fakeReq 最小化 pb.UploadImageReq 两个 getter 所需的能力。
type fakeReq struct {
	meta  *pb.UploadMeta
	chunk []byte
}

func (r *fakeReq) GetMeta() *pb.UploadMeta { return r.meta }
func (r *fakeReq) GetChunk() []byte        { return r.chunk }

// scriptedRecv 把一个预置的 (req, err) 脚本包装成 recv 函数。
type scriptedStep struct {
	req *fakeReq
	err error
}

func scriptedRecv(steps []scriptedStep) func() (*fakeReq, error) {
	i := 0
	return func() (*fakeReq, error) {
		if i >= len(steps) {
			return nil, io.EOF
		}
		s := steps[i]
		i++
		return s.req, s.err
	}
}

func newSink(t *testing.T) *mediautil.TempSink {
	t.Helper()
	s, err := mediautil.NewTempSink(os.TempDir(), 4096)
	if err != nil {
		t.Fatalf("new sink: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestReceiveUploadStream_MetaFirstThenChunksThenEOF(t *testing.T) {
	meta := &pb.UploadMeta{UserId: 7, FileName: "x.jpg"}
	steps := []scriptedStep{
		{req: &fakeReq{meta: meta}},
		{req: &fakeReq{chunk: []byte("abc")}},
		{req: &fakeReq{chunk: []byte("de")}},
		{req: nil, err: io.EOF},
	}
	sink := newSink(t)

	got, err := receiveUploadStream[fakeReq](
		scriptedRecv(steps),
		func(r *fakeReq) *pb.UploadMeta { return r.GetMeta() },
		func(r *fakeReq) []byte { return r.GetChunk() },
		sink,
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != meta {
		t.Fatalf("expected same meta pointer, got %+v", got)
	}
	if sink.Size() != 5 {
		t.Fatalf("expected sink size 5, got %d", sink.Size())
	}
}

func TestReceiveUploadStream_MissingMetaReturnsMediaMetaMissing(t *testing.T) {
	steps := []scriptedStep{
		{req: &fakeReq{chunk: []byte("abc")}}, // 首包不是 meta
	}
	sink := newSink(t)

	_, err := receiveUploadStream[fakeReq](
		scriptedRecv(steps),
		func(r *fakeReq) *pb.UploadMeta { return r.GetMeta() },
		func(r *fakeReq) []byte { return r.GetChunk() },
		sink,
	)
	if err == nil || errx.GetCode(err) != errx.MediaMetaMissing {
		t.Fatalf("expected MediaMetaMissing, got %v", err)
	}
}

func TestReceiveUploadStream_FirstPacketErrWrapsError(t *testing.T) {
	boom := errors.New("net dead")
	steps := []scriptedStep{{req: nil, err: boom}}
	sink := newSink(t)

	_, err := receiveUploadStream[fakeReq](
		scriptedRecv(steps),
		func(r *fakeReq) *pb.UploadMeta { return r.GetMeta() },
		func(r *fakeReq) []byte { return r.GetChunk() },
		sink,
	)
	if err == nil || !errors.Is(err, boom) {
		t.Fatalf("expected wrapped boom err, got %v", err)
	}
}

func TestReceiveUploadStream_SecondMetaReturnsParamError(t *testing.T) {
	meta := &pb.UploadMeta{UserId: 1}
	steps := []scriptedStep{
		{req: &fakeReq{meta: meta}},
		{req: &fakeReq{}}, // meta=nil chunk=nil 表示第二次出现 meta 或未知分支
	}
	sink := newSink(t)

	_, err := receiveUploadStream[fakeReq](
		scriptedRecv(steps),
		func(r *fakeReq) *pb.UploadMeta { return r.GetMeta() },
		func(r *fakeReq) []byte { return r.GetChunk() },
		sink,
	)
	if err == nil || errx.GetCode(err) != errx.ParamError {
		t.Fatalf("expected ParamError, got %v", err)
	}
}

func TestReceiveUploadStream_OversizeMapsToFileTooLarge(t *testing.T) {
	meta := &pb.UploadMeta{UserId: 1}
	big := strings.Repeat("x", 8) // sink limit 小于这个即可
	s, err := mediautil.NewTempSink(os.TempDir(), 4)
	if err != nil {
		t.Fatalf("new sink: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	steps := []scriptedStep{
		{req: &fakeReq{meta: meta}},
		{req: &fakeReq{chunk: []byte(big)}},
	}
	_, err = receiveUploadStream[fakeReq](
		scriptedRecv(steps),
		func(r *fakeReq) *pb.UploadMeta { return r.GetMeta() },
		func(r *fakeReq) []byte { return r.GetChunk() },
		s,
	)
	if err == nil || errx.GetCode(err) != errx.FileTooLarge {
		t.Fatalf("expected FileTooLarge, got %v", err)
	}
}
