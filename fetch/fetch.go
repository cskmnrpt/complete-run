package fetch

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
)

const limit = 100             // Number of results per request
const maxParallelRequests = 6 // Max parallel requests per second

var (
	apiToken    = os.Getenv("QASE_API_TOKEN")
	projectCode = os.Getenv("QASE_PROJECT_CODE")
	outputFile  = "results.json"
	client      = &http.Client{}
	mutex       = &sync.Mutex{}
	wg          sync.WaitGroup
	rateLimiter = time.Tick(time.Second / maxParallelRequests) // Rate limiting mechanism
)

type APIResponse struct {
	Status bool `json:"status"`
	Result struct {
		Total    int                      `json:"total"`
		Filtered int                      `json:"filtered"`
		Count    int                      `json:"count"`
		Entities []map[string]interface{} `json:"entities"`
	} `json:"result"`
}

func fetchResults(offset int, resultsChan chan<- []map[string]interface{}) {
	defer wg.Done()
	<-rateLimiter // Enforce rate limiting

	url := fmt.Sprintf("https://api.qase.io/v1/result/%s?limit=%d&offset=%d", projectCode, limit, offset)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Println("Error creating request:", err)
		return
	}
	req.Header.Add("accept", "application/json")
	req.Header.Add("Token", apiToken)

	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Request error:", err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error reading response:", err)
		return
	}

	var apiResp APIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		fmt.Println("Error parsing JSON:", err)
		return
	}

	if !apiResp.Status {
		fmt.Println("API response status is false")
		return
	}

	resultsChan <- apiResp.Result.Entities
}

func saveResultsToFile(results []map[string]interface{}) {
	mutex.Lock()
	defer mutex.Unlock()

	file, err := os.OpenFile(outputFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	for _, result := range results {
		if err := encoder.Encode(result); err != nil {
			fmt.Println("Error writing to file:", err)
		}
	}
}

func FetchResults() {
	if apiToken == "" || projectCode == "" {
		fmt.Println("Missing required environment variables: QASE_API_TOKEN and QASE_PROJECT_CODE")
		return
	}

	// Fetch initial result to get total count
	url := fmt.Sprintf("https://api.qase.io/v1/result/%s?limit=1&offset=0", projectCode)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Add("accept", "application/json")
	req.Header.Add("Token", apiToken)

	res, err := client.Do(req)
	if err != nil {
		fmt.Println("Error making initial request:", err)
		return
	}
	defer res.Body.Close()

	body, _ := io.ReadAll(res.Body)

	var initialResp APIResponse
	if err := json.Unmarshal(body, &initialResp); err != nil {
		fmt.Println("Error parsing initial response:", err)
		return
	}

	totalResults := initialResp.Result.Total
	fmt.Println("Total results to fetch:", totalResults)

	resultsChan := make(chan []map[string]interface{}, maxParallelRequests)

	// Launch workers to fetch data in parallel
	for offset := 0; offset < totalResults; offset += limit {
		wg.Add(1)
		go fetchResults(offset, resultsChan)
	}

	// Close channel when all fetches are done
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Collect results and write to file
	for results := range resultsChan {
		saveResultsToFile(results)
	}

	fmt.Println("Fetching complete. Results saved to", outputFile)
}
