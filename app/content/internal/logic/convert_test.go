package logic

import (
	"database/sql"
	"esx/app/content/internal/model"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPostToPostInfo_AllFields(t *testing.T) {
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	post := &model.Post{
		Id:            100,
		AuthorId:      200,
		Title:         "测试标题",
		Content:       "测试内容",
		Images:        sql.NullString{String: "img1.jpg,img2.jpg", Valid: true},
		Status:        1,
		ViewCount:     10,
		LikeCount:     20,
		CommentCount:  5,
		FavoriteCount: 3,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	tags := []string{"tag1", "tag2"}

	info := PostToPostInfo(post, tags)

	assert.Equal(t, int64(100), info.Id)
	assert.Equal(t, int64(200), info.AuthorId)
	assert.Equal(t, "测试标题", info.Title)
	assert.Equal(t, "测试内容", info.Content)
	assert.Equal(t, []string{"img1.jpg", "img2.jpg"}, info.Images)
	assert.Equal(t, tags, info.Tags)
	assert.Equal(t, int32(1), info.Status)
	assert.Equal(t, int64(10), info.ViewCount)
	assert.Equal(t, int64(20), info.LikeCount)
	assert.Equal(t, int64(5), info.CommentCount)
	assert.Equal(t, int64(3), info.FavoriteCount)
	assert.Equal(t, now.UnixMilli(), info.CreatedAt)
	assert.Equal(t, now.UnixMilli(), info.UpdatedAt)
}

func TestPostToPostInfo_NullImages(t *testing.T) {
	post := &model.Post{
		Images: sql.NullString{Valid: false},
	}

	info := PostToPostInfo(post, nil)

	assert.Equal(t, []string{}, info.Images)
	assert.Equal(t, []string{}, info.Tags)
}

func TestPostToPostInfo_EmptyImages(t *testing.T) {
	post := &model.Post{
		Images: sql.NullString{String: "", Valid: true},
	}

	info := PostToPostInfo(post, nil)

	assert.Equal(t, []string{}, info.Images)
}

func TestCommentToCommentInfo_TopLevel(t *testing.T) {
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	comment := &model.Comment{
		Id:          1,
		PostId:      100,
		UserId:      200,
		ParentId:    sql.NullInt64{Valid: false}, // 一级评论
		ReplyUserId: sql.NullInt64{Valid: false},
		Content:     "评论内容",
		Status:      1,
		LikeCount:   5,
		CreatedAt:   now,
	}

	info := CommentToCommentInfo(comment)

	assert.Equal(t, int64(1), info.Id)
	assert.Equal(t, int64(100), info.PostId)
	assert.Equal(t, int64(200), info.UserId)
	assert.Equal(t, int64(0), info.ParentId)    // NULL -> 0
	assert.Equal(t, int64(0), info.ReplyUserId) // NULL -> 0
	assert.Equal(t, "评论内容", info.Content)
	assert.Equal(t, int32(1), info.Status)
	assert.Equal(t, int64(5), info.LikeCount)
	assert.Equal(t, now.UnixMilli(), info.CreatedAt)
}

func TestCommentToCommentInfo_Reply(t *testing.T) {
	comment := &model.Comment{
		ParentId:    sql.NullInt64{Int64: 10, Valid: true},
		ReplyUserId: sql.NullInt64{Int64: 300, Valid: true},
	}

	info := CommentToCommentInfo(comment)

	assert.Equal(t, int64(10), info.ParentId)
	assert.Equal(t, int64(300), info.ReplyUserId)
}

func TestTagToTagInfo(t *testing.T) {
	tag := &model.Tag{
		Id:        1,
		Name:      "Go语言",
		PostCount: 42,
	}

	info := TagToTagInfo(tag)

	assert.Equal(t, int64(1), info.Id)
	assert.Equal(t, "Go语言", info.Name)
	assert.Equal(t, int64(42), info.PostCount)
}
