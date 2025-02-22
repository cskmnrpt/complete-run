package filter

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

type TestResult struct {
	Attachments []interface{} `json:"attachments"`
	CaseID      int           `json:"case_id"`
	Comment     *string       `json:"comment"`
	EndTime     string        `json:"end_time"`
	Hash        string        `json:"hash"`
	IsAPIResult bool          `json:"is_api_result"`
	RunID       int           `json:"run_id"`
	StackTrace  *string       `json:"stacktrace"`
	Status      string        `json:"status"`
	Steps       *interface{}  `json:"steps"`
	TimeSpentMS int           `json:"time_spent_ms"`
}

func FilterResults() {
	inputFile := "results.json"
	outputFile := "filtered.txt"

	file, err := os.Open(inputFile)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	runResults := make(map[int][]TestResult)

	// Read and parse each line
	for scanner.Scan() {
		var result TestResult
		if err := json.Unmarshal(scanner.Bytes(), &result); err != nil {
			fmt.Println("Error parsing JSON:", err)
			continue
		}
		runResults[result.RunID] = append(runResults[result.RunID], result)
	}

	if err := scanner.Err(); err != nil {
		fmt.Println("Error reading file:", err)
		return
	}

	selectedRunIDs := processResults(runResults)

	// Write the selected run_ids to a file
	writeOutput(selectedRunIDs, outputFile)
}

func processResults(runResults map[int][]TestResult) []int {
	var selectedRunIDs []int

	for runID, results := range runResults {
		allPassed := true
		caseStatuses := make(map[int][]TestResult)

		for _, result := range results {
			if result.Status != "passed" {
				allPassed = false
			}
			caseStatuses[result.CaseID] = append(caseStatuses[result.CaseID], result)
		}

		if allPassed {
			selectedRunIDs = append(selectedRunIDs, runID)
			continue
		}

		keepRunID := false
		for _, caseResults := range caseStatuses {
			hasPassed := false
			latestPassedTime := ""
			latestOverallTime := ""
			latestOverallStatus := ""

			for _, result := range caseResults {
				// Track the latest overall result (regardless of status)
				if result.EndTime > latestOverallTime {
					latestOverallTime = result.EndTime
					latestOverallStatus = result.Status
				}

				// Track the latest passed result
				if result.Status == "passed" {
					hasPassed = true
					if result.EndTime > latestPassedTime {
						latestPassedTime = result.EndTime
					}
				}
			}

			if hasPassed {
				// Only keep the run if the latest result overall is a "passed" result
				if latestPassedTime == latestOverallTime && latestOverallStatus == "passed" {
					keepRunID = true
				} else {
					keepRunID = false
					break // No need to check further, discard the run
				}
			} else {
				// If there's no pass at all for this case, discard the run
				keepRunID = false
				break
			}
		}

		if keepRunID {
			selectedRunIDs = append(selectedRunIDs, runID)
		}
	}

	sort.Ints(selectedRunIDs)
	return selectedRunIDs
}

func writeOutput(runIDs []int, outputFile string) {
	file, err := os.Create(outputFile)
	if err != nil {
		fmt.Println("Error creating output file:", err)
		return
	}
	defer file.Close()

	output := strings.Trim(strings.Join(strings.Fields(fmt.Sprint(runIDs)), ","), "[]")
	_, err = file.WriteString(output)
	if err != nil {
		fmt.Println("Error writing to file:", err)
	}
}
