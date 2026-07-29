//go:build integration

package testutil

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const rocketMQImage = "apache/rocketmq:5.1.3"

type RocketMQEnv struct {
	NameServer string
	container  testcontainers.Container
}

func SetupRocketMQEnv(t *testing.T, topics ...string) *RocketMQEnv {
	t.Helper()
	env, err := setupRocketMQEnv(topics...)
	require.NoError(t, err)
	return env
}

func setupRocketMQEnv(topics ...string) (*RocketMQEnv, error) {
	ctx := context.Background()
	provider, err := testcontainers.NewDockerProvider()
	if err != nil {
		return nil, fmt.Errorf("rocketmq docker provider: %w", err)
	}
	host, err := provider.DaemonHost(ctx)
	if err != nil {
		return nil, fmt.Errorf("rocketmq docker host: %w", err)
	}
	appendNoProxyHost(host)
	brokerPort, err := randomHighPort()
	if err != nil {
		return nil, fmt.Errorf("rocketmq broker port: %w", err)
	}
	brokerPortSpec := strconv.Itoa(brokerPort) + "/tcp"
	brokerConfig := fmt.Sprintf(`brokerClusterName = DefaultCluster
brokerName = broker-integration
brokerId = 0
brokerIP1 = %s
listenPort = %d
deleteWhen = 04
fileReservedTime = 1
brokerRole = ASYNC_MASTER
flushDiskType = ASYNC_FLUSH
namesrvAddr = 127.0.0.1:9876
autoCreateTopicEnable = true
autoCreateSubscriptionGroup = true
registerNameServerPeriod = 1000
storePathRootDir = /home/rocketmq/store
storePathCommitLog = /home/rocketmq/store/commitlog
storePathConsumeQueue = /home/rocketmq/store/consumequeue
storePathIndex = /home/rocketmq/store/index
storeCheckpoint = /home/rocketmq/store/checkpoint
abortFile = /home/rocketmq/store/abort
`, host, brokerPort)

	req := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        rocketMQImage,
			Entrypoint:   []string{"/bin/bash"},
			ExposedPorts: []string{"9876/tcp", brokerPortSpec},
			Env: map[string]string{
				"JAVA_OPT_EXT": "-Xms256m -Xmx256m -Xmn64m -XX:MaxDirectMemorySize=256m -XX:-UseContainerSupport",
			},
			Files: []testcontainers.ContainerFile{{
				Reader:            strings.NewReader(brokerConfig),
				ContainerFilePath: "/home/rocketmq/broker-integration.conf",
				FileMode:          0o644,
			}},
			Cmd: []string{"-lc", `
set -e
./mqnamesrv &
for attempt in $(seq 1 60); do
  if bash -c '</dev/tcp/127.0.0.1/9876' 2>/dev/null; then
    exec ./mqbroker -n 127.0.0.1:9876 -c /home/rocketmq/broker-integration.conf
  fi
  sleep 1
done
exit 1
`},
			HostConfigModifier: func(hostConfig *container.HostConfig) {
				hostConfig.PortBindings = network.PortMap{
					network.MustParsePort(brokerPortSpec): {{
						HostIP: netip.MustParseAddr("0.0.0.0"), HostPort: strconv.Itoa(brokerPort),
					}},
				}
			},
			WaitingFor: wait.ForLog(`The broker\[.*\] boot success`).
				AsRegexp().WithStartupTimeout(3 * time.Minute),
		},
		Started: true,
	}
	rocketContainer, err := testcontainers.GenericContainer(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("rocketmq container: %w", err)
	}
	cleanup := func(cause error) (*RocketMQEnv, error) {
		_ = testcontainers.TerminateContainer(rocketContainer)
		return nil, cause
	}

	mappedNameServerPort, err := rocketContainer.MappedPort(ctx, "9876/tcp")
	if err != nil {
		return cleanup(fmt.Errorf("rocketmq nameserver port: %w", err))
	}
	nameServer := net.JoinHostPort(host, mappedNameServerPort.Port())
	for _, topic := range topics {
		if strings.TrimSpace(topic) == "" {
			continue
		}
		if err := createRocketMQTopic(ctx, rocketContainer, brokerPort, topic); err != nil {
			return cleanup(err)
		}
	}
	return &RocketMQEnv{NameServer: nameServer, container: rocketContainer}, nil
}

func createRocketMQTopic(
	ctx context.Context,
	rocketContainer testcontainers.Container,
	brokerPort int,
	topic string,
) error {
	var lastOutput string
	created := false
	for attempt := 0; attempt < 10; attempt++ {
		exitCode, output, err := rocketContainer.Exec(ctx, []string{
			"/home/rocketmq/rocketmq-5.1.3/bin/mqadmin", "updateTopic",
			"-n", "127.0.0.1:9876", "-b", net.JoinHostPort("127.0.0.1", strconv.Itoa(brokerPort)), "-t", topic,
			"-r", "4", "-w", "4",
		})
		if err == nil {
			body, readErr := io.ReadAll(output)
			lastOutput = string(body)
			if readErr == nil && exitCode == 0 {
				created = true
				break
			}
		}
		time.Sleep(time.Second)
	}
	if !created {
		return fmt.Errorf("rocketmq create topic %q failed: %s", topic, strings.TrimSpace(lastOutput))
	}
	for attempt := 0; attempt < 30; attempt++ {
		routeCode, routeOutput, routeErr := rocketContainer.Exec(ctx, []string{
			"/home/rocketmq/rocketmq-5.1.3/bin/mqadmin", "topicRoute",
			"-n", "127.0.0.1:9876", "-t", topic,
		})
		if routeErr == nil {
			routeBody, routeReadErr := io.ReadAll(routeOutput)
			lastOutput = string(routeBody)
			if routeReadErr == nil && routeCode == 0 &&
				strings.Contains(lastOutput, "brokerDatas") && strings.Contains(lastOutput, "queueDatas") {
				return nil
			}
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("rocketmq create topic %q failed: %s", topic, strings.TrimSpace(lastOutput))
}

func randomHighPort() (int, error) {
	value, err := rand.Int(rand.Reader, big.NewInt(20_000))
	if err != nil {
		return 0, err
	}
	return 20_000 + int(value.Int64()), nil
}

func appendNoProxyHost(host string) {
	for _, name := range []string{"NO_PROXY", "no_proxy"} {
		current := os.Getenv(name)
		present := false
		for _, entry := range strings.Split(current, ",") {
			if strings.TrimSpace(entry) == host {
				present = true
				break
			}
		}
		if present {
			continue
		}
		if current == "" {
			_ = os.Setenv(name, host)
			continue
		}
		_ = os.Setenv(name, current+","+host)
	}
}

func (e *RocketMQEnv) Close() {
	if e != nil && e.container != nil {
		_ = testcontainers.TerminateContainer(e.container)
	}
}

func (e *RocketMQEnv) WaitConsumerReady(
	ctx context.Context,
	group string,
	topic string,
	timeout time.Duration,
) error {
	deadline := time.Now().Add(timeout)
	var lastOutput string
	for time.Now().Before(deadline) {
		exitCode, output, err := e.container.Exec(ctx, []string{
			"/home/rocketmq/rocketmq-5.1.3/bin/mqadmin", "consumerProgress",
			"-n", "127.0.0.1:9876", "-g", group,
		})
		if err == nil {
			body, readErr := io.ReadAll(output)
			lastOutput = string(body)
			if readErr == nil && exitCode == 0 && strings.Contains(lastOutput, topic) &&
				strings.Contains(lastOutput, "Diff Total: 0") {
				return nil
			}
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("rocketmq consumer %q did not become ready for %q: %s",
		group, topic, strings.TrimSpace(lastOutput))
}
