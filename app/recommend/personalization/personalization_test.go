package personalization

import (
	"context"
	"errors"
	"testing"

	"github.com/cloudwego/kitex/client/callopt"
	"github.com/stretchr/testify/require"

	"esx/app/user/rpc/userservice"
)

type fakeMarker struct {
	values map[string]string
	err    error
}

func (m *fakeMarker) GetCtx(_ context.Context, key string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.values[key], nil
}

type fakeReader struct {
	resp  *userservice.GetPersonalizationPreferenceResp
	err   error
	calls int
}

func (r *fakeReader) GetPersonalizationPreference(_ context.Context, _ *userservice.GetPersonalizationPreferenceReq, _ ...callopt.Option) (*userservice.GetPersonalizationPreferenceResp, error) {
	r.calls++
	return r.resp, r.err
}

func TestOptedOut(t *testing.T) {
	enabled := &userservice.GetPersonalizationPreferenceResp{Enabled: true}
	disabled := &userservice.GetPersonalizationPreferenceResp{Enabled: false}
	cases := []struct {
		name        string
		marker      MarkerGetter
		reader      *fakeReader
		noReader    bool
		userID      int64
		want        bool
		wantErr     string
		readerCalls int
	}{
		{name: "anonymous user is never opted out", marker: &fakeMarker{}, reader: &fakeReader{resp: disabled}, userID: 0, want: false},
		{name: "marker short-circuits authority", marker: &fakeMarker{values: map[string]string{"personalization:optout:42": "1"}}, reader: &fakeReader{resp: enabled}, userID: 42, want: true},
		{name: "missing marker defers to enabled authority", marker: &fakeMarker{}, reader: &fakeReader{resp: enabled}, userID: 42, want: false, readerCalls: 1},
		{name: "missing marker defers to disabled authority", marker: &fakeMarker{}, reader: &fakeReader{resp: disabled}, userID: 42, want: true, readerCalls: 1},
		{name: "marker read failure falls back to authority", marker: &fakeMarker{err: errors.New("cache down")}, reader: &fakeReader{resp: enabled}, userID: 42, want: false, readerCalls: 1},
		{name: "store without marker support uses authority", reader: &fakeReader{resp: enabled}, userID: 42, want: false, readerCalls: 1},
		{name: "unconfigured authority fails closed", marker: &fakeMarker{}, noReader: true, userID: 42, want: true, wantErr: "unavailable"},
		{name: "authority error fails closed", marker: &fakeMarker{}, reader: &fakeReader{err: errors.New("user rpc down")}, userID: 42, want: true, wantErr: "user rpc down", readerCalls: 1},
		{name: "empty authority response fails closed", marker: &fakeMarker{}, reader: &fakeReader{}, userID: 42, want: true, wantErr: "nil", readerCalls: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var reader PreferenceReader = tc.reader
			if tc.noReader {
				reader = nil
			}
			got, err := OptedOut(context.Background(), tc.marker, reader, tc.userID)
			require.Equal(t, tc.want, got)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			if tc.reader != nil {
				require.Equal(t, tc.readerCalls, tc.reader.calls)
			}
		})
	}
}
