package svc

import (
	"testing"

	"esx/app/user/rpc/userservice"
)

func TestCurrentConsentGrantedRequiresLatestVersion(t *testing.T) {
	tests := []struct {
		consent *userservice.GetAgentCapabilityConsentResp
		want    bool
	}{
		{consent: nil, want: false},
		{consent: &userservice.GetAgentCapabilityConsentResp{Granted: false, ConsentVersion: 2, CurrentVersion: 2}, want: false},
		{consent: &userservice.GetAgentCapabilityConsentResp{Granted: true, ConsentVersion: 1, CurrentVersion: 2}, want: false},
		{consent: &userservice.GetAgentCapabilityConsentResp{Granted: true, ConsentVersion: 2, CurrentVersion: 2}, want: true},
		{consent: &userservice.GetAgentCapabilityConsentResp{Granted: true, ConsentVersion: 2, CurrentVersion: 3}, want: false},
	}
	for _, test := range tests {
		if got := currentConsentGranted(test.consent); got != test.want {
			t.Fatalf("consent=%+v got=%v want=%v", test.consent, got, test.want)
		}
	}
}
