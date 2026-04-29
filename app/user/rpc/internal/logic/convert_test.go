package logic

import (
	"database/sql"
	"testing"
	"time"

	"user/internal/model"
	"user/pb/xiaobaihe/user/pb"

	"github.com/stretchr/testify/assert"
)

func TestUserProfileToUserInfo(t *testing.T) {
	tests := []struct {
		name    string
		profile *model.UserProfile
		check   func(t *testing.T, info *pb.UserInfo)
	}{
		{
			name: "完整转换",
			profile: &model.UserProfile{
				Id:                  1,
				Username:            "alice",
				Nickname:            sql.NullString{String: "Alice", Valid: true},
				AvatarUrl:           sql.NullString{String: "http://a.jpg", Valid: true},
				Bio:                 sql.NullString{String: "hi", Valid: true},
				Level:               5,
				FollowerCount:       10,
				FollowingCount:      20,
				PostCount:           30,
				LikeCount:           40,
				CreatedAt:           time.Unix(1710000000, 0),
				FavoritesVisibility: 1,
			},
			check: func(t *testing.T, info *pb.UserInfo) {
				assert.Equal(t, int64(1), info.Id)
				assert.Equal(t, "alice", info.Username)
				assert.Equal(t, "Alice", info.Nickname)
				assert.Equal(t, "http://a.jpg", info.AvatarUrl)
				assert.Equal(t, "hi", info.Bio)
				assert.Equal(t, int32(5), info.Level)
				assert.Equal(t, int64(10), info.FollowerCount)
				assert.Equal(t, int64(30), info.PostCount)
				assert.Equal(t, int64(40), info.LikeCount)
				assert.Equal(t, int64(1710000000), info.CreatedAt)
				assert.Equal(t, int32(1), info.FavoritesVisibility)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := UserProfileToUserInfo(tt.profile)
			tt.check(t, result)
		})
	}
}
