package complete

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type APIResponse struct {
	Status       bool   `json:"status"`
	ErrorMessage string `json:"errorMessage"`
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
