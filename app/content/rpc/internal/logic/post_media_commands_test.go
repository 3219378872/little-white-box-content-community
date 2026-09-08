package logic

import (
	"context"
	"database/sql"
	"testing"

	"esx/app/content/rpc/internal/model"
	"esx/app/content/rpc/internal/svc"
	"esx/app/content/rpc/pb/xiaobaihe/content/pb"
	mediapb "esx/app/media/rpc/pb/xiaobaihe/media/pb"
	"esx/pkg/errx"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestCreatePostRejectsUnverifiedImageURLs(t *testing.T) {
	l := NewCreatePostLogic(context.Background(), &svc.ServiceContext{})
	response, err := l.CreatePost(&pb.CreatePostReq{AuthorId: 1, Title: "title", Content: "content", Status: 1, Images: []string{"https://unowned.invalid/image.jpg"}})
	require.Nil(t, response)
	require.True(t, errx.Is(err, errx.ParamError))
	images, err := createPostImages([]string{"owned.jpg"}, []string{"owned.jpg"})
	require.NoError(t, err)
	assert.Equal(t, []string{"owned.jpg"}, images)
	_, err = createPostImages([]string{"unowned.jpg"}, []string{"owned.jpg"})
	assert.True(t, errx.Is(err, errx.ParamError))
}

func TestUpdatePostMediaPresenceSurvivesRPC(t *testing.T) {
	post := &model.Post{Title: "title", Content: "body", Status: 1, Images: sql.NullString{String: "old.jpg", Valid: true}, MediaIds: sql.NullString{String: "[10]", Valid: true}}
	for _, test := range []struct {
		name    string
		request *pb.UpdatePostReq
		clear   bool
	}{
		{"omitted", &pb.UpdatePostReq{Title: "new"}, false},
		{"images empty", &pb.UpdatePostReq{ImagesProvided: true, Images: []string{}}, true},
		{"media ids empty", &pb.UpdatePostReq{MediaIdsProvided: true, MediaIds: []int64{}}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			wire, err := proto.Marshal(test.request)
			require.NoError(t, err)
			request := new(pb.UpdatePostReq)
			require.NoError(t, proto.Unmarshal(wire, request))
			patch, err := NewUpdatePostLogic(context.Background(), nil).mergePostFields(request, post, nil)
			require.NoError(t, err)
			if !test.clear {
				assert.NotContains(t, patch.fields, "images")
				assert.NotContains(t, patch.fields, "media_ids")
				return
			}
			encoded, err := patch.fields["images"].(*model.JSONField[[]string]).JSONString()
			require.NoError(t, err)
			assert.Equal(t, "[]", encoded)
			assert.Equal(t, sql.NullString{}, patch.fields["media_ids"])
		})
	}
}

func TestUpdatePostRetainsOwnedMediaAndRejectsForeignURLs(t *testing.T) {
	media := &fakeMediaValidator{response: &mediapb.BatchGetMediaResp{Medias: []*mediapb.MediaInfo{{Id: 10, UserId: 1, Status: 1, Url: "old.jpg"}}}}
	l := NewUpdatePostLogic(context.Background(), &svc.ServiceContext{MediaService: media})
	post := &model.Post{Title: "title", Content: "body", Status: 1, Images: sql.NullString{String: "legacy.jpg,old.jpg", Valid: true}, MediaIds: sql.NullString{String: "[10]", Valid: true}}
	request := &pb.UpdatePostReq{AuthorId: 1, Images: []string{"old.jpg", "new.jpg"}, MediaIds: []int64{20}}
	patch, err := l.mergePostFields(request, post, []string{"new.jpg"})
	require.NoError(t, err)
	assert.Equal(t, sql.NullString{String: "[10,20]", Valid: true}, patch.fields["media_ids"])
	request.Images = []string{"foreign.jpg", "new.jpg"}
	_, err = l.mergePostFields(request, post, []string{"new.jpg"})
	assert.True(t, errx.Is(err, errx.ParamError))
	request.Images = []string{"old.jpg"}
	_, err = l.mergePostFields(request, post, []string{"new.jpg"})
	assert.True(t, errx.Is(err, errx.ParamError))
	request.Images = []string{"legacy.jpg"}
	request.MediaIds = nil
	patch, err = l.mergePostFields(request, post, nil)
	require.NoError(t, err)
	assert.Equal(t, sql.NullString{}, patch.fields["media_ids"])
}

func TestUpdatePostRecoversFromInvalidExistingMediaWithoutRetainingURLs(t *testing.T) {
	post := &model.Post{Title: "title", Content: "body", Status: 1, Images: sql.NullString{String: "deleted.jpg,retained.jpg", Valid: true}, MediaIds: sql.NullString{String: "[10,11]", Valid: true}}
	media := &fakeMediaValidator{response: &mediapb.BatchGetMediaResp{Medias: []*mediapb.MediaInfo{{Id: 11, UserId: 1, Status: 1, Url: "retained.jpg"}}}}
	l := NewUpdatePostLogic(context.Background(), &svc.ServiceContext{MediaService: media})
	_, err := l.mergePostFields(&pb.UpdatePostReq{AuthorId: 1, Images: []string{"retained.jpg"}}, post, nil)
	require.True(t, errx.Is(err, errx.ParamError))

	patch, err := l.mergePostFields(&pb.UpdatePostReq{AuthorId: 1, ImagesProvided: true}, post, nil)
	require.NoError(t, err)
	encoded, err := patch.fields["images"].(*model.JSONField[[]string]).JSONString()
	require.NoError(t, err)
	assert.Equal(t, "[]", encoded)
	assert.Equal(t, sql.NullString{}, patch.fields["media_ids"])

	request := &pb.UpdatePostReq{AuthorId: 1, MediaIds: []int64{11}, MediaIdsProvided: true}
	urls, err := validatePostMedia(l.ctx, l.Logger, media, request.AuthorId, request.MediaIds)
	require.NoError(t, err)
	patch, err = l.mergePostFields(request, post, urls)
	require.NoError(t, err)
	encoded, err = patch.fields["images"].(*model.JSONField[[]string]).JSONString()
	require.NoError(t, err)
	assert.Equal(t, `["retained.jpg"]`, encoded)
	assert.Equal(t, sql.NullString{String: "[11]", Valid: true}, patch.fields["media_ids"])
}
