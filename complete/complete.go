package complete

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type APIResponse struct {
	Status       bool   `json:"status"`
	ErrorMessage string `json:"errorMessage"`
}

type RunsAPIResponse struct {
	Status bool `json:"status"`
	Result struct {
		Total    int   `json:"total"`
		Filtered int   `json:"filtered"`
		Count    int   `json:"count"`
		Entities []Run `json:"entities"`
	} `json:"result"`
}

type Run struct {
	ID     int `json:"id"`
	Status int `json:"status"`
}

// RetryConfig holds configuration for retry mechanism
type RetryConfig struct {
	MaxRetries      int
	InitialDelay    time.Duration
	MaxDelay        time.Duration
	BackoffFactor   float64
	RequestTimeout  time.Duration
}

// Default retry configuration
var defaultRetryConfig = RetryConfig{
	MaxRetries:      3,
	InitialDelay:    500 * time.Millisecond,
	MaxDelay:        10 * time.Second,
	BackoffFactor:   2.0,
	RequestTimeout:  30 * time.Second,
}

// Create HTTP client with timeout
var httpClient = &http.Client{
	Timeout: defaultRetryConfig.RequestTimeout,
}

// isRetryableError determines if an error should be retried
func isRetryableError(err error, statusCode int) bool {
	if err != nil {
		// Network errors, timeouts, etc. are retryable
		return true
	}
	
	// HTTP status codes that are retryable
	switch statusCode {
	case 429: // Too Many Requests
		return true
	case 500, 502, 503, 504: // Server errors
		return true
	default:
		return false
	}
}

// calculateBackoffDelay calculates the delay for exponential backoff
func calculateBackoffDelay(attempt int, config RetryConfig) time.Duration {
	delay := time.Duration(float64(config.InitialDelay) * math.Pow(config.BackoffFactor, float64(attempt)))
	if delay > config.MaxDelay {
		delay = config.MaxDelay
	}
	return delay
}

// retryableHTTPRequest performs an HTTP request with retry logic
func retryableHTTPRequest(req *http.Request, config RetryConfig) (*http.Response, error) {
	var lastErr error
	var resp *http.Response
	
	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		// Use the HTTP client's timeout instead of context timeout to avoid conflicts
		resp, lastErr = httpClient.Do(req)
		
		if lastErr == nil && resp != nil {
			// Check if the status code indicates success or non-retryable error
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return resp, nil
			}
			
			if !isRetryableError(nil, resp.StatusCode) {
				return resp, fmt.Errorf("non-retryable HTTP error: %d", resp.StatusCode)
			}
			
			// Close the response body for retryable errors
			resp.Body.Close()
		}
		
		// Don't sleep after the last attempt
		if attempt < config.MaxRetries {
			delay := calculateBackoffDelay(attempt, config)
			fmt.Printf("Request failed (attempt %d/%d), retrying in %v...\n", 
				attempt+1, config.MaxRetries+1, delay)
			time.Sleep(delay)
		}
	}
	
	return resp, fmt.Errorf("request failed after %d attempts: %v", config.MaxRetries+1, lastErr)
}

func CompleteRuns() {
	apiToken := os.Getenv("QASE_API_TOKEN")
	projectCode := os.Getenv("QASE_PROJECT_CODE")
	if apiToken == "" || projectCode == "" {
		fmt.Println("Missing API token or project code in environment variables")
		return
	}

	runIDs := readRunIDs("final.txt")
	rateLimiter := time.Tick(200 * time.Millisecond) // 5 requests per second

	for _, runID := range runIDs {
		<-rateLimiter
		if !completeRun(apiToken, projectCode, runID) {
			logError(runID)
		}
	}
}

func readRunIDs(filename string) []int {
	content, err := os.ReadFile(filename)
	if err != nil {
		fmt.Println("Error reading file:", err)
		return nil
	}
	parts := strings.Split(strings.TrimSpace(string(content)), ",")
	var runIDs []int
	for _, part := range parts {
		var id int
		fmt.Sscanf(part, "%d", &id)
		runIDs = append(runIDs, id)
	}
	return runIDs
}

func completeRun(apiToken, projectCode string, runID int) bool {
	url := fmt.Sprintf("https://api.qase.io/v1/run/%s/%d/complete", projectCode, runID)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		fmt.Printf("Error creating request for run %d: %v\n", runID, err)
		return false
	}
	req.Header.Add("accept", "application/json")
	req.Header.Add("Token", apiToken)

	// Use a more aggressive retry config for completion calls
	completionRetryConfig := RetryConfig{
		MaxRetries:      2, // Fewer retries for completion to avoid duplicate operations
		InitialDelay:    300 * time.Millisecond,
		MaxDelay:        5 * time.Second,
		BackoffFactor:   2.0,
		RequestTimeout:  20 * time.Second,
	}

	res, err := retryableHTTPRequest(req, completionRetryConfig)
	if err != nil {
		fmt.Printf("API request failed for run %d after retries: %v ❌\n", runID, err)
		return false
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		fmt.Printf("Error reading response for run %d: %v ❌\n", runID, err)
		return false
	}

	var apiResp APIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		fmt.Printf("Error parsing JSON response for run %d: %v ❌\n", runID, err)
		return false
	}

	if apiResp.Status {
		fmt.Printf("Successfully marked Run ID %d as complete ✅\n", runID)
	} else {
		fmt.Printf("Failed to mark Run ID %d as complete (API returned false) ❌\n", runID)
		if apiResp.ErrorMessage != "" {
			fmt.Printf("  Error message: %s\n", apiResp.ErrorMessage)
		}
	}

	return apiResp.Status
}

func logError(runID int) {
	file, err := os.OpenFile("errors.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println("Error opening error log file:", err)
		return
	}
	defer file.Close()

	logger := bufio.NewWriter(file)
	logger.WriteString(fmt.Sprintf("Run ID %d: Test run not found\n", runID))
	logger.Flush()
}

// CompleteAllInProgressRuns fetches all in-progress test runs and marks them as complete
func CompleteAllInProgressRuns() {
	apiToken := os.Getenv("QASE_API_TOKEN")
	projectCode := os.Getenv("QASE_PROJECT_CODE")
	if apiToken == "" || projectCode == "" {
		fmt.Println("Missing API token or project code in environment variables")
		return
	}

	fmt.Println("Fetching all in-progress test runs...")
	inProgressRuns := fetchAllInProgressRuns(apiToken, projectCode)
	
	if len(inProgressRuns) == 0 {
		fmt.Println("No in-progress test runs found.")
		return
	}

	fmt.Printf("Found %d in-progress test runs. Starting completion process...\n", len(inProgressRuns))
	
	// Complete runs with rate limiting (3-5 calls per second)
	completeRunsInParallel(apiToken, projectCode, inProgressRuns)
}

// fetchAllInProgressRuns fetches all test runs and filters for in-progress ones
func fetchAllInProgressRuns(apiToken, projectCode string) []int {
	const limit = 100
	var allInProgressRuns []int
	offset := 0
	consecutiveFailures := 0
	maxConsecutiveFailures := 3

	fmt.Println("Starting to fetch test runs with robust retry mechanism...")

	for {
		url := fmt.Sprintf("https://api.qase.io/v1/run/%s?limit=%d&offset=%d", projectCode, limit, offset)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			fmt.Printf("Error creating request: %v\n", err)
			consecutiveFailures++
			if consecutiveFailures >= maxConsecutiveFailures {
				fmt.Printf("Too many consecutive failures (%d), stopping fetch process\n", consecutiveFailures)
				break
			}
			continue
		}
		req.Header.Add("accept", "application/json")
		req.Header.Add("Token", apiToken)

		fmt.Printf("Fetching runs at offset %d...\n", offset)
		resp, err := retryableHTTPRequest(req, defaultRetryConfig)
		if err != nil {
			fmt.Printf("Failed to fetch runs at offset %d after retries: %v\n", offset, err)
			consecutiveFailures++
			if consecutiveFailures >= maxConsecutiveFailures {
				fmt.Printf("Too many consecutive failures (%d), stopping fetch process\n", consecutiveFailures)
				break
			}
			// Skip this batch and try the next one
			offset += limit
			continue
		}
		defer resp.Body.Close()

		// Reset consecutive failures on successful request
		consecutiveFailures = 0

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			fmt.Printf("Error reading response: %v\n", err)
			offset += limit
			continue
		}

		var apiResp RunsAPIResponse
		if err := json.Unmarshal(body, &apiResp); err != nil {
			fmt.Printf("Error parsing JSON: %v\n", err)
			offset += limit
			continue
		}

		if !apiResp.Status {
			fmt.Printf("API response status is false at offset %d, skipping batch\n", offset)
			offset += limit
			continue
		}

		// Filter for in-progress runs (status = 0)
		batchInProgressCount := 0
		for _, run := range apiResp.Result.Entities {
			if run.Status == 0 { // 0 = in-progress
				allInProgressRuns = append(allInProgressRuns, run.ID)
				batchInProgressCount++
			}
		}

		fmt.Printf("✅ Fetched %d runs (offset: %d), found %d in-progress in this batch, %d total so far\n", 
			len(apiResp.Result.Entities), offset, batchInProgressCount, len(allInProgressRuns))

		// Check if we've fetched all runs
		if len(apiResp.Result.Entities) < limit {
			fmt.Println("Reached end of test runs")
			break
		}

		offset += limit
		
		// Small delay to be respectful to the API
		time.Sleep(200 * time.Millisecond)
	}

	fmt.Printf("Fetch complete. Found %d in-progress runs total\n", len(allInProgressRuns))
	return allInProgressRuns
}

// completeRunsInParallel completes runs with rate limiting (3-5 calls per second)
func completeRunsInParallel(apiToken, projectCode string, runIDs []int) {
	const maxConcurrent = 5
	const requestsPerSecond = 4 // 4 requests per second to stay within 3-5 range
	
	semaphore := make(chan struct{}, maxConcurrent)
	rateLimiter := time.Tick(time.Second / requestsPerSecond)
	
	var wg sync.WaitGroup
	var successCount, errorCount int
	var mu sync.Mutex

	for _, runID := range runIDs {
		wg.Add(1)
		
		go func(id int) {
			defer wg.Done()
			
			// Rate limiting
			<-rateLimiter
			semaphore <- struct{}{} // Acquire semaphore
			defer func() { <-semaphore }() // Release semaphore
			
			success := completeRun(apiToken, projectCode, id)
			
			mu.Lock()
			if success {
				successCount++
			} else {
				errorCount++
				logError(id)
			}
			mu.Unlock()
		}(runID)
	}

	wg.Wait()
	
	fmt.Printf("\nCompletion Summary:\n")
	fmt.Printf("✅ Successfully completed: %d runs\n", successCount)
	fmt.Printf("❌ Failed to complete: %d runs\n", errorCount)
	if errorCount > 0 {
		fmt.Printf("Check errors.txt for details on failed runs\n")
	}
}
