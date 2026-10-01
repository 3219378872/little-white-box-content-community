package logic

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"esx/app/ad/internal/serving"
	"esx/app/ad/internal/store"
	"esx/app/ad/rpc/internal/config"
	"esx/app/ad/rpc/internal/svc"
	"esx/pkg/errx"
	redis "esx/pkg/redisstore"
)

var testNow = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

// fakeStore 是 svc.Store 的内存实现：每个方法返回预置结果与 err，并记录调用。
type fakeStore struct {
	advertiser *store.AdvertiserWithQualifications
	ad         *store.AdWithSnapshots
	ads        []store.AdWithSnapshots
	snapshots  map[int64]*store.Snapshot
	quals      map[int64][]store.Qualification
	asset      *store.Asset
	assetID    int64

	err, qualsErr, idErr, snapshotErr error

	calls           map[string]int
	advertiserInput store.AdvertiserInput
	qualInput       store.QualificationInput
	adInput         store.AdInput
	createdAsset    store.Asset
}

func (f *fakeStore) record(name string) {
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[name]++
}

func (f *fakeStore) ApplyAdvertiser(_ context.Context, in store.AdvertiserInput, _ time.Time) (*store.AdvertiserWithQualifications, error) {
	f.record("ApplyAdvertiser")
	f.advertiserInput = in
	return f.advertiser, f.err
}

func (f *fakeStore) GetAdvertiserByUser(context.Context, int64) (*store.AdvertiserWithQualifications, error) {
	f.record("GetAdvertiserByUser")
	return f.advertiser, f.err
}

func (f *fakeStore) AddQualification(_ context.Context, in store.QualificationInput, _ time.Time) (*store.AdvertiserWithQualifications, error) {
	f.record("AddQualification")
	f.qualInput = in
	return f.advertiser, f.err
}

func (f *fakeStore) CreateAd(_ context.Context, _ int64, in store.AdInput, _ string, _ time.Time) (*store.AdWithSnapshots, error) {
	f.record("CreateAd")
	f.adInput = in
	return f.ad, f.err
}

func (f *fakeStore) UpdateAd(_ context.Context, _, _, _ int64, in store.AdInput, _ string, _ time.Time) (*store.AdWithSnapshots, error) {
	f.record("UpdateAd")
	f.adInput = in
	return f.ad, f.err
}

func (f *fakeStore) GetAd(context.Context, int64, int64) (*store.AdWithSnapshots, error) {
	f.record("GetAd")
	return f.ad, f.err
}

func (f *fakeStore) ListAds(context.Context, int64, int64, int) ([]store.AdWithSnapshots, int64, bool, error) {
	f.record("ListAds")
	return f.ads, 42, true, f.err
}

func (f *fakeStore) SnapshotAt(_ context.Context, adID, _ int64) (*store.Snapshot, error) {
	f.record("SnapshotAt")
	if f.snapshotErr != nil {
		return nil, f.snapshotErr
	}
	snap, ok := f.snapshots[adID]
	if !ok {
		return nil, store.ErrNotFound
	}
	return snap, nil
}

func (f *fakeStore) QualificationsOf(_ context.Context, advertiserID int64) ([]store.Qualification, error) {
	f.record("QualificationsOf")
	return f.quals[advertiserID], f.qualsErr
}

func (f *fakeStore) NewAssetID() (int64, error) {
	f.record("NewAssetID")
	return f.assetID, f.idErr
}

func (f *fakeStore) CreateAsset(_ context.Context, asset store.Asset, _ string, _ time.Time) (*store.Asset, error) {
	f.record("CreateAsset")
	f.createdAsset = asset
	if f.err != nil {
		return nil, f.err
	}
	return &asset, nil
}

func (f *fakeStore) GetAsset(context.Context, int64) (*store.Asset, error) {
	f.record("GetAsset")
	return f.asset, f.err
}

type fakeAssets struct {
	objects        map[string][]byte
	putErr, getErr error
}

func (f *fakeAssets) PutPrivate(_ context.Context, key string, content []byte, _ string) error {
	if f.putErr != nil {
		return f.putErr
	}
	if f.objects == nil {
		f.objects = map[string][]byte{}
	}
	f.objects[key] = content
	return nil
}

func (f *fakeAssets) GetPrivate(_ context.Context, key string) ([]byte, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.objects[key], nil
}

// fakeKV 模拟 serving 用到的 Redis 子集：哈希、带过期字符串，以及计频与隐藏脚本。
type fakeKV struct {
	hashes  map[string]map[string]string
	strings map[string]string
	capped  map[string]bool

	getErr, hgetallErr, evalErr, setErr error
	reserveCalls                        int
}

func newFakeKV() *fakeKV {
	return &fakeKV{hashes: map[string]map[string]string{}, strings: map[string]string{}, capped: map[string]bool{}}
}

func (f *fakeKV) EvalCtx(_ context.Context, script string, keys []string, args ...any) (any, error) {
	if f.evalErr != nil {
		return nil, f.evalErr
	}
	if strings.Contains(script, "HINCRBY") {
		f.reserveCalls++
		var granted []any
		for _, arg := range args[:len(args)-2] {
			if id := arg.(string); !f.capped[id] {
				granted = append(granted, id)
			}
		}
		return granted, nil
	}
	hash := f.hashes[keys[0]]
	if hash == nil {
		hash = map[string]string{}
		f.hashes[keys[0]] = hash
	}
	hash[args[0].(string)] = args[1].(string)
	return int64(1), nil
}

func (f *fakeKV) GetCtx(_ context.Context, key string) (string, error) {
	if f.getErr != nil {
		return "", f.getErr
	}
	value, ok := f.strings[key]
	if !ok {
		return "", redis.Nil
	}
	return value, nil
}

func (f *fakeKV) SetexCtx(_ context.Context, key, value string, _ int) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.strings[key] = value
	return nil
}

func (f *fakeKV) HgetallCtx(_ context.Context, key string) (map[string]string, error) {
	if f.hgetallErr != nil {
		return nil, f.hgetallErr
	}
	return f.hashes[key], nil
}

func newSvcCtx(fs *fakeStore) (*svc.ServiceContext, *fakeAssets, *fakeKV) {
	assets, kv := &fakeAssets{}, newFakeKV()
	var c config.Config
	c.Assets.PublicBaseURL = "https://cdn.example.test/xbh-media"
	return &svc.ServiceContext{
		Config: c, Store: fs, Assets: assets, Redis: kv, Index: serving.Index{KV: kv},
		Cache: svc.NewCache(time.Minute), Clock: func() time.Time { return testNow },
	}, assets, kv
}

func requireCode(t *testing.T, err error, code int) {
	t.Helper()
	require.Error(t, err)
	require.Equal(t, code, errx.GetCode(err), "err=%v", err)
}

var errBoom = errors.New("boom")
