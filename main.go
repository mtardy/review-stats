package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	owner             = flag.String("owner", "", "GitHub repository owner (required)")
	repo              = flag.String("repo", "", "GitHub repository name (required)")
	numWorkers        = flag.Int("workers", 50, "Number of parallel workers")
	timeWindowDays    = flag.Int("days", 365, "Number of days to look back for PRs")
	cacheDir          = flag.String("cache", "review-stats-cache", "Directory to cache GitHub API responses")
	minReviewsPercent = flag.Float64("min-reviews-percent", 0.5, "Minimum percentage of total reviews to include a reviewer (e.g., 0.5 = 0.5%)")
	excludeAuthors    = flag.String("exclude-authors", "", "Comma-separated list of PR author usernames to exclude (e.g., 'cilium-renovate[bot],dependabot[bot]')")
	sortBy            = flag.String("sort", "rate", "Sort reviewers by: rate, requested, completed, pending, response, approved, commented, changes")
)

func processPR(pr PullRequest) PRResult {
	result := PRResult{
		PR:                 pr,
		CompletedReviewers: make(map[string]ReviewDetail),
	}

	// Fetch pending reviewers (still waiting)
	pendingReviewers, reviewersTrusted, reviewersFromCache, err := fetchRequestedReviewers(pr.Number, pr.State)
	if err != nil {
		result.Err = fmt.Errorf("could not fetch requested reviewers: %w", err)
		return result
	}
	result.PendingReviewers = pendingReviewers

	// Fetch completed reviews
	reviews, reviewsTrusted, reviewsFromCache, err := fetchReviews(pr.Number, pr.State)
	if err != nil {
		result.Err = fmt.Errorf("could not fetch reviews: %w", err)
		return result
	}

	// Determine overall cache status
	result.Trusted = reviewersTrusted && reviewsTrusted
	result.FromCache = reviewersFromCache && reviewsFromCache

	// Track who completed reviews (keep the most recent review per reviewer)
	for _, r := range reviews {
		login := r.User.Login
		// Skip PR author's own comments or empty logins
		if login == "" || login == pr.User.Login {
			continue
		}
		// Only count meaningful review states
		switch r.State {
		case "APPROVED", "CHANGES_REQUESTED", "COMMENTED":
			if existing, ok := result.CompletedReviewers[login]; !ok || r.SubmittedAt.After(existing.SubmittedAt) {
				result.CompletedReviewers[login] = ReviewDetail{
					SubmittedAt: r.SubmittedAt,
					State:       r.State,
				}
			}
		}
	}

	return result
}

func main() {
	flag.Parse()

	// Validate required flags
	if *owner == "" || *repo == "" {
		fmt.Fprintln(os.Stderr, "Error: -owner and -repo flags are required")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Usage:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Initialize cache
	cacheDirPath = *cacheDir
	if cacheDirPath != "" {
		useCache = true
		if files, err := os.ReadDir(cacheDirPath); err == nil && len(files) > 0 {
			fmt.Printf("📂 Using cache: %s (%d files)\n", cacheDirPath, len(files))
		}
	}

	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Printf("║       GitHub Review Stats — %-32s ║\n", *owner+"/"+*repo)
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()

	if os.Getenv("GITHUB_TOKEN") == "" {
		fmt.Println("💡 Tip: Set GITHUB_TOKEN env variable to avoid rate limits.")
		fmt.Println("   export GITHUB_TOKEN=ghp_your_token_here")
		fmt.Println()
	}

	// Calculate cutoff date
	cutoffDate := time.Now().AddDate(0, 0, -*timeWindowDays)
	fmt.Printf("📅 Analyzing PRs created after: %s (last %d days)\n\n", cutoffDate.Format("2006-01-02"), *timeWindowDays)

	fmt.Println("📥 Fetching open PRs...")
	openPRs, err := fetchPullRequests("open", cutoffDate)
	if err != nil {
		log.Printf("Warning fetching open PRs: %v", err)
	}

	fmt.Println("📥 Fetching recently closed PRs...")
	closedPRs, err := fetchPullRequests("closed", cutoffDate)
	if err != nil {
		log.Printf("Warning fetching closed PRs: %v", err)
	}

	allPRs := append(openPRs, closedPRs...)
	fmt.Printf("\n📊 Analyzing %d PRs (%d open, %d closed)...\n",
		len(allPRs), len(openPRs), len(closedPRs))

	stats := make(map[string]*ReviewerStats)
	var pendingDetails []PendingReviewDetail

	// Create channels for work distribution
	prChan := make(chan PullRequest, len(allPRs))
	resultChan := make(chan PRResult, len(allPRs))

	// Start worker pool
	var wg sync.WaitGroup
	for range *numWorkers {
		wg.Go(func() {
			for pr := range prChan {
				result := processPR(pr)
				resultChan <- result
			}
		})
	}

	// Parse excluded authors
	var excludedAuthors map[string]bool
	if *excludeAuthors != "" {
		excludedAuthors = make(map[string]bool)
		for author := range strings.SplitSeq(*excludeAuthors, ",") {
			excludedAuthors[strings.TrimSpace(author)] = true
		}
	}

	// Count PRs to process (after filtering)
	prsToProcess := 0
	openToProcess := 0
	closedToProcess := 0
	for _, pr := range allPRs {
		if !excludedAuthors[pr.User.Login] {
			prsToProcess++
			if pr.State == "open" {
				openToProcess++
			} else {
				closedToProcess++
			}
		}
	}

	// Show filtered count if filtering is active
	if len(excludedAuthors) > 0 && prsToProcess < len(allPRs) {
		fmt.Printf("🔍 After author filtering: %d PRs (%d open, %d closed)\n",
			prsToProcess, openToProcess, closedToProcess)
	}
	fmt.Println()

	// Send PRs to workers (with optional filtering)
	go func() {
		for _, pr := range allPRs {
			// Skip PRs from excluded authors
			if excludedAuthors[pr.User.Login] {
				continue
			}
			prChan <- pr
		}
		close(prChan)
	}()

	// Wait for all workers to finish and close result channel
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results
	processed := 0
	for result := range resultChan {
		processed++
		fmt.Printf("  %s [%d/%d] PR #%d: %s\n",
			cacheStatusIcon(result.Trusted, result.FromCache),
			processed,
			prsToProcess,
			result.PR.Number,
			truncate(result.PR.Title, 50))

		if result.Err != nil {
			log.Printf("    ⚠️  Error processing PR: %v", result.Err)
			continue
		}

		// Record completed reviewers
		for login, detail := range result.CompletedReviewers {
			if _, ok := stats[login]; !ok {
				stats[login] = &ReviewerStats{}
			}
			stats[login].Requested++
			stats[login].Completed++

			// Track review type
			switch detail.State {
			case "APPROVED":
				stats[login].Approved++
			case "CHANGES_REQUESTED":
				stats[login].ChangesRequested++
			case "COMMENTED":
				stats[login].Commented++
			}

			responseTime := detail.SubmittedAt.Sub(result.PR.CreatedAt)
			if responseTime > 0 {
				stats[login].TotalResponse += responseTime
				stats[login].ResponseCount++
			}
		}

		// Record pending (not yet reviewed) reviewers
		for _, login := range result.PendingReviewers {
			if _, ok := stats[login]; !ok {
				stats[login] = &ReviewerStats{}
			}
			stats[login].Requested++
			stats[login].Pending++

			if result.PR.State == "open" {
				pendingDetails = append(pendingDetails, PendingReviewDetail{
					PRNumber: result.PR.Number,
					PRTitle:  result.PR.Title,
					PRURL:    result.PR.HTMLURL,
					PRAge:    time.Since(result.PR.CreatedAt),
				})
			}
		}
	}

	printPendingDetails(pendingDetails)
	printStats(stats, *sortBy)
	printSummary(stats)
}
