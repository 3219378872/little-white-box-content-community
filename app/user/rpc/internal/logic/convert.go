// storage conversion functions

package logic

import (
	"user/internal/model"
	"user/pb/xiaobaihe/user/pb"
)

func UserProfileToUserInfo(profile *model.UserProfile) *pb.UserInfo {
	return &pb.UserInfo{
		Id:                  profile.Id,
		Username:            profile.Username,
		Nickname:            profile.Nickname.String,
		AvatarUrl:           profile.AvatarUrl.String,
		Bio:                 profile.Bio.String,
		Level:               int32(profile.Level),
		FollowerCount:       profile.FollowerCount,
		FollowingCount:      profile.FollowingCount,
		PostCount:           profile.PostCount,
		LikeCount:           profile.LikeCount,
		CreatedAt:           profile.CreatedAt.Unix(),
		FavoritesVisibility: int32(profile.FavoritesVisibility),
	}
}
