package deploy

import (
	"os"
	"strings"
	"testing"
)

func TestPrometheusRuntimeMonitoringContract(t *testing.T) {
	prometheusConfig := mustReadMonitoringFile(t, "prometheus/prometheus.yml")
	for _, fragment := range []string{
		"/etc/prometheus/alerts.yml",
		"host.docker.internal:9190",
		"host.docker.internal:9188",
		"host.docker.internal:9103",
		"host.docker.internal:9121",
		"host.docker.internal:9122",
		"host.docker.internal:9123",
		"host.docker.internal:9124",
		"host.docker.internal:9131",
		"host.docker.internal:9132",
		"host.docker.internal:9133",
		"host.docker.internal:9134",
		"host.docker.internal:9135",
		"host.docker.internal:9136",
	} {
		if !strings.Contains(prometheusConfig, fragment) {
			t.Errorf("prometheus config is missing %q", fragment)
		}
	}

	alerts := mustReadMonitoringFile(t, "prometheus/alerts.yml")
	for _, metric := range []string{
		"esx_recommend_rpc_pipeline_total",
		"esx_recommend_rpc_inference_total",
		"esx_recommend_rpc_results_total",
		"esx_assistant_agent_llm_calls_total",
		"esx_assistant_agent_tool_calls_total",
		"esx_assistant_agent_first_token_seconds_bucket",
		"esx_assistant_agent_queue_age_seconds_bucket",
		"esx_assistant_agent_lease_recover_total",
		"esx_outbox_backlog_count",
		"esx_outbox_oldest_age_seconds",
		"esx_outbox_backlog_collections_total",
		"esx_behavior_rpc_mq_publish_total",
		"esx_behavior_log_consumer_messages_total",
		"esx_behavior_log_store_write_seconds_bucket",
		"esx_recommend_feature_consumer_event_lag_seconds_bucket",
		"esx_search_index_consumer_event_lag_seconds_bucket",
		"esx_embedding_index_consumer_event_lag_seconds_bucket",
		"esx_assistant_watch_matcher_event_lag_seconds_bucket",
	} {
		if !strings.Contains(alerts, metric) {
			t.Errorf("prometheus alerts are missing metric %q", metric)
		}
	}

	productionConfig := mustReadMonitoringFile(t, "prometheus/prometheus.production.yml")
	for _, fragment := range []string{
		"behavior-log-consumer:9131",
		"recommend-consumer:9132",
		"search-consumer:9133",
		"embedding-consumer:9134",
		"assistant-watch-consumer:9135",
		"assistant-agent:9136",
	} {
		if !strings.Contains(productionConfig, fragment) {
			t.Errorf("production prometheus config is missing %q", fragment)
		}
	}

	compose := mustReadMonitoringFile(t, "docker-compose.middleware.yml")
	for _, fragment := range []string{
		"./prometheus/alerts.yml:/etc/prometheus/alerts.yml",
		"host.docker.internal:host-gateway",
	} {
		if !strings.Contains(compose, fragment) {
			t.Errorf("prometheus compose wiring is missing %q", fragment)
		}
	}
}

func TestRuntimeServicesExposeFrameworkHealthAndMetrics(t *testing.T) {
	services := map[string][]string{
		"../app/behavior/rpc/etc/behavior.yaml":       {"Health: true", "Port: 9121", "EnableMetrics: true"},
		"../app/search/rpc/etc/search.yaml":           {"Health: true", "Port: 9122", "EnableMetrics: true"},
		"../app/recommend/rpc/etc/recommend.yaml":     {"Health: true", "Port: 9123", "EnableMetrics: true"},
		"../app/assistant/rpc/etc/assistant.yaml":     {"Health: true", "Port: 9124", "EnableMetrics: true"},
		"../app/content/rpc/etc/content.yaml":         {"Port: 9188", "EnableMetrics: true"},
		"../app/interaction/rpc/etc/interaction.yaml": {"Port: 9103", "EnableMetrics: true"},
		"../app/user/rpc/etc/user.yaml":               {"Port: 9190", "EnableMetrics: true"},
	}
	for path, fragments := range services {
		config := mustReadMonitoringFile(t, path)
		for _, fragment := range fragments {
			if !strings.Contains(config, fragment) {
				t.Errorf("%s is missing %q", path, fragment)
			}
		}
	}

	for _, path := range []string{"../app/behavior/rpc/behavior.go", "../app/search/rpc/search.go"} {
		mainSource := mustReadMonitoringFile(t, path)
		if strings.Contains(mainSource, "RegisterHealthServer") {
			t.Errorf("%s manually registers health in addition to zrpc Health", path)
		}
	}
}

func TestMQConsumersExposePrometheusMetrics(t *testing.T) {
	consumers := map[string][]string{
		"../app/pipeline/behaviorlog/etc/behavior-log.yaml": {"Name: behavior-log-consumer", "Port: 9131"},
		"../app/recommend/mq/etc/recommend-consumer.yaml":   {"Name: recommend-feature-consumer", "Port: 9132"},
		"../app/search/mq/etc/search-consumer.yaml":         {"Name: search-index-consumer", "Port: 9133"},
		"../app/embedding/mq/etc/embedding-consumer.yaml":   {"Name: embedding-index-consumer", "Port: 9134"},
		"../app/assistant/mq/etc/watch-consumer.yaml":       {"Name: assistant-watch-matcher", "Port: 9135"},
		"../app/assistant/worker/etc/agent.yaml":            {"Name: assistant-agent", "Port: 9136"},
	}
	for path, fragments := range consumers {
		config := mustReadMonitoringFile(t, path)
		for _, fragment := range fragments {
			if !strings.Contains(config, fragment) {
				t.Errorf("%s is missing %q", path, fragment)
			}
		}
	}
}

func mustReadMonitoringFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
