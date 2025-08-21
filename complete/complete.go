package complete

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
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
	req, _ := http.NewRequest("POST", url, nil)
	req.Header.Add("accept", "application/json")
	req.Header.Add("Token", apiToken)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Println("API request failed:", err)
		return false
	}
	defer res.Body.Close()

	body, _ := io.ReadAll(res.Body)
	var apiResp APIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		fmt.Println("Error parsing JSON response:", err)
		return false
	}

	if apiResp.Status {
		fmt.Printf("Successfully marked Run ID %d as complete ✅\n", runID) // <-- Success message
	} else {
		fmt.Printf("Failed to mark Run ID %d as complete ❌\n", runID)
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

	for {
		url := fmt.Sprintf("https://api.qase.io/v1/run/%s?limit=%d&offset=%d", projectCode, limit, offset)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			fmt.Printf("Error creating request: %v\n", err)
			break
		}
		req.Header.Add("accept", "application/json")
		req.Header.Add("Token", apiToken)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			fmt.Printf("Request error: %v\n", err)
			break
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			fmt.Printf("Error reading response: %v\n", err)
			break
		}

		var apiResp RunsAPIResponse
		if err := json.Unmarshal(body, &apiResp); err != nil {
			fmt.Printf("Error parsing JSON: %v\n", err)
			break
		}

		if !apiResp.Status {
			fmt.Println("API response status is false")
			break
		}

		// Filter for in-progress runs (status = 0)
		for _, run := range apiResp.Result.Entities {
			if run.Status == 0 { // 0 = in-progress
				allInProgressRuns = append(allInProgressRuns, run.ID)
			}
		}

		fmt.Printf("Fetched %d runs (offset: %d), found %d in-progress so far\n", 
			len(apiResp.Result.Entities), offset, len(allInProgressRuns))

		// Check if we've fetched all runs
		if len(apiResp.Result.Entities) < limit {
			break
		}

		offset += limit
		
		// Small delay to be respectful to the API
		time.Sleep(100 * time.Millisecond)
	}

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
