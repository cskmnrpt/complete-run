package match

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
	Status bool `json:"status"`
	Result struct {
		ID     int   `json:"id"`
		Status int   `json:"status"`
		Cases  []int `json:"cases"`
	} `json:"result"`
}

type TestResult struct {
	RunID   int    `json:"run_id"`
	CaseID  int    `json:"case_id"`
	Status  string `json:"status"`
	EndTime string `json:"end_time"`
}

func MatchResults() {
	apiToken := os.Getenv("QASE_API_TOKEN")
	projectCode := os.Getenv("QASE_PROJECT_CODE")
	if apiToken == "" || projectCode == "" {
		fmt.Println("Missing API token or project code in environment variables")
		return
	}

	runIDs := readRunIDs("filtered.txt")
	results := readResults("results.json")
	validRunIDs := []string{}

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 5) // Limit to 5 requests per second

	var mu sync.Mutex

	for _, runID := range runIDs {
		wg.Add(1)
		semaphore <- struct{}{} // Acquire a slot
		go func(runID int) {
			defer wg.Done()
			cases, valid := fetchCasesForRunID(apiToken, projectCode, runID)
			if valid && validateRunCases(runID, cases, results) {
				mu.Lock()
				validRunIDs = append(validRunIDs, fmt.Sprintf("%d", runID))
				mu.Unlock()
			}
			time.Sleep(200 * time.Millisecond) // Maintain rate limit
			<-semaphore                        // Release a slot
		}(runID)
	}

	wg.Wait()
	writeValidRunIDs("final.txt", validRunIDs)
}

func readRunIDs(filename string) []int {
	content, err := os.ReadFile(filename)
	if err != nil {
		fmt.Println("Error reading file:", err)
		return nil
	}
	fmt.Printf("Contents of %s: %s\n", filename, string(content))

	parts := strings.Split(strings.TrimSpace(string(content)), ",")
	var runIDs []int
	for _, part := range parts {
		var id int
		fmt.Sscanf(part, "%d", &id)
		runIDs = append(runIDs, id)
	}
	fmt.Printf("Parsed Run IDs: %v\n", runIDs)
	return runIDs
}

func fetchCasesForRunID(apiToken, projectCode string, runID int) ([]int, bool) {
	url := fmt.Sprintf("https://api.qase.io/v1/run/%s/%d?include=cases", projectCode, runID)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Add("accept", "application/json")
	req.Header.Add("Token", apiToken)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("API request failed for runID %d: %v\n", runID, err)
		return nil, false
	}
	defer res.Body.Close()

	body, _ := io.ReadAll(res.Body)
	fmt.Printf("API Response for runID %d: %s\n", runID, string(body))

	var apiResp APIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		fmt.Printf("Error parsing JSON response for runID %d: %v\n", runID, err)
		return nil, false
	}

	if !apiResp.Status || apiResp.Result.Status != 0 {
		fmt.Printf("Invalid API response for runID %d (Status: %d)\n", runID, apiResp.Result.Status)
		return nil, false
	}

	return apiResp.Result.Cases, true
}

func readResults(filename string) []TestResult {
	file, err := os.Open(filename)
	if err != nil {
		fmt.Println("Error opening results file:", err)
		return nil
	}
	defer file.Close()

	var results []TestResult
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var result TestResult
		if err := json.Unmarshal([]byte(scanner.Text()), &result); err == nil {
			results = append(results, result)
		} else {
			fmt.Printf("Error parsing test result JSON: %s\n", scanner.Text())
		}
	}
	fmt.Printf("Total test results read: %d\n", len(results))
	return results
}

func validateRunCases(runID int, caseIDs []int, results []TestResult) bool {
	fmt.Printf("Validating runID: %d with expected cases: %v\n", runID, caseIDs)

	foundCases := make(map[int]int)
	latestPassTime := make(map[int]string)
	passedCases := make(map[int]bool)

	for _, result := range results {
		if result.RunID == runID {
			foundCases[result.CaseID]++
			if result.Status == "passed" {
				passedCases[result.CaseID] = true
				if latestPassTime[result.CaseID] == "" || result.EndTime > latestPassTime[result.CaseID] {
					latestPassTime[result.CaseID] = result.EndTime
				}
			}
		}
	}

	for _, result := range results {
		if result.RunID == runID && result.Status != "passed" {
			if latestPassTime[result.CaseID] != "" && result.EndTime > latestPassTime[result.CaseID] {
				fmt.Printf("RunID %d failed validation: Case %d has a non-passed result (%s) after latest pass at %s\n",
					runID, result.CaseID, result.Status, latestPassTime[result.CaseID])
				return false
			}
		}
	}

	fmt.Printf("RunID %d is valid\n", runID)
	return true
}

func writeValidRunIDs(filename string, runIDs []string) {
	fmt.Printf("Final list of valid runIDs to be written: %v\n", runIDs)
	content := strings.Join(runIDs, ",")
	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		fmt.Printf("Error writing to file %s: %v\n", filename, err)
	}
}
