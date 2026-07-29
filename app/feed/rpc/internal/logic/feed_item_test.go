package logic

import (
	"context"
	"testing"

	"esx/app/content/rpc/contentservice"
	"esx/app/feed/rpc/xiaobaihe/feed/pb"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestEnrichFeedItems_BatchesAndDeduplicatesPostIDs(t *testing.T) {
	contentSvc := new(mockContentService)
	baseItems := make([]*pb.FeedItem, 0, 102)
	firstIDs := make([]int64, 0, 100)
	firstPosts := make([]*contentservice.PostInfo, 0, 100)
	for id := int64(1); id <= 101; id++ {
		baseItems = append(baseItems, &pb.FeedItem{PostId: id, Score: float64(id)})
		if id <= 100 {
			firstIDs = append(firstIDs, id)
			firstPosts = append(firstPosts, &contentservice.PostInfo{Id: id, Status: 1})
		}
	}
	baseItems = append(baseItems, &pb.FeedItem{PostId: 1, Score: 999})

	contentSvc.On("GetPostsByIds", mock.Anything, &contentservice.GetPostsByIdsReq{PostIds: firstIDs}).Return(&contentservice.GetPostsByIdsResp{Posts: firstPosts}, nil).Once()
	contentSvc.On("GetPostsByIds", mock.Anything, &contentservice.GetPostsByIdsReq{PostIds: []int64{101}}).Return(&contentservice.GetPostsByIdsResp{Posts: []*contentservice.PostInfo{{Id: 101, Status: 1}}}, nil).Once()

	items, err := enrichFeedItems(context.Background(), contentSvc, baseItems)

	require.NoError(t, err)
	require.Len(t, items, 101)
	require.Equal(t, float64(1), items[0].Score)
	require.Equal(t, int64(101), items[100].PostId)
	contentSvc.AssertExpectations(t)
}
