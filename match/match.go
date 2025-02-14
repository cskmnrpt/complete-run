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
	RunID  int    `json:"run_id"`
	CaseID int    `json:"case_id"`
	Status string `json:"status"`
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
	parts := strings.Split(strings.TrimSpace(string(content)), ",")
	var runIDs []int
	for _, part := range parts {
		var id int
		fmt.Sscanf(part, "%d", &id)
		runIDs = append(runIDs, id)
	}
	return runIDs
}

func fetchCasesForRunID(apiToken, projectCode string, runID int) ([]int, bool) {
	url := fmt.Sprintf("https://api.qase.io/v1/run/%s/%d?include=cases", projectCode, runID)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Add("accept", "application/json")
	req.Header.Add("Token", apiToken)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Println("API request failed:", err)
		return nil, false
	}
	defer res.Body.Close()

	body, _ := io.ReadAll(res.Body)
	var apiResp APIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		fmt.Println("Error parsing JSON response:", err)
		return nil, false
	}

	if !apiResp.Status || apiResp.Result.Status != 0 {
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
		}
	}
	return results
}

func validateRunCases(runID int, caseIDs []int, results []TestResult) bool {
	caseCount := make(map[int]int)
	for _, caseID := range caseIDs {
		caseCount[caseID]++
	}

	foundCases := make(map[int]int)
	passedCases := make(map[int]bool) // Track if all results for a case have passed

	for _, result := range results {
		if result.RunID == runID {
			foundCases[result.CaseID]++
			if result.Status != "passed" {
				passedCases[result.CaseID] = false
			} else if _, exists := passedCases[result.CaseID]; !exists {
				passedCases[result.CaseID] = true
			}
		}
	}

	for caseID, expectedCount := range caseCount {
		if foundCases[caseID] < expectedCount || !passedCases[caseID] {
			return false
		}
	}
	return true
}

func writeValidRunIDs(filename string, runIDs []string) {
	content := strings.Join(runIDs, ",")
	os.WriteFile(filename, []byte(content), 0644)
}
