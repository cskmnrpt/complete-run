package main

import (
	"complete_run/complete"
	"complete_run/fetch"
	"complete_run/filter"
	"complete_run/match"
	"fmt"
)

func main() {
	fmt.Println("Starting Qase Automation Pipeline...")

	fetch.FetchResults()
	filter.FilterResults()
	match.MatchResults()
	complete.CompleteRuns()

	fmt.Println("Pipeline execution finished successfully!")
}
