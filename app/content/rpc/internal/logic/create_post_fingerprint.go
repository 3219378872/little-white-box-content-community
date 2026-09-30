package logic

import (
	"strconv"
	"strings"

	pb "esx/kitex_gen/content"
	"esx/pkg/idempotencyx"
)

func createPostIdempotencyRecord(in *pb.CreatePostReq) idempotencyx.IdempotencyRecord {
	// Counts preserve array boundaries. Keep input order: attachment order is
	// observable, and tags must not collide merely because a tag contains commas.
	parts := []string{in.Title, in.Content, strconv.Itoa(len(in.Images))}
	parts = append(parts, in.Images...)
	parts = append(parts, strconv.Itoa(len(in.Tags)))
	parts = append(parts, in.Tags...)
	parts = append(parts, strconv.Itoa(len(in.MediaIds)))
	for _, id := range in.MediaIds {
		parts = append(parts, strconv.FormatInt(id, 10))
	}
	parts = append(parts, strconv.FormatInt(int64(in.Status), 10))
	record := idempotencyx.IdempotencyRecord{
		Scope: "post:create", UserID: in.AuthorId, Key: strings.TrimSpace(in.IdempotencyKey),
		CommandHash: idempotencyx.VersionedCommandHash("post:create:v2", parts...),
	}
	// No historical payload was retained. Mutable post/tag rows cannot prove
	// what was originally submitted. Permit only an injective legacy subset;
	// ambiguous old keys fail closed as conflicts instead of replaying another
	// command or creating a duplicate. See the rollout guide.
	if legacyCreatePostUnambiguous(in) {
		record.LegacyCommandHash = idempotencyx.CommandHash(in.Title, in.Content,
			strings.Join(in.Images, ","), strings.Join(in.Tags, ","),
			strings.Join(sortedMediaIDs(in.MediaIds), ","), strconv.FormatInt(int64(in.Status), 10))
	}
	return record
}

func legacyCreatePostUnambiguous(in *pb.CreatePostReq) bool {
	if strings.ContainsRune(in.Title, 0) || strings.ContainsRune(in.Content, 0) ||
		len(in.Tags) > 1 || len(in.MediaIds) > 1 {
		return false
	}
	for _, tag := range in.Tags {
		if strings.ContainsAny(tag, ",\x00") {
			return false
		}
	}
	for _, image := range in.Images {
		if image == "" || strings.ContainsAny(image, ",\x00") {
			return false
		}
	}
	return true
}
