package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"

	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"
	"esx/app/content/rpc/contentservice"
	"esx/app/media/rpc/mediaservice"
	"esx/pkg/errx"
)

func TestResolveAttachmentsRejectsForeignMedia(t *testing.T) {
	session := &Session{Attachments: []Attachment{{MediaID: 11, URL: "http://x/11.png"}}}
	if _, _, err := resolveAttachments(session, []int64{99}, maxPostImages); err == nil {
		t.Fatal("expected error when mediaId is outside session attachments")
	}
	ids, urls, err := resolveAttachments(session, []int64{11}, maxPostImages)
	if err != nil || len(ids) != 1 || ids[0] != 11 || urls[0] != "http://x/11.png" {
		t.Fatalf("unexpected result: ids=%v urls=%v err=%v", ids, urls, err)
	}
}

type fakeContentService struct {
	contentservice.ContentService
	create     func(ctx context.Context, req *contentservice.CreatePostReq) (*contentservice.CreatePostResp, error)
	deletePost func(ctx context.Context, req *contentservice.DeletePostReq) (*contentservice.DeletePostResp, error)
	getPost    func(ctx context.Context, req *contentservice.GetPostReq) (*contentservice.GetPostResp, error)
}

func (f *fakeContentService) CreatePost(ctx context.Context, req *contentservice.CreatePostReq, _ ...grpc.CallOption) (*contentservice.CreatePostResp, error) {
	return f.create(ctx, req)
}

func (f *fakeContentService) DeletePost(ctx context.Context, req *contentservice.DeletePostReq, _ ...grpc.CallOption) (*contentservice.DeletePostResp, error) {
	return f.deletePost(ctx, req)
}

func (f *fakeContentService) GetPost(ctx context.Context, req *contentservice.GetPostReq, _ ...grpc.CallOption) (*contentservice.GetPostResp, error) {
	return f.getPost(ctx, req)
}

type fakeMediaService struct {
	mediaservice.MediaService
}

func (f *fakeMediaService) BatchGetMedia(ctx context.Context, req *mediaservice.BatchGetMediaReq, _ ...grpc.CallOption) (*mediaservice.BatchGetMediaResp, error) {
	infos := make([]*mediaservice.MediaInfo, 0, len(req.MediaIds))
	for _, id := range req.MediaIds {
		if id != 11 {
			continue
		}
		infos = append(infos, &mediaservice.MediaInfo{Id: id, UserId: 7, Status: 1})
	}
	return &mediaservice.BatchGetMediaResp{Medias: infos}, nil
}

type fakeBroker struct {
	opened   string
	waitErr  error
	approved bool
}

func (f *fakeBroker) Open(_ context.Context, userID int64, requestID, callID, toolName, summary string, ttl time.Duration) error {
	f.opened = callID
	return nil
}

func (f *fakeBroker) Resolve(context.Context, int64, string, string, bool) error { return nil }

func (f *fakeBroker) Wait(context.Context, int64, string, string, time.Duration) (bool, error) {
	return f.approved, f.waitErr
}

func newTestSession(session *Session) *Session {
	if session.Confirms == nil {
		session.Confirms = &fakeBroker{approved: true}
	}
	if session.Emit == nil {
		session.Emit = func(*pb.ChatEvent) error { return nil }
	}
	return session
}

func TestCreatePostDerivesIdempotencyAndRestrictsImages(t *testing.T) {
	var captured *contentservice.CreatePostReq
	content := &fakeContentService{
		create: func(_ context.Context, req *contentservice.CreatePostReq) (*contentservice.CreatePostResp, error) {
			captured = req
			return &contentservice.CreatePostResp{PostId: 5, Status: 1, Revision: 1}, nil
		},
	}
	registry, err := NewToolRegistry(Clients{Content: content, Media: &fakeMediaService{}}, []string{ToolCreatePost})
	if err != nil {
		t.Fatal(err)
	}
	session := newTestSession(&Session{
		UserID: 7, RequestID: "req-1",
		Attachments: []Attachment{{MediaID: 11, URL: "http://x/11.png"}},
	})
	output, _, err := registry.Call(context.Background(), session, ToolCreatePost, "call-1",
		`{"title":"hello","content":"body","image_media_ids":[11]}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "post_id=5") {
		t.Fatalf("unexpected output %q", output)
	}
	if captured.IdempotencyKey != "agent:create:req-1:call-1" {
		t.Fatalf("idempotency key not derived from request: %q", captured.IdempotencyKey)
	}
	if len(captured.MediaIds) != 1 || captured.MediaIds[0] != 11 || captured.Images[0] != "http://x/11.png" {
		t.Fatalf("media linkage mismatch: %+v", captured)
	}

	// 失败路径（AGNT-013）：引用会话外媒体必须失败。
	if _, _, err := registry.Call(context.Background(), session, ToolCreatePost, "call-2",
		`{"title":"hi","content":"b","image_media_ids":[42]}`); err == nil {
		t.Fatal("expected foreign media rejection")
	}
}

func TestDeletePostRequiresConfirmation(t *testing.T) {
	var deleted bool
	var expectedRevision int64 = 4
	content := &fakeContentService{
		getPost: func(_ context.Context, _ *contentservice.GetPostReq) (*contentservice.GetPostResp, error) {
			return &contentservice.GetPostResp{Post: &contentservice.PostInfo{Id: 9, Revision: 4}}, nil
		},
		deletePost: func(_ context.Context, req *contentservice.DeletePostReq) (*contentservice.DeletePostResp, error) {
			deleted = true
			expectedRevision = req.ExpectedRevision
			return &contentservice.DeletePostResp{}, nil
		},
	}
	registry, err := NewToolRegistry(Clients{Content: content}, []string{ToolDeletePost})
	if err != nil {
		t.Fatal(err)
	}

	// 拒绝路径：不执行删除，并把拒绝反馈给模型。
	denied := &fakeBroker{approved: false}
	deniedSession := newTestSession(&Session{UserID: 7, RequestID: "r", Confirms: denied})
	output, _, err := registry.Call(context.Background(), deniedSession, ToolDeletePost, "call-d",
		`{"post_id":9}`)
	if err != nil || deleted {
		t.Fatalf("declined confirm must not delete: deleted=%v err=%v", deleted, err)
	}
	if !strings.Contains(output, "拒绝") {
		t.Fatalf("unexpected decline feedback %q", output)
	}

	// 同意路径：先读版本再删除。
	approvedSession := newTestSession(&Session{UserID: 7, RequestID: "r"})
	if _, _, err := registry.Call(context.Background(), approvedSession, ToolDeletePost, "call-a",
		`{"post_id":9}`); err != nil || !deleted {
		t.Fatalf("approved confirm must delete: deleted=%v err=%v", deleted, err)
	}
	if expectedRevision != 4 {
		t.Fatalf("expected pre-read revision 4, got %d", expectedRevision)
	}

	// 超时路径：视为拒绝（AGNT-021）。
	expired := &fakeBroker{waitErr: ErrConfirmExpired}
	expiredSession := newTestSession(&Session{UserID: 7, RequestID: "r", Confirms: expired})
	output, _, err = registry.Call(context.Background(), expiredSession, ToolDeletePost, "call-e",
		`{"post_id":9}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "已取消") {
		t.Fatalf("expired confirm feedback unexpected: %q", output)
	}
}

func TestUpdatePostRejectsEmptyChange(t *testing.T) {
	registry, err := NewToolRegistry(Clients{}, []string{ToolUpdatePost})
	if err != nil {
		t.Fatal(err)
	}
	session := newTestSession(&Session{UserID: 7})
	if _, _, err := registry.Call(context.Background(), session, ToolUpdatePost, "c",
		`{"post_id":3}`); !errx.Is(err, errx.ParamError) {
		t.Fatalf("expected param error for empty update, got %v", err)
	}
}

func TestWebSearchUnavailableFailsClosed(t *testing.T) {
	registry, err := NewToolRegistry(Clients{}, []string{ToolWebSearch})
	if err != nil {
		t.Fatal(err)
	}
	session := newTestSession(&Session{})
	if _, _, err := registry.Call(context.Background(), session, ToolWebSearch, "c",
		`{"query":"golang"}`); err == nil {
		t.Fatal("expected unavailable error, got nil")
	}
}
