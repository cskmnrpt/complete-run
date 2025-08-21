# complete-run
A script to complete all test runs that have nothing but passed results in them, or to complete all in-progress test runs.

<br>

## Environment Variables
The script requires the following environment variables:
- `QASE_API_TOKEN`: Authentication token for QASE API.
- `QASE_PROJECT_CODE`: Project code for identifying test runs.

API Token and project code can be defined in your repository `secrets` and `variables` respectively. Alternatively, they can be provided while starting the workflow in the Actions tab.

It's recommended to define the token in `secrets` to avoid it from being printed in the logs. Values provided in the workflow will always override.

---

<br>

## Usage

### Default Mode (Pipeline)
Run the script without any flags to execute the full pipeline:
```bash
go run main.go
```

### Complete All In-Progress Runs
Use the `--complete-all` flag to mark all in-progress test runs as complete:
```bash
go run main.go --complete-all
```

This mode:
- Fetches all test runs from the project using pagination (100 runs per request)
- Filters for runs with `status = 0` (in-progress)
- Marks all in-progress runs as complete using parallel API calls
- Uses rate limiting (4 requests per second) to respect API limits
- Provides a completion summary with success/error counts

---

<br>

## Workflow Overview

### Default Pipeline Mode

#### 1. Fetching Test Results
- Fetch test results from the QASE API.
- Store them in `results.json`, with each line containing one JSON object.

#### 2. Filtering Results
- Read `results.json` line by line.
- Group results by `run_id`.
- If all results of a `run_id` have `status = "passed"`, select the `run_id`.
- If any results within a `run_id` have a non-passed status:
  - Ensure every `case_id` with a failed result has at least one corresponding `passed` result.
  - If so, only keep the latest `passed` result.
- Write selected `run_id`s to `filtered.txt`.

#### 3. Matching with API Data
- Read `filtered.txt` to retrieve `run_id`s.
- Make an API call to `https://api.qase.io/v1/run/<project-code>/<run_id>?include=cases`.
- If API response contains `"status": 0` (i.e., `in_progress`), proceed.
- Find all matching `run_id` entries in `results.json`.
- Validate each case:
  - Every `case_id` in API response must exist in `results.json` for that `run_id`.
  - If a `case_id` appears multiple times in API response, it must appear at least as many times in `results.json`.
- Write valid `run_id`s to `final.txt`.

#### 4. Completing Runs
- Read `final.txt` to extract valid `run_id`s.
- Make API calls to mark each test run as complete.
- Use rate limiting (max 5 requests per second).
- If an error occurs (`"status": false` in API response), log the `run_id` in `errors.txt`.

### Complete All Mode (`--complete-all`)

This mode bypasses the filtering and validation pipeline and directly:

#### 1. Fetching All Test Runs
- Fetches all test runs from the project using pagination
- Makes API calls to `https://api.qase.io/v1/run/<project-code>?limit=100&offset=<offset>`
- Continues fetching until all runs are retrieved (handles projects with >100 runs)

#### 2. Filtering In-Progress Runs
- Filters runs where `status = 0` (in-progress status)
- Collects all in-progress run IDs for completion

#### 3. Parallel Completion
- Marks all in-progress runs as complete using parallel API calls
- Uses rate limiting (4 requests per second) to respect API limits
- Provides real-time progress updates and final completion summary
- Logs any failed completions to `errors.txt`

---


## Output Files
| File Name       | Description |
|----------------|-------------|
| `results.json` | Raw test results from QASE API. |
| `filtered.txt` | `run_id`s that passed filtering. |
| `final.txt`    | `run_id`s validated against API data. |
| `errors.txt`   | Logs of test runs that could not be completed. |

---

## Execution Order
1. **Fetch results:** `fetch.FetchResults()`
2. **Filter results:** `filter.FilterResults()`
3. **Match API data:** `match.MatchResults()`
4. **Complete runs:** `complete.CompleteRuns()`

---

## Rate Limiting
- **Default Pipeline Mode**: The script enforces a limit of **5 API requests per second**.
- **Complete All Mode**: Uses **4 API requests per second** for completion calls to stay within the 3-5 requests/second range.
- This prevents exceeding QASE's API rate limits in both modes.

---

## Error Handling
- If API requests fail, they are logged in `errors.txt`.
- If a test run fails validation, it is discarded.
- Any JSON parsing or file I/O errors are logged in the console.
