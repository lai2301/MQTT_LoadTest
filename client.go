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
	if err == nil {
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
	}

	c.stats.ReceivedMessages++
	c.stats.LastReceived[c.clientIndex] = time.Now()
}

func (c *Client) publishWithRetry(payload []byte, maxRetries int, retryDelay time.Duration) error {
	var lastErr error
	for i := 0; i <= maxRetries; i++ {
		token := c.client.Publish(c.topic, byte(c.config.QoS), c.config.RetainMessage, payload)
		if token.WaitTimeout(5 * time.Second) {
			if token.Error() == nil {
				c.stats.mutex.Lock()
				c.stats.PublishedMessages++
				if i > 0 {
					c.stats.RetrySuccesses++
				}
				c.stats.LastPublished[c.clientIndex] = time.Now()
				c.stats.mutex.Unlock()
				return nil
			}
			lastErr = token.Error()
			if strings.Contains(lastErr.Error(), "timeout") {
				c.stats.mutex.Lock()
				c.stats.TimeoutErrors++
				c.stats.mutex.Unlock()
			} else {
				c.stats.mutex.Lock()
				c.stats.ConnectionErrors++
				c.stats.mutex.Unlock()
			}
		} else {
			lastErr = fmt.Errorf("publish timeout")
			c.stats.mutex.Lock()
			c.stats.TimeoutErrors++
			c.stats.mutex.Unlock()
		}

		if i < maxRetries {
			c.stats.mutex.Lock()
			c.stats.RetryAttempts++
			c.stats.mutex.Unlock()
			time.Sleep(retryDelay)
			fmt.Printf("Retrying publish for client %s (attempt %d/%d)\n", c.ID, i+2, maxRetries+1)
		}
	}
	return lastErr
}

func (c *Client) publishLoop() {
	if !c.startTime.IsZero() {
		time.Sleep(time.Until(c.startTime))
	}

	publishInterval := time.Second / time.Duration(c.config.PublishRate)
	ticker := time.NewTicker(publishInterval)
	defer ticker.Stop()

	// Pre-allocate payloads
	const poolSize = 1000
	payloadPool := make([][]byte, poolSize)
	for i := range payloadPool {
		payloadPool[i] = make([]byte, c.config.MessageSize)
		_, _ = rand.Read(payloadPool[i][16:]) // Leave space for timestamp and msgID
	}

	// Larger buffer for publish channel
	publishChan := make(chan []byte, poolSize)
	
	// More concurrent publishers
	const numPublishers = 5
	var publishWg sync.WaitGroup
	
	// Start multiple publisher goroutines
	for i := 0; i < numPublishers; i++ {
		publishWg.Add(1)
		go func() {
			defer publishWg.Done()
			for payload := range publishChan {
				if err := c.publishWithRetry(payload, DefaultClientOptions().MaxRetries, DefaultClientOptions().RetryDelay); err != nil {
					c.stats.mutex.Lock()
					c.stats.Errors++
					c.stats.mutex.Unlock()
				}
			}
		}()
	}

	messageID := uint64(0)
	payloadIndex := 0
	lastCount := uint64(0)
	lastTime := time.Now()
	
	// Create batch of messages
	const batchSize = 10
	batch := make([][]byte, 0, batchSize)

	for {
		select {
		case <-c.stopChan:
			close(publishChan)
			publishWg.Wait()
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

			// Prepare batch of messages
			batch = batch[:0]
			for i := 0; i < batchSize; i++ {
				messageID++
				payload := payloadPool[payloadIndex]
				payloadIndex = (payloadIndex + 1) % len(payloadPool)

				// Update timestamp and message ID
				timestamp := time.Now().UnixNano()
				copy(payload[0:8], []byte(fmt.Sprintf("%08d", timestamp)))
				copy(payload[8:16], []byte(fmt.Sprintf("%08d", messageID)))

				batch = append(batch, append([]byte(nil), payload...))
			}

			// Send batch to publishers
			for _, msg := range batch {
				select {
				case publishChan <- msg:
					// Message queued successfully
				default:
					c.stats.mutex.Lock()
					c.stats.DroppedMessages++
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
