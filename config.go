package main

import (
	"github.com/spf13/pflag"
)

type Config struct {
	BrokerURL     string
	NumClients    int
	MessageSize   int
	PublishRate   int
	TestDuration  int
	TopicPrefix   string
	Username      string
	Password      string
	QoS           int
	ClientPrefix  string
	SubClients    int
	RetainMessage bool
}

func LoadConfig() *Config {
	cfg := &Config{}

	pflag.StringVar(&cfg.BrokerURL, "broker", "tcp://localhost:1883", "MQTT broker URL")
	pflag.IntVar(&cfg.NumClients, "clients", 100, "Number of concurrent clients")
	pflag.IntVar(&cfg.MessageSize, "size", 100, "Size of each message in bytes")
	pflag.IntVar(&cfg.PublishRate, "rate", 1000, "Messages per second per client")
	pflag.IntVar(&cfg.TestDuration, "duration", 60, "Test duration in seconds")
	pflag.StringVar(&cfg.TopicPrefix, "topic-prefix", "loadtest/", "Prefix for MQTT topics")
	pflag.StringVar(&cfg.Username, "username", "auth", "MQTT username")
	pflag.StringVar(&cfg.Password, "password", "auth", "MQTT password")
	pflag.IntVar(&cfg.QoS, "qos", 0, "MQTT QoS level (0, 1, or 2)")
	pflag.StringVar(&cfg.ClientPrefix, "prefix", "loadtest-client-", "Client ID prefix")
	pflag.IntVar(&cfg.SubClients, "sub-clients", 50, "Number of subscribing clients")
	pflag.BoolVar(&cfg.RetainMessage, "retain", false, "Retain published messages")

	pflag.Parse()

	return cfg
}
