package logic

import (
	"context"
	"errors"
	"esx/app/media/rpc/internal/mediautil"
	"esx/app/media/rpc/pb/xiaobaihe/media/pb"
	"esx/pkg/errx"
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

func TestMediaIdempotencyRecord(t *testing.T) {
	meta := &pb.UploadMeta{UserId: 7, FileName: "a.png", Quality: 85, MaxWidth: 1000, MaxHeight: 800, IdempotencyKey: " idem-1 "}
	rec := mediaIdempotencyRecord(meta, "hash-a")
	if rec.Scope != "media:upload" || rec.UserID != 7 || rec.Key != "idem-1" {
		t.Fatalf("unexpected record: %+v", rec)
	}
	if rec.CommandHash == "" {
		t.Fatal("expected command hash")
	}

	// 同参数同内容同键 → 相同 hash；不同内容/不同参数 → 不同 hash（CORE-051 异命令冲突）
	other := mediaIdempotencyRecord(&pb.UploadMeta{UserId: 7, FileName: "a.png", Quality: 85, MaxWidth: 1000, MaxHeight: 800, IdempotencyKey: "idem-1"}, "hash-a")
	if other.CommandHash != rec.CommandHash {
		t.Fatal("expected identical command hash for identical command")
	}
	differentContent := mediaIdempotencyRecord(&pb.UploadMeta{UserId: 7, FileName: "a.png", Quality: 85, MaxWidth: 1000, MaxHeight: 800, IdempotencyKey: "idem-1"}, "hash-b")
	if differentContent.CommandHash == rec.CommandHash {
		t.Fatal("expected different command hash for different file content")
	}
	differentName := mediaIdempotencyRecord(&pb.UploadMeta{UserId: 7, FileName: "b.png", Quality: 85, MaxWidth: 1000, MaxHeight: 800, IdempotencyKey: "idem-1"}, "hash-a")
	if differentName.CommandHash == rec.CommandHash {
		t.Fatal("expected different command hash for different file name")
	}
}

func TestSHA256File(t *testing.T) {
	// 已知内容 → 确定性指纹；文件不存在 → 错误。
	path := t.TempDir() + "/sample.bin"
	if err := os.WriteFile(path, []byte("hello media"), 0o600); err != nil {
		t.Fatal(err)
	}
	sum, err := sha256File(context.Background(), path)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if sum == "" {
		t.Fatal("expected non-empty hash")
	}
	again, err := sha256File(context.Background(), path)
	if err != nil || again != sum {
		t.Fatalf("expected deterministic hash, got %q then %q (err=%v)", sum, again, err)
	}
	if _, err := sha256File(context.Background(), t.TempDir()+"/missing.bin"); err == nil {
		t.Fatal("expected error for missing file")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := sha256File(canceled, path); err == nil {
		t.Fatal("expected error for canceled context")
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
