package main

import (
	"context"

	"github.com/cloudwego/kitex/client/callopt"

	"esx/app/ad/rpc/adservice"
	"esx/app/review/rpc/reviewservice"
	adpb "esx/kitex_gen/ad"
	reviewpb "esx/kitex_gen/review"
	"esx/pkg/errx"
)

// contractAdService 与 contractReviewService 只提供路由契约所需的最小响应。
type contractAdService struct{ adservice.AdService }

func contractAdvertiser() *adservice.AdvertiserResp {
	return &adservice.AdvertiserResp{Found: true, Advertiser: &adpb.AdvertiserView{
		AdvertiserId: 5, Name: "Acme", Markets: []string{"US"}, Revision: 1, ReviewStatus: "pending_review",
	}}
}

func contractAd() *adservice.AdResp {
	return &adservice.AdResp{Ad: &adpb.AdView{AdId: 7, Revision: 1, ReviewStatus: "pending_review", ServingStatus: "none",
		Latest: &adpb.AdContent{Title: "Beans", LandingUrl: "https://coffee.shop.com", LandingDomain: "coffee.shop.com"}}}
}

func (contractAdService) GetMyAdvertiser(context.Context, *adservice.GetMyAdvertiserReq, ...callopt.Option) (*adservice.AdvertiserResp, error) {
	return contractAdvertiser(), nil
}
func (contractAdService) ApplyAdvertiser(context.Context, *adservice.ApplyAdvertiserReq, ...callopt.Option) (*adservice.AdvertiserResp, error) {
	return contractAdvertiser(), nil
}
func (contractAdService) AddQualification(context.Context, *adservice.AddQualificationReq, ...callopt.Option) (*adservice.AdvertiserResp, error) {
	return contractAdvertiser(), nil
}
func (contractAdService) UploadAsset(_ context.Context, in *adservice.UploadAssetReq, _ ...callopt.Option) (*adservice.UploadAssetResp, error) {
	return &adservice.UploadAssetResp{Asset: &adpb.AssetView{AssetId: 9, Kind: in.Kind, Sha256: "abc", MimeType: "image/png", Size: int64(len(in.Content))}}, nil
}
func (contractAdService) ReadAsset(context.Context, *adservice.ReadAssetReq, ...callopt.Option) (*adservice.ReadAssetResp, error) {
	return &adservice.ReadAssetResp{Content: []byte{1, 2}, MimeType: "image/png"}, nil
}
func (contractAdService) ListAds(context.Context, *adservice.ListAdsReq, ...callopt.Option) (*adservice.ListAdsResp, error) {
	return &adservice.ListAdsResp{Ads: []*adpb.AdView{contractAd().Ad}}, nil
}
func (contractAdService) CreateAd(context.Context, *adservice.CreateAdReq, ...callopt.Option) (*adservice.AdResp, error) {
	return contractAd(), nil
}
func (contractAdService) GetAd(context.Context, *adservice.GetAdReq, ...callopt.Option) (*adservice.AdResp, error) {
	return contractAd(), nil
}
func (contractAdService) UpdateAd(context.Context, *adservice.UpdateAdReq, ...callopt.Option) (*adservice.AdResp, error) {
	return contractAd(), nil
}
func (contractAdService) HideAd(context.Context, *adservice.HideAdReq, ...callopt.Option) (*adservice.HideAdResp, error) {
	return &adservice.HideAdResp{}, nil
}
func (contractAdService) ReportAd(context.Context, *adservice.ReportAdReq, ...callopt.Option) (*adservice.ReportAdResp, error) {
	return &adservice.ReportAdResp{Counted: true}, nil
}
func (contractAdService) AppealAd(_ context.Context, in *adservice.AppealAdReq, _ ...callopt.Option) (*adservice.AdResp, error) {
	if in.AdId == 8 {
		return nil, errx.NewWithCode(errx.AdAppealNotAllowed)
	}
	return contractAd(), nil
}
func (contractAdService) GetSponsoredSlots(context.Context, *adservice.GetSponsoredSlotsReq, ...callopt.Option) (*adservice.GetSponsoredSlotsResp, error) {
	return &adservice.GetSponsoredSlotsResp{}, nil
}

type contractReviewService struct{ reviewservice.ReviewService }

func contractTask() *reviewservice.TaskResp {
	return &reviewservice.TaskResp{Found: true, Task: &reviewpb.TaskView{TaskId: 3, BizType: "ad_creative", ObjectId: 7,
		ObjectRevision: 1, Purpose: "initial", Status: "claimed", LeaseGeneration: 1, SnapshotJson: "{}"}}
}

func (contractReviewService) GetReviewer(context.Context, *reviewservice.GetReviewerReq, ...callopt.Option) (*reviewservice.GetReviewerResp, error) {
	return &reviewservice.GetReviewerResp{Active: true, Roles: []string{"reviewer"}, Markets: []string{"US"}, Languages: []string{"en"}}, nil
}
func (contractReviewService) GetQueueSummary(context.Context, *reviewservice.GetQueueSummaryReq, ...callopt.Option) (*reviewservice.GetQueueSummaryResp, error) {
	return &reviewservice.GetQueueSummaryResp{PolicyVersion: "v1"}, nil
}
func (contractReviewService) ClaimTask(context.Context, *reviewservice.ClaimTaskReq, ...callopt.Option) (*reviewservice.TaskResp, error) {
	return contractTask(), nil
}
func (contractReviewService) GetTask(context.Context, *reviewservice.GetTaskReq, ...callopt.Option) (*reviewservice.TaskResp, error) {
	return contractTask(), nil
}
func (contractReviewService) RenewTask(_ context.Context, in *reviewservice.LeaseReq, _ ...callopt.Option) (*reviewservice.TaskResp, error) {
	if in.LeaseGeneration != 1 {
		return nil, errx.NewWithCode(errx.ReviewLeaseLost)
	}
	return contractTask(), nil
}
func (contractReviewService) ReleaseTask(context.Context, *reviewservice.LeaseReq, ...callopt.Option) (*reviewservice.ReleaseTaskResp, error) {
	return &reviewservice.ReleaseTaskResp{}, nil
}
func (contractReviewService) SubmitDecision(_ context.Context, in *reviewservice.SubmitDecisionReq, _ ...callopt.Option) (*reviewservice.SubmitDecisionResp, error) {
	if in.UserId == 999 {
		return nil, errx.NewWithCode(errx.ReviewRoleRequired)
	}
	return &reviewservice.SubmitDecisionResp{DecisionId: 11, Verdict: in.Verdict, PolicyCodes: in.PolicyCodes, PolicyVersion: "v1"}, nil
}
func (contractReviewService) ListPolicies(context.Context, *reviewservice.ListPoliciesReq, ...callopt.Option) (*reviewservice.ListPoliciesResp, error) {
	return &reviewservice.ListPoliciesResp{PolicyVersion: "v1", Codes: []*reviewpb.PolicyCode{{Code: "LANDING.URL", Title: "落地页地址不合规"}}, Markets: []string{"US"}}, nil
}
func (contractReviewService) ListSeeds(context.Context, *reviewservice.ListSeedsReq, ...callopt.Option) (*reviewservice.ListSeedsResp, error) {
	return &reviewservice.ListSeedsResp{}, nil
}
func (contractReviewService) ConfirmSeed(context.Context, *reviewservice.SeedActionReq, ...callopt.Option) (*reviewservice.SeedResp, error) {
	return &reviewservice.SeedResp{Seed: &reviewpb.SeedView{SeedId: 4, Status: "active"}}, nil
}
func (contractReviewService) RetireSeed(context.Context, *reviewservice.SeedActionReq, ...callopt.Option) (*reviewservice.SeedResp, error) {
	return &reviewservice.SeedResp{Seed: &reviewpb.SeedView{SeedId: 4, Status: "retired"}}, nil
}
func (contractReviewService) AuthorizeEvidenceMedia(_ context.Context, in *reviewservice.AuthorizeEvidenceMediaReq, _ ...callopt.Option) (*reviewservice.AuthorizeEvidenceMediaResp, error) {
	return &reviewservice.AuthorizeEvidenceMediaResp{Allowed: in.MediaId == 9}, nil
}
