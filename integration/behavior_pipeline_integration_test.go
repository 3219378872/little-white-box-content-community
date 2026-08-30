//go:build integration

package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"esx/app/behavior/rpc/xiaobaihe/behavior/pb"
	"esx/pkg/event"
	"esx/pkg/interceptor"
	"esx/pkg/mqx"
	"esx/pkg/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

const (
	behaviorUserID  = int64(424_200)
	featureVersion  = "v2"
	recallKeyPrefix = "recommend"
	internalSecret  = "behavior-pipeline-integration-secret"
)

type recentBehavior struct {
	EventID       int64  `json:"event_id"`
	ClientEventID string `json:"client_event_id"`
	RequestID     string `json:"request_id"`
	Action        string `json:"action"`
	TargetID      int64  `json:"target_id"`
}

func TestBehaviorRPCFanoutPersistsCorrelatedRawEventAndFeaturesExactlyOnce(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	t.Cleanup(cancel)

	rocketEnv := testutil.SetupRocketMQEnv(t,
		mqx.TopicUserBehaviorV2,
		mqx.TopicPostCreate,
		mqx.TopicPostUpdate,
		mqx.TopicPostDelete,
	)
	t.Cleanup(rocketEnv.Close)
	clickHouseEnv := testutil.SetupClickHouseEnv(t, testutil.ClickHouseSchemaPath("xbh_analytics.sql"))
	t.Cleanup(clickHouseEnv.Close)
	redisEnv := testutil.SetupRedisEnv(t)
	t.Cleanup(redisEnv.Close)

	repoRoot := repositoryRoot(t)
	binDir := t.TempDir()
	behaviorBin := buildBinary(t, repoRoot, binDir, "behavior-rpc", "./app/behavior/rpc")
	behaviorLogBin := buildBinary(t, repoRoot, binDir, "behavior-log", "./app/pipeline/behaviorlog")
	recommendBin := buildBinary(t, repoRoot, binDir, "recommend-feature", "./app/recommend/mq")

	runID := strconv.FormatInt(time.Now().UnixNano(), 36)
	behaviorLogGroup := "behavior-log-integration-" + runID
	recommendGroup := "recommend-feature-integration-" + runID
	configDir := t.TempDir()
	behaviorLogConfig := writeConfig(t, configDir, "behavior-log.yaml", fmt.Sprintf(`
Name: behavior-log-consumer-integration
MQ:
  NameServer: %s
  GroupName: %s
  Topic: %s
  Tag: ""
  ConsumeOrder: false
  MaxReconsumeTimes: 2
ClickHouseDSN: %s
Redis:
  Host: %s
  Type: node
DedupTTL: 3600
`, yamlString(rocketEnv.NameServer), yamlString(behaviorLogGroup),
		yamlString(mqx.TopicUserBehaviorV2), yamlString(clickHouseEnv.DSN), yamlString(redisEnv.Addr)))
	recommendConfig := writeConfig(t, configDir, "recommend-feature.yaml", fmt.Sprintf(`
Name: recommend-feature-consumer-integration
MQ:
  NameServer: %s
  GroupName: %s
  Topic: %s
  Tag: ""
  ConsumeOrder: false
  MaxReconsumeTimes: 2
Redis:
  Host: %s
  Type: node
FeatureVersion: %s
FeatureTTL: 3600
RecallKeyPrefix: %s
CandidateTTL: 3600
DeadLetterTTL: 3600
DeadLetterMaxLength: 100
`, yamlString(rocketEnv.NameServer), yamlString(recommendGroup),
		yamlString(mqx.TopicUserBehaviorV2), yamlString(redisEnv.Addr),
		yamlString(featureVersion), yamlString(recallKeyPrefix)))

	behaviorLogProcess := startProcess(t, ctx, repoRoot, behaviorLogBin, "-f", behaviorLogConfig)
	behaviorLogProcess.waitForLog(t, "Behavior-log consumer started", 45*time.Second)
	recommendProcess := startProcess(t, ctx, repoRoot, recommendBin, "-f", recommendConfig)
	recommendProcess.waitForLog(t, "Recommend MQ consumer started", 45*time.Second)
	require.NoError(t, rocketEnv.WaitConsumerReady(ctx,
		behaviorLogGroup, mqx.TopicUserBehaviorV2, 60*time.Second))
	require.NoError(t, rocketEnv.WaitConsumerReady(ctx,
		recommendGroup, mqx.TopicUserBehaviorV2, 60*time.Second))

	behaviorPort := availablePort(t)
	behaviorAddress := net.JoinHostPort("127.0.0.1", strconv.Itoa(behaviorPort))
	behaviorConfig := writeConfig(t, configDir, "behavior.yaml", fmt.Sprintf(`
Name: behavior.rpc.integration
ListenOn: %s
Mode: test
Timeout: 5000
InternalSecret: %s
MQ:
  NameServer: %s
  GroupName: %s
  Retry: 1
  SendTimeout: 5000
MaxBatchSize: 100
MaxPastAgeHours: 720
MaxFutureSkewSeconds: 300
`, yamlString(behaviorAddress), yamlString(internalSecret), yamlString(rocketEnv.NameServer),
		yamlString("behavior-producer-integration-"+runID)))
	behaviorProcess := startProcess(t, ctx, repoRoot, behaviorBin, "-f", behaviorConfig)
	behaviorProcess.waitForLog(t, "Starting rpc server", 45*time.Second)

	conn := waitForHealthyGRPC(t, ctx, behaviorAddress, 45*time.Second)
	t.Cleanup(func() { _ = conn.Close() })
	client := pb.NewBehaviorServiceClient(conn)

	clientEventID := "client-e2e-" + runID
	requestID := "request-e2e-" + runID
	position := int32(3)
	request := &pb.RecordEventsReq{
		UserId: behaviorUserID, SessionId: "session-e2e-" + runID,
		TraceId: "trace-e2e-" + runID, ClientVersion: "integration-test",
		Events: []*pb.ClientBehaviorEvent{{
			ClientEventId: clientEventID, OccurredAt: time.Now().UnixMilli(),
			Action: event.BehaviorActionExposure, TargetId: 987_654, TargetType: "post",
			Scene: "home", RequestId: requestID, Position: &position,
			RecallSource: "itemcf", ModelVersion: "rank-integration",
		}},
	}

	first, err := client.RecordEvents(ctx, request)
	require.NoError(t, err)
	require.Len(t, first.Results, 1)
	require.True(t, first.Results[0].Accepted)
	second, err := client.RecordEvents(ctx, request)
	require.NoError(t, err)
	require.Len(t, second.Results, 1)
	require.True(t, second.Results[0].Accepted)
	assert.Equal(t, first.Results[0].EventId, second.Results[0].EventId)
	expectedEventID := event.DeterministicBehaviorEventID(clientEventID)
	assert.Equal(t, expectedEventID, first.Results[0].EventId)

	waitForClickHouseEvent(t, clickHouseEnv.DB, expectedEventID, clientEventID, requestID)
	waitForRedisFeature(t, redisEnv.Redis, expectedEventID, clientEventID, requestID)
	waitForRedisKey(t, redisEnv.Redis, "dedup:behavior:v2:"+strconv.FormatInt(expectedEventID, 10))
	waitForRedisKey(t, redisEnv.Redis, "feature:v2:dedup:"+strconv.FormatInt(expectedEventID, 10))

	// Both consumer groups have acknowledged the event by this point. Give a
	// concurrently delivered duplicate a bounded window to expose bad idempotency.
	time.Sleep(2 * time.Second)
	assertExactlyOneRawEvent(t, clickHouseEnv.DB, expectedEventID)
	assertExactlyOneFeature(t, redisEnv.Redis, expectedEventID)

	invalidClientEventID := "invalid-client-e2e-" + runID
	invalid := &pb.RecordEventsReq{
		UserId: behaviorUserID,
		Events: []*pb.ClientBehaviorEvent{{
			ClientEventId: invalidClientEventID, OccurredAt: time.Now().UnixMilli(),
			Action: event.BehaviorActionExposure, TargetId: 987_655, TargetType: "post",
			Scene: "home", Position: &position,
		}},
	}
	invalidResponse, err := client.RecordEvents(ctx, invalid)
	require.NoError(t, err)
	require.Len(t, invalidResponse.Results, 1)
	assert.False(t, invalidResponse.Results[0].Accepted)
	assert.Contains(t, invalidResponse.Results[0].Reason, "request_id")
	assertNoDownstreamEvent(t, clickHouseEnv.DB, redisEnv.Redis, invalidClientEventID)

	behaviorProcess.terminateGracefully(t, 10*time.Second)
}

func waitForClickHouseEvent(t *testing.T, db *sql.DB, eventID int64, clientEventID, requestID string) {
	t.Helper()
	eventually(t, 90*time.Second, func() (bool, error) {
		var count uint64
		var storedClientEventID, storedRequestID string
		err := db.QueryRowContext(context.Background(), `SELECT count(), any(client_event_id), any(request_id)
			FROM xbh_analytics.behavior_events FINAL WHERE event_id = ?`, eventID).
			Scan(&count, &storedClientEventID, &storedRequestID)
		return count == 1 && storedClientEventID == clientEventID && storedRequestID == requestID, err
	})
}

func waitForRedisFeature(t *testing.T, rds *redis.Redis, eventID int64, clientEventID, requestID string) {
	t.Helper()
	eventually(t, 90*time.Second, func() (bool, error) {
		values, err := rds.LrangeCtx(context.Background(), "feature:v2:u:424200:recent", 0, -1)
		if err != nil || len(values) != 1 {
			return false, err
		}
		var recent recentBehavior
		if err := json.Unmarshal([]byte(values[0]), &recent); err != nil {
			return false, err
		}
		return recent.EventID == eventID && recent.ClientEventID == clientEventID &&
			recent.RequestID == requestID && recent.Action == event.BehaviorActionExposure &&
			recent.TargetID == 987_654, nil
	})
}

func waitForRedisKey(t *testing.T, rds *redis.Redis, key string) {
	t.Helper()
	eventually(t, 90*time.Second, func() (bool, error) {
		return rds.ExistsCtx(context.Background(), key)
	})
}

func assertExactlyOneRawEvent(t *testing.T, db *sql.DB, eventID int64) {
	t.Helper()
	var count uint64
	require.NoError(t, db.QueryRowContext(context.Background(),
		"SELECT count() FROM xbh_analytics.behavior_events FINAL WHERE event_id = ?", eventID).Scan(&count))
	assert.Equal(t, uint64(1), count)
}

func assertExactlyOneFeature(t *testing.T, rds *redis.Redis, eventID int64) {
	t.Helper()
	values, err := rds.LrangeCtx(context.Background(), "feature:v2:u:424200:recent", 0, -1)
	require.NoError(t, err)
	require.Len(t, values, 1)
	var recent recentBehavior
	require.NoError(t, json.Unmarshal([]byte(values[0]), &recent))
	assert.Equal(t, eventID, recent.EventID)
	score, err := rds.ZscoreByFloatCtx(context.Background(), "recommend:v2:recall:post:hot:home", "987654")
	require.NoError(t, err)
	assert.Equal(t, float64(1), score)
}

func assertNoDownstreamEvent(t *testing.T, db *sql.DB, rds *redis.Redis, clientEventID string) {
	t.Helper()
	time.Sleep(500 * time.Millisecond)
	var count uint64
	require.NoError(t, db.QueryRowContext(context.Background(),
		"SELECT count() FROM xbh_analytics.behavior_events FINAL WHERE client_event_id = ?", clientEventID).
		Scan(&count))
	assert.Zero(t, count)
	values, err := rds.LrangeCtx(context.Background(), "feature:v2:u:424200:recent", 0, -1)
	require.NoError(t, err)
	for _, value := range values {
		assert.NotContains(t, value, clientEventID)
	}
}

func eventually(t *testing.T, timeout time.Duration, check func() (bool, error)) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		ok, err := check()
		if err == nil && ok {
			return
		}
		lastErr = err
		time.Sleep(200 * time.Millisecond)
	}
	require.NoError(t, lastErr)
	t.Fatalf("condition was not met within %s", timeout)
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(filename), ".."))
}

func buildBinary(t *testing.T, root, outputDir, name, packagePath string) string {
	t.Helper()
	output := filepath.Join(outputDir, name)
	cmd := exec.Command("go", "build", "-o", output, packagePath)
	cmd.Dir = root
	combined, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "build %s: %s", packagePath, combined)
	return output
}

func writeConfig(t *testing.T, directory, name, contents string) string {
	t.Helper()
	path := filepath.Join(directory, name)
	require.NoError(t, os.WriteFile(path, []byte(strings.TrimSpace(contents)+"\n"), 0o600))
	return path
}

func yamlString(value string) string {
	return strconv.Quote(value)
}

func availablePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func waitForHealthyGRPC(t *testing.T, parent context.Context, address string, timeout time.Duration) *grpc.ClientConn {
	t.Helper()
	conn, err := grpc.NewClient(address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(interceptor.InternalAuthUnaryClientInterceptor(internalSecret)),
	)
	require.NoError(t, err)
	deadline := time.Now().Add(timeout)
	client := healthpb.NewHealthClient(conn)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(parent, time.Second)
		response, checkErr := client.Check(ctx, &healthpb.HealthCheckRequest{})
		cancel()
		if checkErr == nil && response.Status == healthpb.HealthCheckResponse_SERVING {
			return conn
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = conn.Close()
	t.Fatalf("gRPC server %s did not become healthy within %s", address, timeout)
	return nil
}

type managedProcess struct {
	cmd      *exec.Cmd
	logFile  *os.File
	done     chan struct{}
	errMu    sync.Mutex
	exitErr  error
	closeOne sync.Once
}

func startProcess(t *testing.T, ctx context.Context, directory, binary string, args ...string) *managedProcess {
	t.Helper()
	logFile, err := os.CreateTemp(t.TempDir(), filepath.Base(binary)+"-*.log")
	require.NoError(t, err)
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = directory
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, cmd.Start())
	process := &managedProcess{cmd: cmd, logFile: logFile, done: make(chan struct{})}
	go func() {
		err := cmd.Wait()
		process.errMu.Lock()
		process.exitErr = err
		process.errMu.Unlock()
		close(process.done)
	}()
	t.Cleanup(func() {
		if t.Failed() {
			contents, readErr := os.ReadFile(process.logFile.Name())
			if readErr == nil {
				t.Logf("%s output:\n%s", filepath.Base(binary), contents)
			}
		}
		process.close()
	})
	return process
}

func (p *managedProcess) waitForLog(t *testing.T, text string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		contents, err := os.ReadFile(p.logFile.Name())
		require.NoError(t, err)
		if strings.Contains(string(contents), text) {
			return
		}
		select {
		case <-p.done:
			p.errMu.Lock()
			exitErr := p.exitErr
			p.errMu.Unlock()
			t.Fatalf("process exited before logging %q: %v\n%s", text, exitErr, contents)
		default:
		}
		time.Sleep(100 * time.Millisecond)
	}
	contents, _ := os.ReadFile(p.logFile.Name())
	t.Fatalf("process did not log %q within %s\n%s", text, timeout, contents)
}

func (p *managedProcess) close() {
	p.closeOne.Do(func() {
		select {
		case <-p.done:
		default:
			_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGTERM)
			select {
			case <-p.done:
			case <-time.After(5 * time.Second):
				_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL)
				<-p.done
			}
		}
		_ = p.logFile.Close()
	})
}

func (p *managedProcess) terminateGracefully(t *testing.T, timeout time.Duration) {
	t.Helper()
	select {
	case <-p.done:
		t.Fatal("process exited before graceful termination was requested")
	default:
	}
	require.NoError(t, syscall.Kill(-p.cmd.Process.Pid, syscall.SIGTERM))
	select {
	case <-p.done:
		p.errMu.Lock()
		exitErr := p.exitErr
		p.errMu.Unlock()
		if exitErr != nil {
			contents, _ := os.ReadFile(p.logFile.Name())
			t.Fatalf("process did not exit cleanly after SIGTERM: %v\n%s", exitErr, contents)
		}
	case <-time.After(timeout):
		contents, _ := os.ReadFile(p.logFile.Name())
		t.Fatalf("process did not exit within %s after SIGTERM\n%s", timeout, contents)
	}
}
