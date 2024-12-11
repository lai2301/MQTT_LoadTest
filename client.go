package main

import (
	"crypto/rand"
	"fmt"
	"strconv"
	"sync"
	"time"
	"crypto/tls"
	"strings"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type Client struct {
	ID           string
	client       mqtt.Client
	config       *Config
	stats        *Stats
	stopChan     chan struct{}
	isSubscriber bool
	messagesChan chan []byte
	topic        string
	clientIndex  int
	startTime    time.Time
}

type Stats struct {
	PublishedMessages uint64
	ReceivedMessages  uint64
	Errors            uint64
	LastPublished     map[int]time.Time // Track last publish time per client
	LastReceived      map[int]time.Time // Track last receive time per client
	ChannelStats      map[int]*ChannelStat
	mutex             sync.Mutex
}

type ChannelStat struct {
	Published   uint64
	Received    uint64
	MinRate     float64
	MaxRate     float64
	TotalRate   float64
	SampleCount uint64
}

type ClientOptions struct {
	MaxInflight     int
	BatchSize       int
	PublishTimeout  time.Duration
	ConnectTimeout  time.Duration
}

func DefaultClientOptions() ClientOptions {
	return ClientOptions{
		MaxInflight:     100,
		BatchSize:       10,
		PublishTimeout:  5 * time.Second,
		ConnectTimeout:  10 * time.Second,
	}
}

func NewStats() *Stats {
	return &Stats{
		LastPublished: make(map[int]time.Time),
		LastReceived:  make(map[int]time.Time),
		ChannelStats:  make(map[int]*ChannelStat),
	}
}

func (s *Stats) UpdateChannelRate(channelID int, rate float64) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	stat, exists := s.ChannelStats[channelID]
	if !exists {
		stat = &ChannelStat{
			MinRate: rate,
			MaxRate: rate,
		}
		s.ChannelStats[channelID] = stat
	}

	stat.TotalRate += rate
	stat.SampleCount++

	if rate < stat.MinRate {
		stat.MinRate = rate
	}
	if rate > stat.MaxRate {
		stat.MaxRate = rate
	}
}

func NewClient(id int, config *Config, stats *Stats, isSubscriber bool) (*Client, error) {
	// Use different prefixes for pub/sub but same ID for pairs
	clientPrefix := "loadtest-pub-"
	if isSubscriber {
		clientPrefix = "loadtest-sub-"
	}
	clientID := fmt.Sprintf("%s%d", clientPrefix, id/2)
	topic := fmt.Sprintf("%s%d", config.TopicPrefix, id/2)

	opts := mqtt.NewClientOptions().
		AddBroker(config.BrokerURL).
		SetClientID(clientID).
		SetUsername(config.Username).
		SetPassword(config.Password).
		SetCleanSession(false).
		SetAutoReconnect(true).
		SetKeepAlive(30 * time.Second).
		SetPingTimeout(10 * time.Second).
		SetMaxReconnectInterval(1 * time.Second).
		SetOrderMatters(false).
		SetConnectTimeout(10 * time.Second).
		SetOnConnectHandler(func(client mqtt.Client) {
			fmt.Printf("Client %s connected successfully\n", clientID)
		}).
		SetConnectionLostHandler(func(client mqtt.Client, err error) {
			fmt.Printf("Connection lost for client %s: %v\n", clientID, err)
			// Try to reconnect immediately
			time.Sleep(100 * time.Millisecond)
			client.Connect()
		})

	// Configure TLS if using SSL
	if strings.HasPrefix(config.BrokerURL, "ssl://") {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: true,  // Note: In production, you should properly verify certificates
		}
		opts.SetTLSConfig(tlsConfig)
	}

	if isSubscriber {
		opts.SetDefaultPublishHandler(func(client mqtt.Client, msg mqtt.Message) {
			stats.mutex.Lock()
			stats.ReceivedMessages++
			stats.LastReceived[id/2] = time.Now()
			stats.mutex.Unlock()

			// Extract timestamp from payload for latency calculation
			if len(msg.Payload()) >= 8 {
				sentTime := string(msg.Payload()[:8])
				if timestamp, err := strconv.ParseInt(sentTime, 10, 64); err == nil {
					_ = time.Since(time.Unix(0, timestamp))
				}
			}
		})
	}

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		return nil, token.Error()
	}

	return &Client{
		ID:           clientID,
		client:       client,
		config:       config,
		stats:        stats,
		stopChan:     make(chan struct{}),
		isSubscriber: isSubscriber,
		messagesChan: make(chan []byte, 100),
		topic:        topic,
		clientIndex:  id / 2,
		startTime:    time.Time{},
	}, nil
}

func (c *Client) Start() {
	if c.isSubscriber {
		go c.subscribeLoop()
	} else {
		go c.publishLoop()
	}
}

func (c *Client) SetStartTime(t time.Time) {
	c.startTime = t
}

func (c *Client) subscribeLoop() {
	// Add subscription with callback to confirm subscription
	token := c.client.Subscribe(c.topic, byte(c.config.QoS), func(client mqtt.Client, msg mqtt.Message) {
		c.stats.mutex.Lock()
		c.stats.ReceivedMessages++
		c.stats.LastReceived[c.clientIndex] = time.Now()
		c.stats.mutex.Unlock()

		// Extract timestamp from payload for latency calculation
		if len(msg.Payload()) >= 8 {
			sentTime := string(msg.Payload()[:8])
			if timestamp, err := strconv.ParseInt(sentTime, 10, 64); err == nil {
				_ = time.Since(time.Unix(0, timestamp))
			}
		}
	})

	if token.Wait() && token.Error() != nil {
		c.stats.mutex.Lock()
		c.stats.Errors++
		c.stats.mutex.Unlock()
		fmt.Printf("Error subscribing to topic %s: %v\n", c.topic, token.Error())
		return
	}

	<-c.stopChan

	// Clean unsubscribe
	if token := c.client.Unsubscribe(c.topic); token.Wait() && token.Error() != nil {
		fmt.Printf("Error unsubscribing from topic %s: %v\n", c.topic, token.Error())
	}
}

func (c *Client) publishLoop() {
	if !c.startTime.IsZero() {
		time.Sleep(time.Until(c.startTime))
	}

	// Use smaller intervals for more frequent publishing
	publishInterval := time.Second / time.Duration(c.config.PublishRate)
	ticker := time.NewTicker(publishInterval)
	defer ticker.Stop()

	payload := make([]byte, c.config.MessageSize)
	lastCount := uint64(0)
	lastTime := time.Now()

	// Pre-generate multiple random payloads
	payloadPool := make([][]byte, 10)
	for i := range payloadPool {
		payloadPool[i] = make([]byte, c.config.MessageSize)
		_, _ = rand.Read(payloadPool[i][8:]) // Leave space for timestamp
	}

	// Create a buffered channel for publish tokens
	tokenChan := make(chan mqtt.Token, 100)
	
	// Start a goroutine to handle publish completions
	go func() {
		for token := range tokenChan {
			if token.Wait() && token.Error() != nil {
				c.stats.mutex.Lock()
				c.stats.Errors++
				c.stats.mutex.Unlock()
				fmt.Printf("Error publishing to topic %s: %v\n", c.topic, token.Error())
				continue
			}

			c.stats.mutex.Lock()
			c.stats.PublishedMessages++
			c.stats.LastPublished[c.clientIndex] = time.Now()
			c.stats.mutex.Unlock()
		}
	}()

	payloadIndex := 0
	for {
		select {
		case <-c.stopChan:
			close(tokenChan)
			return
		case <-ticker.C:
			// Update stats every second
			if time.Since(lastTime) >= time.Second {
				c.stats.mutex.Lock()
				currentCount := c.stats.PublishedMessages
					rate := float64(currentCount-lastCount) / time.Since(lastTime).Seconds()
					c.stats.UpdateChannelRate(c.clientIndex, rate)
					lastCount = currentCount
					c.stats.mutex.Unlock()
					lastTime = time.Now()
			}

			// Use pre-generated payload
			copy(payload, payloadPool[payloadIndex])
			payloadIndex = (payloadIndex + 1) % len(payloadPool)

			// Update timestamp
			timestamp := time.Now().UnixNano()
			copy(payload[0:8], []byte(fmt.Sprintf("%d", timestamp)))

			// Publish without blocking
			token := c.client.Publish(c.topic, byte(c.config.QoS), c.config.RetainMessage, payload)
			select {
			case tokenChan <- token:
			default:
				// If channel is full, handle completion here
				if token.Wait() && token.Error() != nil {
					c.stats.mutex.Lock()
					c.stats.Errors++
					c.stats.mutex.Unlock()
				}
			}
		}
	}
}

func (c *Client) Stop() {
	close(c.stopChan)
	if c.isSubscriber {
		c.client.Unsubscribe(c.topic)
	}
	c.client.Disconnect(250)
}
