package main

import (
	"complete_run/complete"
	"complete_run/fetch"
	"complete_run/filter"
	"complete_run/match"
	"flag"
	"fmt"
)

func main() {
	completeAll := flag.Bool("complete-all", false, "Mark all in-progress test runs as complete")
	flag.Parse()

	if *completeAll {
		fmt.Println("Starting Complete All In-Progress Runs...")
		complete.CompleteAllInProgressRuns()
		fmt.Println("Complete All execution finished successfully!")
		return
	}

	fmt.Println("Starting Qase Automation Pipeline...")

	fetch.FetchResults()
	filter.FilterResults()
	match.MatchResults()
	complete.CompleteRuns()

	fmt.Println("Pipeline execution finished successfully!")
}
