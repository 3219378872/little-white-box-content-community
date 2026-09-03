package postmap

import (
	"reflect"
	"testing"

	"esx/app/content/rpc/contentservice"
	"esx/app/gateway/internal/logic/authorx"
	"esx/app/gateway/internal/types"
)

func samplePost() *contentservice.PostInfo {
	return &contentservice.PostInfo{
		Id: 100, AuthorId: 7, Title: "t", Content: "c",
		Images: []string{"i"}, Tags: []string{"tag"}, Status: 1,
		ViewCount: 1, LikeCount: 2, CommentCount: 3, FavoriteCount: 4,
		Revision: 5, CreatedAt: 6, MediaIds: []int64{9},
	}
}

func TestItem_FillsViewerAndAuthor(t *testing.T) {
	item := Item(samplePost(), map[int64]bool{100: true}, map[int64]bool{100: true}, authorx.Author{Name: "Alice", Avatar: "https://a"})
	if item.FavoriteCount != 4 || !item.IsLiked || !item.IsFavorited {
		t.Fatalf("counts/flags: %+v", item)
	}
	if len(item.MediaIds) != 1 || item.MediaIds[0] != 9 {
		t.Fatalf("media ids: %+v", item.MediaIds)
	}
	if item.AuthorName != "Alice" || item.AuthorAvatar != "https://a" {
		t.Fatalf("author: %+v", item)
	}
}

func TestItem_NilPost(t *testing.T) {
	if !reflect.DeepEqual(Item(nil, nil, nil, authorx.Author{}), types.PostItem{}) {
		t.Fatal("nil post should yield zero item")
	}
}

func TestDetail_MatchesItem(t *testing.T) {
	author := authorx.Author{Name: "Alice", Avatar: "https://a"}
	item := Item(samplePost(), map[int64]bool{100: true}, nil, author)
	detail := Detail(samplePost(), map[int64]bool{100: true}, nil, author)
	if !reflect.DeepEqual(types.GetPostResp(item), detail) {
		t.Fatalf("detail mismatch: item=%+v detail=%+v", item, detail)
	}
}

func TestItems_SkipsNilAndLooksUpAuthor(t *testing.T) {
	list := Items([]*contentservice.PostInfo{nil, samplePost()}, nil, nil, map[int64]authorx.Author{
		7: {Name: "Alice"},
	})
	if len(list) != 1 || list[0].Id != 100 || list[0].AuthorName != "Alice" {
		t.Fatalf("unexpected list %+v", list)
	}
}
