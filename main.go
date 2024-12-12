package main

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/schollz/progressbar/v3"
)

func main() {
	config := LoadConfig()

	// Ensure we have equal numbers of publishers and subscribers
	if config.NumClients != config.SubClients {
		config.NumClients = config.SubClients
	}

	// Verify the publish rate is being set correctly
	fmt.Printf("Publishing rate per client: %d messages/second\n", config.PublishRate)

	stats := NewStats()

	// Create progress bar
	bar := progressbar.NewOptions(config.TestDuration,
		progressbar.OptionSetDescription("Running test"),
		progressbar.OptionSetWidth(50),
		progressbar.OptionShowCount(),
		progressbar.OptionShowIts(),
		progressbar.OptionSetItsString("s"),
	)

	// Create publisher clients
	publishers := make([]*Client, config.NumClients)
	subscribers := make([]*Client, config.SubClients)
	var wg sync.WaitGroup

	// Create subscriber clients in parallel
	fmt.Printf("Initializing %d pairs of publishers and subscribers...\n", config.SubClients)
	subWg := sync.WaitGroup{}
	subChan := make(chan error, config.SubClients)

	for i := 0; i < config.SubClients; i++ {
		subWg.Add(1)
		go func(index int) {
			defer subWg.Done()
			client, err := NewClient(index*2, config, stats, true)
			if err != nil {
				fmt.Printf("Error creating subscriber %d: %v\n", index, err)
				subChan <- err
				return
			}
			subscribers[index] = client
			go client.Start()
		}(i)
	}

	// Wait for all subscribers to be created
	subWg.Wait()
	close(subChan)

	// Check for subscriber creation errors
	errCount := 0
	for err := range subChan {
		if err != nil {
			errCount++
		}
	}
	if errCount > 0 {
		fmt.Printf("Warning: %d subscribers failed to initialize\n", errCount)
	}

	// Increase wait time for subscribers to connect and subscribe
	time.Sleep(3 * time.Second)

	// Create publisher clients in parallel
	fmt.Printf("Creating %d publisher clients...\n", config.NumClients)
	pubWg := sync.WaitGroup{}
	pubChan := make(chan error, config.NumClients)

	for i := 0; i < config.NumClients; i++ {
		pubWg.Add(1)
		go func(index int) {
			defer pubWg.Done()
			client, err := NewClient(index*2+1, config, stats, false)
			if err != nil {
				fmt.Printf("Error creating publisher %d: %v\n", index, err)
				pubChan <- err
				return
			}
			publishers[index] = client
		}(i)
	}

	// Wait for all publishers to be created
	pubWg.Wait()
	close(pubChan)

	// Check for publisher creation errors
	errCount = 0
	for err := range pubChan {
		if err != nil {
			errCount++
		}
	}
	if errCount > 0 {
		fmt.Printf("Warning: %d publishers failed to initialize\n", errCount)
	}

	// Set synchronized start time for all publishers
	publishStartTime := time.Now().Add(2 * time.Second)
	
	// Start all publishers simultaneously
	for _, pub := range publishers {
		if pub != nil {
			pub.SetStartTime(publishStartTime)
			wg.Add(1)
			go func(c *Client) {
				defer wg.Done()
				c.Start()
			}(pub)
		}
	}

	// Wait a moment for all clients to be ready
	time.Sleep(time.Second)
	
	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	
	// Create a done channel to signal completion
	done := make(chan struct{})

	startTime := time.Now()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	go func() {
		for range ticker.C {
			elapsed := time.Since(startTime).Seconds()
			_ = bar.Set(int(elapsed))

			if int(elapsed) >= config.TestDuration {
				close(done)
				return
			}

			// Print statistics every 5 seconds
			if int(elapsed)%5 == 0 {
				fmt.Printf("\n=== Test Statistics (%.1f seconds elapsed) ===\n", elapsed)
				fmt.Printf("Messages: Pub=%d, Sub=%d, Rate=%.2f msg/sec, Errors=%d\n",
					stats.PublishedMessages, stats.ReceivedMessages,
					float64(stats.PublishedMessages)/elapsed, stats.Errors)
			}
		}
	}()

	// Wait for either completion or interrupt
	select {
	case <-done:
		fmt.Println("\nTest completed, shutting down...")
	case <-sigChan:
		fmt.Println("\nReceived interrupt signal, shutting down...")
	}

	// Stop all clients with a timeout
	stopChan := make(chan struct{})
	go func() {
		var wg sync.WaitGroup
		
		// Stop publishers first
		for _, pub := range publishers {
			if pub != nil {
				wg.Add(1)
				go func(c *Client) {
					defer wg.Done()
					c.Stop()
				}(pub)
			}
		}
		wg.Wait()

		// Then stop subscribers
		for _, sub := range subscribers {
			if sub != nil {
				wg.Add(1)
				go func(c *Client) {
					defer wg.Done()
					c.Stop()
				}(sub)
			}
		}
		wg.Wait()
		
		close(stopChan)
	}()

	// Wait for shutdown with timeout
	select {
	case <-stopChan:
		fmt.Println("All clients stopped successfully")
	case <-time.After(5 * time.Second):
		fmt.Println("Shutdown timed out")
	}

	// Print final statistics
	printFinalStats(stats, config, time.Since(startTime).Seconds())
}

func printFinalStats(stats *Stats, config *Config, elapsed float64) {
    fmt.Printf("\n=== Final Test Results ===\n")
    fmt.Printf("Total Publishers: %d\n", config.NumClients)
    fmt.Printf("Total Subscribers: %d\n", config.SubClients)
	fmt.Printf("Message Rate Per Client: %d\n", config.PublishRate)
    fmt.Printf("Test Duration: %.2f seconds\n", elapsed)
    
    fmt.Printf("\n=== Message Statistics ===\n")
    fmt.Printf("Total Messages Published: %d\n", stats.PublishedMessages)
    fmt.Printf("Total Messages Received: %d\n", stats.ReceivedMessages)
    lossCount := stats.PublishedMessages - stats.ReceivedMessages
    lossRate := 100 - float64(stats.ReceivedMessages)/float64(stats.PublishedMessages)*100
    fmt.Printf("Messages Lost: %d (%.2f%%)\n", lossCount, lossRate)
    fmt.Printf("Dropped Messages: %d\n", stats.DroppedMessages)
    fmt.Printf("Duplicate Messages: %d\n", stats.DuplicateMessages)
    fmt.Printf("Out of Order Messages: %d\n", stats.OutOfOrderMessages)
    fmt.Printf("Processing Errors: %d\n", stats.ProcessingErrors)
    
    fmt.Printf("\n=== Performance Metrics ===\n")
    fmt.Printf("Average Publishing Rate: %.2f msg/sec\n",
        float64(stats.PublishedMessages)/elapsed)
    fmt.Printf("Average Receiving Rate: %.2f msg/sec\n",
        float64(stats.ReceivedMessages)/elapsed)
    fmt.Printf("Average Publishing Rate Per Channel: %.2f msg/sec\n",
        float64(stats.PublishedMessages)/(elapsed*float64(config.NumClients)))
    fmt.Printf("Average Receiving Rate Per Channel: %.2f msg/sec\n",
        float64(stats.ReceivedMessages)/(elapsed*float64(config.SubClients)))
    
    fmt.Printf("\n=== Error Statistics ===\n")
    fmt.Printf("Total Errors: %d\n", stats.Errors)
    fmt.Printf("Retry Attempts: %d\n", stats.RetryAttempts)
    fmt.Printf("Successful Retries: %d\n", stats.RetrySuccesses)
    fmt.Printf("Timeout Errors: %d\n", stats.TimeoutErrors)
    fmt.Printf("Connection Errors: %d\n", stats.ConnectionErrors)
    
    if stats.RetryAttempts > 0 {
        fmt.Printf("Retry Success Rate: %.2f%%\n", 
            float64(stats.RetrySuccesses)/float64(stats.RetryAttempts)*100)
    }
}
