package logic

import (
	"context"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/rpc/internal/svc"
	"esx/app/assistant/watch"
	"esx/app/user/rpc/userservice"
	pb "esx/kitex_gen/assistant"
	"esx/pkg/errx"
	"fmt"
	"testing"

	"github.com/cloudwego/kitex/client/callopt"
)

type watchConsentUser struct {
	userservice.UserService
	version int32
	granted bool
	err     error
	calls   int
}

func (u *watchConsentUser) GetAgentCapabilityConsent(context.Context, *userservice.GetAgentCapabilityConsentReq, ...callopt.Option) (*userservice.GetAgentCapabilityConsentResp, error) {
	u.calls++
	return &userservice.GetAgentCapabilityConsentResp{Granted: u.granted, ConsentVersion: u.version}, u.err
}

type watchConsentStore struct {
	store.Store
	version int32
	granted bool
	err     error
	calls   int
}

func (s *watchConsentStore) AgentConsent(context.Context, int64) (int32, bool, error) {
	s.calls++
	return s.version, s.granted, s.err
}

func TestWatchMutationsRequireConsent(t *testing.T) {
	for _, state := range []string{"never_granted", "revoked", "outdated_version", "future_version", "read_failure"} {
		t.Run(state, func(t *testing.T) {
			for _, operation := range []string{"create", "enable", "disable", "delete"} {
				t.Run(operation, func(t *testing.T) {
					ctx := context.Background()
					w := watch.NewMapStore()
					u := &watchConsentUser{}
					s := &watchConsentStore{Store: store.NewMemoryStore()}
					if state == "outdated_version" {
						u.version = 1
						s.version = 1
						u.granted = true
						s.granted = true
					}
					if state == "future_version" {
						s.version = 3
						s.granted = true
					}
					if state == "read_failure" {
						u.err = fmt.Errorf("consent unavailable")
						s.err = u.err
					}
					service := &svc.ServiceContext{Watch: w, Store: s, UserService: u}
					var err error
					if operation == "create" {
						_, err = NewCreateWatchTaskLogic(ctx, service).CreateWatchTask(&pb.CreateWatchTaskReq{UserId: 1, ConditionType: watch.KeywordNewPost, TargetType: "keyword", TargetText: "synthetic topic"})
					} else {
						task, e := w.Create(ctx, watch.Task{UserID: 1, ConditionType: watch.KeywordNewPost, TargetType: "keyword", TargetText: "synthetic topic"})
						if e != nil {
							t.Fatal(e)
						}
						if operation == "delete" {
							_, err = NewDeleteWatchTaskLogic(ctx, service).DeleteWatchTask(&pb.DeleteWatchTaskReq{UserId: 1, Id: task.ID, ExpectedVersion: task.Version})
						} else {
							_, err = NewUpdateWatchTaskLogic(ctx, service).UpdateWatchTask(&pb.UpdateWatchTaskReq{UserId: 1, Id: task.ID, ExpectedVersion: task.Version, Enabled: operation == "enable"})
						}
					}
					t.Logf("state=%s operation=%s err=%v RPC_consent_reads=%d store_consent_reads=%d", state, operation, err, u.calls, s.calls)
					if err == nil {
						t.Fatalf("WCH-021: mutation succeeded without valid Agent consent")
					}
					if s.calls == 0 {
						t.Fatal("authoritative consent was not checked")
					}
					tasks, listErr := w.ListTasks(ctx, 1)
					if listErr != nil {
						t.Fatal(listErr)
					}
					if operation == "create" {
						if len(tasks) != 0 {
							t.Fatal("denied create mutated tasks")
						}
					} else if len(tasks) != 1 || tasks[0].Version != 1 || !tasks[0].Enabled {
						t.Fatalf("denied mutation changed task: %+v", tasks)
					}
				})
			}
		})
	}
}

func TestWatchConsentKeepsOwnershipAndRevisionControls(t *testing.T) {
	ctx := context.Background()
	w := watch.NewMapStore()
	service := &svc.ServiceContext{Watch: w, Store: store.NewMemoryStore()}
	task, err := w.Create(ctx, watch.Task{UserID: 1, ConditionType: watch.KeywordNewPost, TargetType: "keyword", TargetText: "topic"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewUpdateWatchTaskLogic(ctx, service).UpdateWatchTask(&pb.UpdateWatchTaskReq{UserId: 2, Id: task.ID, ExpectedVersion: 1, Enabled: false}); !errx.Is(err, errx.NotFound) {
		t.Fatalf("wrong owner update=%v", err)
	}
	if _, err := NewDeleteWatchTaskLogic(ctx, service).DeleteWatchTask(&pb.DeleteWatchTaskReq{UserId: 2, Id: task.ID, ExpectedVersion: 1}); !errx.Is(err, errx.NotFound) {
		t.Fatalf("wrong owner delete=%v", err)
	}
	if _, err := NewUpdateWatchTaskLogic(ctx, service).UpdateWatchTask(&pb.UpdateWatchTaskReq{UserId: 1, Id: task.ID, ExpectedVersion: 99, Enabled: false}); !errx.Is(err, errx.ContentVersionConflict) {
		t.Fatalf("wrong version update=%v", err)
	}
	if _, err := NewDeleteWatchTaskLogic(ctx, service).DeleteWatchTask(&pb.DeleteWatchTaskReq{UserId: 1, Id: task.ID, ExpectedVersion: 99}); !errx.Is(err, errx.ContentVersionConflict) {
		t.Fatalf("wrong version delete=%v", err)
	}
}

func TestWatchMutationsWithCurrentConsent(t *testing.T) {
	ctx := context.Background()
	w := watch.NewMapStore()
	s := &svc.ServiceContext{Watch: w, Store: store.NewMemoryStore()}
	created, err := NewCreateWatchTaskLogic(ctx, s).CreateWatchTask(&pb.CreateWatchTaskReq{UserId: 1, ConditionType: watch.KeywordNewPost, TargetType: "keyword", TargetText: "topic"})
	if err != nil {
		t.Fatal(err)
	}
	for _, enabled := range []bool{false, true} {
		updated, err := NewUpdateWatchTaskLogic(ctx, s).UpdateWatchTask(&pb.UpdateWatchTaskReq{UserId: 1, Id: created.Task.Id, ExpectedVersion: created.Task.Version, Enabled: enabled})
		if err != nil {
			t.Fatal(err)
		}
		created.Task = updated.Task
	}
	if _, err := NewDeleteWatchTaskLogic(ctx, s).DeleteWatchTask(&pb.DeleteWatchTaskReq{UserId: 1, Id: created.Task.Id, ExpectedVersion: created.Task.Version}); err != nil {
		t.Fatal(err)
	}
	tasks, _ := w.ListTasks(ctx, 1)
	if len(tasks) != 0 {
		t.Fatal("delete not applied")
	}
}
