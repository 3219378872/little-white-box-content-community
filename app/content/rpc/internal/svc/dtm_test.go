package svc

import (
	"esx/app/content/rpc/internal/config"
	"testing"

	"github.com/dtm-labs/dtm/client/dtmcli"
	"github.com/dtm-labs/dtm/client/dtmcli/dtmimp"
	"github.com/stretchr/testify/require"
)

func TestConfigureDTMBarrierTableUsesBusinessLocalTable(t *testing.T) {
	original := dtmimp.BarrierTableName
	t.Cleanup(func() {
		dtmcli.SetBarrierTableName(original)
	})
	dtmcli.SetBarrierTableName("dtm_barrier.barrier")

	configureDTMBarrierTable()

	require.Equal(t, "dtm_barrier", dtmimp.BarrierTableName)
}

func TestValidateDTMConfigRequiresReliableMessageAddresses(t *testing.T) {
	err := validateDTMConfig(config.Config{})
	require.Error(t, err)

	err = validateDTMConfig(config.Config{
		DtmServer:         "dtm:36790",
		ContentBusiServer: "content:8088",
		FeedBusiServer:    "feed:9091",
	})
	require.NoError(t, err)
}
