package tool

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"esx/app/assistant/internal/store"
	"esx/app/content/rpc/contentservice"
	"esx/app/content/visibility"
	"esx/pkg/errx"
	"fmt"
	"strings"

	"github.com/zeromicro/go-zero/core/logx"
)

func (r *Registry) bindSources(ctx context.Context, session *Session, sources []store.SourceRef, text string) (string, error) {
	var retrieved []map[string]any
	var b strings.Builder
	b.WriteString(text)
	b.WriteString("\n来源 handle（仅可对本 run 使用 present_sources）：")
	for _, src := range sources {
		handle := src.Handle
		if handle == "" {
			handle = randomHandle()
		}
		payload, _ := json.Marshal(src)
		insert := func(ctx context.Context, target store.Store) error {
			if session.Fence.Generation > 0 {
				run, err := target.GetRun(ctx, session.RunID)
				if err != nil {
					return err
				}
				if run.CancelRequested {
					return errx.New(errx.ParamError, "run was cancelled")
				}
			}
			_, err := target.InsertSource(ctx, store.Source{
				RunID: session.RunID, Handle: handle, Kind: src.Kind, AuthorityID: src.AuthorityID,
				Revision: src.Revision, PayloadJSON: string(payload), CreatedAtMs: store.NowMs(),
			})
			if err != nil {
				return err
			}
			if session.ClientProtocolVersion >= 2 {
				fragments := src.Evidence
				if excerpt := sourceExcerpt(src); excerpt != "" {
					fragments = append([]store.Evidence{evidenceFor(session.RunID, handle, src.Kind, excerpt, "")}, fragments...)
				}
				for _, fragment := range fragments {
					fragment = evidenceFor(session.RunID, handle, fragment.Kind, fragment.Text, fragment.CommentID)
					if err := target.PutEvidence(ctx, fragment); err != nil {
						return err
					}
				}
			}
			return nil
		}
		var err error
		if session.Fence.Generation > 0 {
			err = r.store.RunStep(ctx, session.Fence, insert)
		} else {
			err = insert(ctx, r.store)
		}
		if err != nil {
			logx.WithContext(ctx).Errorw("source ledger insert failed", logx.Field("err", err.Error()))
			return "", fmt.Errorf("source ledger insert: %w", err)
		}
		fmt.Fprintf(&b, "\n- %s (%s)", handle, src.Kind)
		if session.ClientProtocolVersion >= 2 {
			fragments, err := r.store.ListEvidence(ctx, session.RunID, handle)
			if err != nil {
				return "", err
			}
			retrieved = append(retrieved, map[string]any{"handle": handle, "kind": src.Kind, "title": src.Title, "retrieved_evidence": fragments})
		}
	}
	if session.ClientProtocolVersion >= 2 {
		raw, err := json.Marshal(map[string]any{"sources": retrieved})
		return string(raw), err
	}
	return b.String(), nil
}

func randomHandle() string {
	var buf [8]byte
	_, _ = rand.Read(buf[:])
	return "src_" + hex.EncodeToString(buf[:])
}

func publishedPosts(ctx context.Context, client contentservice.ContentService, ids []int64) (map[int64]*contentservice.PostInfo, error) {
	return visibility.PublishedByIDs(ctx, client, ids)
}
