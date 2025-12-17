package mqtt_agent

import "testing"

func withTopicPrefix(prefix string, fn func()) {
	oldConf := gConf
	gConf = Config{TopicPrefix: prefix}
	fn()
	gConf = oldConf
}

func TestFirstN(t *testing.T) {
	if got := FirstN("hello", 10); got != "hello" {
		t.Fatalf("expected full string when length within limit, got %q", got)
	}

	if got := FirstN("0123456789abcdef", 8); got != "01234567" {
		t.Fatalf("expected truncated string, got %q", got)
	}
}

func TestTopicBuildersAndExtractors(t *testing.T) {
	withTopicPrefix("mqtt2ping/", func() {
		if got := topicSubStatus(); got != "mqtt2ping/status" {
			t.Fatalf("topicSubStatus() = %q", got)
		}

		if got := topicSubDestinationStatus(); got != "mqtt2ping/status/#" {
			t.Fatalf("topicSubDestinationStatus() = %q", got)
		}

		if got := topicSubDestinationConfig(); got != "mqtt2ping/destination/#" {
			t.Fatalf("topicSubDestinationConfig() = %q", got)
		}

		if dest, ok := GetTopicSubDestinationStatus("mqtt2ping/status/router"); !ok || dest != "router" {
			t.Fatalf("unexpected destination status parse: dest=%q ok=%t", dest, ok)
		}

		if dest, ok := GetTopicSubDestinationStatus("mqtt2ping/status"); !ok || dest != "" {
			t.Fatalf("expected broadcast status topic to be recognized, dest=%q ok=%t", dest, ok)
		}

		if _, ok := GetTopicSubDestinationStatus("mqtt2ping/state/router"); ok {
			t.Fatal("expected GetTopicSubDestinationStatus to reject state topic")
		}

		if dest, ok := GetTopicSubDestinationConfig("mqtt2ping/destination/router"); !ok || dest != "router" {
			t.Fatalf("unexpected destination config parse: dest=%q ok=%t", dest, ok)
		}

		if _, ok := GetTopicSubDestinationConfig("mqtt2ping/destination"); ok {
			t.Fatal("expected GetTopicSubDestinationConfig to reject incomplete topic")
		}

		if topic := GetTopicSubStatus("mqtt2ping/status/router"); topic != "mqtt2ping/status/router" {
			t.Fatalf("GetTopicSubStatus returned %q", topic)
		}

		if topic := GetTopicSubConfig("mqtt2ping/destination/router"); topic != "mqtt2ping/destination/router" {
			t.Fatalf("GetTopicSubConfig returned %q", topic)
		}
	})
}

func TestAdvertiseHelpers(t *testing.T) {
	withTopicPrefix("mqtt2ping/", func() {
		topic, payload := MsgPubAdvState("sensor1", true)
		if topic != "mqtt2ping/state/sensor1" || payload != "online" {
			t.Fatalf("unexpected adv state: %q %q", topic, payload)
		}

		topic, payload = MsgPubAdvState("sensor1", false)
		if topic != "mqtt2ping/state/sensor1" || payload != "offline" {
			t.Fatalf("unexpected adv state offline: %q %q", topic, payload)
		}

		topic, payload = MsgPubAdvInfo("sensor1", "pong")
		if topic != "mqtt2ping/info/sensor1" || payload != "pong" {
			t.Fatalf("unexpected adv info: %q %q", topic, payload)
		}
	})
}
