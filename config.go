package main

import (
	"github.com/spf13/pflag"
)

type TestMode string

const (
	ModePairwise  TestMode = "pairwise"  // 1:1 pub/sub pairs (default)
	ModeNToOne    TestMode = "ntone"     // N publishers to 1 subscriber
	ModeOneToN    TestMode = "oton"      // 1 publisher to N subscribers
	ModeManyToMany TestMode = "mtom"     // M publishers to N subscribers
)

type Config struct {
	BrokerURL     string
	Username      string
	Password      string
	TopicPrefix   string
	NumClients    int
	SubClients    int
	PublishRate   int
	MessageSize   int
	TestDuration  int
	QoS           int
	RetainMessage bool
	TestMode      TestMode
}

func LoadConfig() *Config {
	brokerURL := pflag.String("broker", "tcp://localhost:1883", "MQTT broker URL")
	username := pflag.String("username", "", "MQTT username")
	password := pflag.String("password", "", "MQTT password")
	topicPrefix := pflag.String("topic", "loadtest/", "Topic prefix")
	numClients := pflag.Int("clients", 1, "Number of publisher clients")
	subClients := pflag.Int("sub-clients", 1, "Number of subscriber clients")
	publishRate := pflag.Int("rate", 1, "Messages per second per publisher")
	messageSize := pflag.Int("size", 100, "Message size in bytes")
	testDuration := pflag.Int("duration", 30, "Test duration in seconds")
	qos := pflag.Int("qos", 0, "MQTT QoS level (0, 1, or 2)")
	retain := pflag.Bool("retain", false, "Retain messages")
	mode := pflag.String("mode", "pairwise", "Test mode (pairwise, ntone, oton, mtom)")

	pflag.Parse()

	return &Config{
		BrokerURL:     *brokerURL,
		Username:      *username,
		Password:      *password,
		TopicPrefix:   *topicPrefix,
		NumClients:    *numClients,
		SubClients:    *subClients,
		PublishRate:   *publishRate,
		MessageSize:   *messageSize,
		TestDuration:  *testDuration,
		QoS:           *qos,
		RetainMessage: *retain,
		TestMode:      TestMode(*mode),
	}
}
