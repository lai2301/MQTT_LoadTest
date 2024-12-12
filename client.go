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
	TestDuration int
}

type Stats struct {
	PublishedMessages uint64
	ReceivedMessages  uint64
	Errors            uint64
	RetryAttempts     uint64    // Track total retry attempts
	RetrySuccesses    uint64    // Track successful retries
	TimeoutErrors     uint64    // Track timeout errors
	ConnectionErrors  uint64    // Track connection-related errors
	DroppedMessages  uint64    // Track messages dropped by subscriber
	DuplicateMessages uint64   // Track duplicate messages
	OutOfOrderMessages uint64  // Track out-of-order messages
	ProcessingErrors uint64    // Track message processing errors
	LastPublished     map[int]time.Time
	LastReceived      map[int]time.Time
	ChannelStats      map[int]*ChannelStat
	LastMessageID    map[int]uint64  // Track last message ID per channel
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
	MaxRetries      int
	RetryDelay     time.Duration
}

func DefaultClientOptions() ClientOptions {
	return ClientOptions{
		MaxInflight:     100,
		BatchSize:       10,
		PublishTimeout:  5 * time.Second,
		ConnectTimeout:  10 * time.Second,
		MaxRetries:      3,
		RetryDelay:     100 * time.Millisecond,
	}
}

func NewStats() *Stats {
	return &Stats{
		LastPublished: make(map[int]time.Time),
		LastReceived:  make(map[int]time.Time),
		ChannelStats:  make(map[int]*ChannelStat),
		LastMessageID: make(map[int]uint64),
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
	clientID := generateClientID(id, config.TestMode, isSubscriber)
	topic := generateTopic(id, config.TestMode, config.TopicPrefix, isSubscriber)

	opts := mqtt.NewClientOptions().
		AddBroker(config.BrokerURL).
		SetClientID(clientID).
		SetUsername(config.Username).
		SetPassword(config.Password).
		SetCleanSession(false).
		SetAutoReconnect(true).
		SetKeepAlive(60 * time.Second).
		SetPingTimeout(20 * time.Second).
		SetConnectTimeout(30 * time.Second).
		SetWriteTimeout(10 * time.Second).
		SetMaxReconnectInterval(5 * time.Second).
		SetConnectRetryInterval(2 * time.Second).
		SetResumeSubs(true).
		SetOrderMatters(false).
		SetMessageChannelDepth(1000).
		SetStore(mqtt.NewMemoryStore()).
		SetOnConnectHandler(func(client mqtt.Client) {
			fmt.Printf("Client %s connected successfully at %v\n", 
				clientID, time.Now().Format("15:04:05.000"))
		}).
		SetConnectionLostHandler(func(client mqtt.Client, err error) {
			if !strings.Contains(err.Error(), "EOF") {
				fmt.Printf("Connection lost for client %s at %v: %v\n", 
					clientID, time.Now().Format("15:04:05.000"), err)
			}
			stats.mutex.Lock()
			stats.ConnectionErrors++
			stats.mutex.Unlock()
		})

	// Configure TLS if using SSL
	if strings.HasPrefix(config.BrokerURL, "ssl://") {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: true,
			MinVersion:       tls.VersionTLS12,
			MaxVersion:       tls.VersionTLS13,
			Renegotiation:    tls.RenegotiateNever,
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
		clientIndex:  id,
		startTime:    time.Time{},
		TestDuration: config.TestDuration,
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
	// Create message processing channel with large buffer
	msgChan := make(chan mqtt.Message, 1000)
	
	// Start multiple message processors
	const numProcessors = 5
	for i := 0; i < numProcessors; i++ {
		go func() {
			for msg := range msgChan {
				c.processMessage(msg)
			}
		}()
	}

	// Subscription function
	subscribe := func() error {
		if !c.client.IsConnected() {
			return fmt.Errorf("client not connected")
		}

		token := c.client.Subscribe(c.topic, byte(c.config.QoS), func(client mqtt.Client, msg mqtt.Message) {
			select {
			case msgChan <- msg:
				// Message queued for processing
			default:
				c.stats.mutex.Lock()
				c.stats.DroppedMessages++
				c.stats.mutex.Unlock()
			}
		})

		if token.Wait() && token.Error() != nil {
			return token.Error()
		}
		return nil
	}

	// Initial subscription
	if err := subscribe(); err != nil {
		fmt.Printf("Initial subscription failed for %s: %v\n", c.ID, err)
	}

	// Create reconnection ticker
	reconnectTicker := time.NewTicker(time.Second)
	defer reconnectTicker.Stop()

	for {
		select {
		case <-c.stopChan:
			// Clean unsubscribe only if connected
			if c.client.IsConnected() {
				if token := c.client.Unsubscribe(c.topic); token.Wait() && token.Error() != nil {
					fmt.Printf("Error unsubscribing from topic %s: %v\n", c.topic, token.Error())
				}
			}
			close(msgChan)
			return

		case <-reconnectTicker.C:
			if !c.client.IsConnected() {
				fmt.Printf("Client %s disconnected, attempting to reconnect...\n", c.ID)
				if token := c.client.Connect(); token.Wait() && token.Error() != nil {
					fmt.Printf("Reconnection failed for %s: %v\n", c.ID, token.Error())
					continue
				}
				// Resubscribe after reconnection
				if err := subscribe(); err != nil {
					fmt.Printf("Resubscription failed for %s: %v\n", c.ID, err)
				} else {
					fmt.Printf("Client %s successfully reconnected and resubscribed\n", c.ID)
				}
			}
		}
	}
}

func (c *Client) processMessage(msg mqtt.Message) {
	if len(msg.Payload()) < 16 {
		c.stats.mutex.Lock()
		c.stats.ProcessingErrors++
		c.stats.mutex.Unlock()
		return
	}

	c.stats.mutex.Lock()
	defer c.stats.mutex.Unlock()

	msgID, err := strconv.ParseUint(string(msg.Payload()[8:16]), 10, 64)
	if err != nil {
		c.stats.ProcessingErrors++
		return
	}

	switch c.config.TestMode {
	case ModeNToOne:
		// In N-to-1 mode, don't check for duplicates
		c.stats.ReceivedMessages++
		c.stats.LastReceived[c.clientIndex] = time.Now()
		
	case ModeManyToMany:
		// Extract publisher ID from topic
		parts := strings.Split(msg.Topic(), "/")
		if len(parts) < 1 {
			c.stats.ProcessingErrors++
			return
		}
		pubID, err := strconv.Atoi(parts[len(parts)-1])
		if err != nil {
			c.stats.ProcessingErrors++
			return
		}

		// Track last message ID per publisher
		lastID, exists := c.stats.LastMessageID[pubID]
		if exists && msgID <= lastID {
			// Skip duplicate messages from same publisher
			return
		}
		
		c.stats.LastMessageID[pubID] = msgID
		c.stats.ReceivedMessages++
		c.stats.LastReceived[c.clientIndex] = time.Now()
		
	default:
		// For other modes, check for duplicates and order
		lastID, exists := c.stats.LastMessageID[c.clientIndex]
		if exists {
			if msgID <= lastID {
				c.stats.DuplicateMessages++
				return
			} else if msgID > lastID+1 {
				c.stats.OutOfOrderMessages++
				c.stats.DroppedMessages += msgID - lastID - 1
			}
		}
		c.stats.LastMessageID[c.clientIndex] = msgID
		c.stats.ReceivedMessages++
		c.stats.LastReceived[c.clientIndex] = time.Now()
	}
}

func (c *Client) publishWithRetry(payload []byte, maxRetries int, retryDelay time.Duration, topic string) error {
	if !c.client.IsConnected() {
		c.stats.mutex.Lock()
		c.stats.ConnectionErrors++
		c.stats.mutex.Unlock()
		return fmt.Errorf("client not connected")
	}

	var lastErr error
	for i := 0; i <= maxRetries; i++ {
		token := c.client.Publish(topic, byte(c.config.QoS), c.config.RetainMessage, payload)
		
		if token.WaitTimeout(5 * time.Second) {
			if err := token.Error(); err != nil {
				lastErr = err
				if i == maxRetries {
					fmt.Printf("Client %s - Final publish attempt failed: %v\n", c.ID, err)
				}
			} else {
				c.stats.mutex.Lock()
				c.stats.PublishedMessages++
				if i > 0 {
					c.stats.RetrySuccesses++
				}
				c.stats.LastPublished[c.clientIndex] = time.Now()
				c.stats.mutex.Unlock()
				return nil
			}
		} else {
			lastErr = fmt.Errorf("publish timeout")
			if i == maxRetries {
				fmt.Printf("Client %s - Final publish attempt timed out\n", c.ID)
			}
		}

		// Update error statistics
		if strings.Contains(lastErr.Error(), "timeout") {
			c.stats.mutex.Lock()
			c.stats.TimeoutErrors++
			c.stats.mutex.Unlock()
		} else {
			c.stats.mutex.Lock()
			c.stats.ConnectionErrors++
			c.stats.mutex.Unlock()
		}

		if i < maxRetries {
			c.stats.mutex.Lock()
			c.stats.RetryAttempts++
			c.stats.mutex.Unlock()
			time.Sleep(retryDelay)
		}
	}
	return lastErr
}

func (c *Client) publishLoop() {
	if !c.startTime.IsZero() {
		time.Sleep(time.Until(c.startTime))
	}

	// Pre-allocate payloads
	const poolSize = 1000
	payloadPool := make([][]byte, poolSize)
	for i := range payloadPool {
		payloadPool[i] = make([]byte, c.config.MessageSize)
		_, _ = rand.Read(payloadPool[i][16:]) // Leave space for timestamp and msgID
	}

	messageID := uint64(0)
	payloadIndex := 0
	startTime := time.Now()
	publishInterval := time.Second / time.Duration(c.config.PublishRate)

	// For OneToN mode, create a list of subscriber topics
	var topics []string
	if c.config.TestMode == ModeOneToN {
		for i := 0; i < c.config.SubClients; i++ {
			topics = append(topics, fmt.Sprintf("%s%d", c.config.TopicPrefix, i))
		}
	} else {
		topics = []string{c.topic}
	}

	fmt.Printf("Client %s - Starting publisher (rate: %d msg/sec, interval: %v)\n", 
		c.ID, c.config.PublishRate, publishInterval)

	// Main publishing loop
	for {
		select {
		case <-c.stopChan:
			return
		default:
		}

		if time.Since(startTime) >= time.Duration(c.config.TestDuration)*time.Second {
			return
		}

		messageID++
		payload := make([]byte, c.config.MessageSize)
		copy(payload, payloadPool[payloadIndex])
		payloadIndex = (payloadIndex + 1) % len(payloadPool)

		timestamp := time.Now().UnixNano()
		copy(payload[0:8], []byte(fmt.Sprintf("%08d", timestamp)))
		copy(payload[8:16], []byte(fmt.Sprintf("%08d", messageID)))

		// Publish to all topics for OneToN mode
		for _, topic := range topics {
			if err := c.publishWithRetry(payload, DefaultClientOptions().MaxRetries, DefaultClientOptions().RetryDelay, topic); err != nil {
				c.stats.mutex.Lock()
				c.stats.Errors++
				c.stats.mutex.Unlock()
				fmt.Printf("Client %s - Publish error to topic %s: %v\n", c.ID, topic, err)
			}
		}

		time.Sleep(publishInterval)
	}
}

func (c *Client) Stop() {
	// Signal stop to all goroutines
	close(c.stopChan)

	// Give subscribers time to process remaining messages
	time.Sleep(100 * time.Millisecond)

	if c.isSubscriber {
		// Only attempt to unsubscribe if connected
		if c.client.IsConnected() {
			token := c.client.Unsubscribe(c.topic)
			token.WaitTimeout(2 * time.Second) // Wait with timeout for unsubscribe to complete
		}
	}

	// Disconnect with a longer quiesce time
	c.client.Disconnect(1000) // 1 second to complete pending work
}

func generateTopic(id int, mode TestMode, prefix string, isSubscriber bool) string {
	switch mode {
	case ModePairwise:
		return fmt.Sprintf("%s%d", prefix, id)
	case ModeNToOne:
		return fmt.Sprintf("%sntone", prefix)
	case ModeOneToN:
		if isSubscriber {
			return fmt.Sprintf("%s%d", prefix, id)
		}
		// Publisher will use individual topics in publishLoop
		return prefix
	case ModeManyToMany:
		if isSubscriber {
			return fmt.Sprintf("%s#", prefix)
		}
		return fmt.Sprintf("%s%d", prefix, id)
	default:
		return fmt.Sprintf("%s%d", prefix, id)
	}
}

func generateClientID(id int, mode TestMode, isSubscriber bool) string {
	prefix := "loadtest-pub-"
	if isSubscriber {
		prefix = "loadtest-sub-"
	}
	return fmt.Sprintf("%s%d", prefix, id)
}
