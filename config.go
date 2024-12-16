package main

import (
	"github.com/spf13/pflag"
)

type TestMode string

const (
	ModePairwise   TestMode = "pairwise" // 1:1 pub/sub pairs (default)
	ModeNToOne     TestMode = "ntone"    // N publishers to 1 subscriber
	ModeOneToN     TestMode = "oton"     // 1 publisher to N subscribers
	ModeManyToMany TestMode = "mtom"     // M publishers to N subscribers
)

type Config struct {
	BrokerURL      string
	Username       string
	Password       string
	TopicPrefix    string
	ClientIDPrefix string
	NumClients     int
	SubClients     int
	PublishRate    int
	MessageSize    int
	TestDuration   int
	QoS            int
	RetainMessage  bool
	TestMode       TestMode

	// MQTT Client Options
	CleanSession        bool
	AutoReconnect       bool
	ResumeSubs          bool
	OrderMatters        bool
	MessageChannelDepth uint
	InsecureSkipVerify  bool
}

func LoadConfig() *Config {
	brokerURL := pflag.String("broker", "tcp://localhost:1883", "MQTT broker URL")
	username := pflag.String("username", "", "MQTT username")
	password := pflag.String("password", "", "MQTT password")
	topicPrefix := pflag.String("topic", "loadtest/", "Topic prefix")
	clientIDPrefix := pflag.String("client-prefix", "loadtest", "Client ID prefix")
	numClients := pflag.Int("clients", 1, "Number of publisher clients")
	subClients := pflag.Int("sub-clients", 1, "Number of subscriber clients")
	publishRate := pflag.Int("rate", 10, "Messages per second per publisher")
	messageSize := pflag.Int("size", 100, "Message size in bytes")
	testDuration := pflag.Int("duration", 30, "Test duration in seconds")
	qos := pflag.Int("qos", 0, "MQTT QoS level (0, 1, or 2)")
	retain := pflag.Bool("retain", false, "Retain messages")
	mode := pflag.String("mode", "pairwise", "Test mode (pairwise, ntone, oton, mtom)")

	// MQTT Client Options
	cleanSession := pflag.Bool("clean-session", true, "Clean session on connect")
	autoReconnect := pflag.Bool("auto-reconnect", true, "Auto reconnect on connection loss")
	resumeSubs := pflag.Bool("resume-subs", true, "Resume subscriptions after reconnect")
	orderMatters := pflag.Bool("order-matters", false, "Maintain message order")
	msgChannelDepth := pflag.Uint("msg-channel-depth", 1000, "Message channel buffer size")
	insecureSkipVerify := pflag.Bool("insecure-skip-verify", true, "Skip TLS certificate verification")

	pflag.Parse()

	return &Config{
		BrokerURL:      *brokerURL,
		Username:       *username,
		Password:       *password,
		TopicPrefix:    *topicPrefix,
		ClientIDPrefix: *clientIDPrefix,
		NumClients:     *numClients,
		SubClients:     *subClients,
		PublishRate:    *publishRate,
		MessageSize:    *messageSize,
		TestDuration:   *testDuration,
		QoS:            *qos,
		RetainMessage:  *retain,
		TestMode:       TestMode(*mode),

		// MQTT Client Options
		CleanSession:        *cleanSession,
		AutoReconnect:       *autoReconnect,
		ResumeSubs:          *resumeSubs,
		OrderMatters:        *orderMatters,
		MessageChannelDepth: *msgChannelDepth,
		InsecureSkipVerify:  *insecureSkipVerify,
	}
}
