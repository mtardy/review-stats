package main

import (
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"text/tabwriter"
	"time"
)

type sortOption string

const (
	sortRate      sortOption = "rate"
	sortRequested sortOption = "requested"
	sortCompleted sortOption = "completed"
	sortPending   sortOption = "pending"
	sortResponse  sortOption = "response"
	sortApproved  sortOption = "approved"
	sortCommented sortOption = "commented"
	sortChanges   sortOption = "changes"
)

type reviewerKV struct {
	Key   string
	Value *ReviewerStats
}

// sortAliases maps alternative names to canonical sort options
var sortAliases = map[string]sortOption{
	"request":  sortRequested,
	"requests": sortRequested,
	"req":      sortRequested,
	"reqs":     sortRequested,

	"complete": sortCompleted,
	"done":     sortCompleted,
	"comp":     sortCompleted,

	"pend":    sortPending,
	"waiting": sortPending,
	"wait":    sortPending,

	"resp":         sortResponse,
	"responsetime": sortResponse,
	"time":         sortResponse,
	"avgresponse":  sortResponse,
	"avg":          sortResponse,

	"approve":   sortApproved,
	"approvals": sortApproved,
	"approval":  sortApproved,
	"app":       sortApproved,

	"comment":  sortCommented,
	"comments": sortCommented,
	"com":      sortCommented,

	"change":           sortChanges,
	"changesrequested": sortChanges,
	"changesreq":       sortChanges,
	"cr":               sortChanges,

	"completion":     sortRate,
	"completionrate": sortRate,
	"percentage":     sortRate,
	"pct":            sortRate,
	"percent":        sortRate,
}

// comparePercentage compares two percentages (numerator/denominator)
func comparePercentage(num1, denom1, num2, denom2 int) bool {
	pct1 := float64(0)
	if denom1 > 0 {
		pct1 = float64(num1) / float64(denom1)
	}
	pct2 := float64(0)
	if denom2 > 0 {
		pct2 = float64(num2) / float64(denom2)
	}
	return pct1 > pct2
}

func sortReviewers(sorted []reviewerKV, sortBy string) error {
	validSortOptions := []sortOption{sortRate, sortRequested, sortCompleted, sortPending, sortResponse, sortApproved, sortCommented, sortChanges}

	var normalizedSortBy sortOption
	if alias, ok := sortAliases[strings.ToLower(sortBy)]; ok {
		normalizedSortBy = alias
	} else {
		normalizedSortBy = sortOption(strings.ToLower(sortBy))
	}

	if !slices.Contains(validSortOptions, normalizedSortBy) {
		validStrings := make([]string, len(validSortOptions))
		for i, opt := range validSortOptions {
			validStrings[i] = string(opt)
		}
		return fmt.Errorf("invalid sort option '%s'. Valid options: %s", sortBy, strings.Join(validStrings, ", "))
	}

	sort.Slice(sorted, func(i, j int) bool {
		si, sj := sorted[i].Value, sorted[j].Value

		switch normalizedSortBy {
		case sortRequested:
			return si.Requested > sj.Requested
		case sortCompleted:
			return si.Completed > sj.Completed
		case sortPending:
			return si.Pending > sj.Pending
		case sortResponse:
			// Sort by average response time (lower is better)
			avgI := time.Duration(0)
			if si.ResponseCount > 0 {
				avgI = si.TotalResponse / time.Duration(si.ResponseCount)
			}
			avgJ := time.Duration(0)
			if sj.ResponseCount > 0 {
				avgJ = sj.TotalResponse / time.Duration(sj.ResponseCount)
			}
			// Handle cases where one or both have no response time
			if si.ResponseCount == 0 && sj.ResponseCount == 0 {
				return false
			}
			if si.ResponseCount == 0 {
				return false
			}
			if sj.ResponseCount == 0 {
				return true
			}
			return avgI < avgJ
		case sortApproved:
			return comparePercentage(si.Approved, si.Completed, sj.Approved, sj.Completed)
		case sortCommented:
			return comparePercentage(si.Commented, si.Completed, sj.Commented, sj.Completed)
		case sortChanges:
			return comparePercentage(si.ChangesRequested, si.Completed, sj.ChangesRequested, sj.Completed)
		case sortRate:
			return comparePercentage(si.Completed, si.Requested, sj.Completed, sj.Requested)
		}
		return false
	})

	return nil
}

func printStats(stats map[string]*ReviewerStats, sortBy string) {
	fmt.Println()
	fmt.Println("📊 REVIEWER STATISTICS")
	fmt.Println()

	// Calculate total reviews for filtering
	totalReviews := 0
	for _, s := range stats {
		totalReviews += s.Requested
	}

	// Calculate minimum threshold
	minThreshold := max(int(float64(totalReviews)*(*minReviewsPercent/100.0)), 1)

	var sorted []reviewerKV
	var filtered []string
	for k, v := range stats {
		// Filter out reviewers below threshold
		if v.Requested < minThreshold {
			filtered = append(filtered, k)
			continue
		}
		sorted = append(sorted, reviewerKV{k, v})
	}

	if len(filtered) > 0 {
		sort.Strings(filtered)
		fmt.Printf("(Filtered out %d reviewers with < %d reviews, %.2f%% of total: %s)\n\n",
			len(filtered), minThreshold, *minReviewsPercent, strings.Join(filtered, ", "))
	}

	if err := sortReviewers(sorted, sortBy); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Reviewer\tRequested\tCompleted\tPending\tRate\tAvg Resp\tApproved\tCommented\tChanges Req")
	fmt.Fprintln(w, "--------\t---------\t---------\t-------\t----\t--------\t--------\t---------\t-----------")

	for _, item := range sorted {
		s := item.Value
		rate := float64(0)
		if s.Requested > 0 {
			rate = float64(s.Completed) / float64(s.Requested) * 100
		}

		avgResp := "N/A"
		if s.ResponseCount > 0 {
			avg := s.TotalResponse / time.Duration(s.ResponseCount)
			avgResp = formatDuration(avg)
		}

		// Calculate percentages for review types
		approvedPct := float64(0)
		commentedPct := float64(0)
		changesPct := float64(0)
		if s.Completed > 0 {
			approvedPct = float64(s.Approved) / float64(s.Completed) * 100
			commentedPct = float64(s.Commented) / float64(s.Completed) * 100
			changesPct = float64(s.ChangesRequested) / float64(s.Completed) * 100
		}

		fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%.1f%%\t%s\t%.1f%%\t%.1f%%\t%.1f%%\n",
			item.Key, s.Requested, s.Completed, s.Pending,
			rate, avgResp, approvedPct, commentedPct, changesPct)
	}
	w.Flush()
}

func printPendingDetails(details []PendingReviewDetail) {
	if len(details) == 0 {
		return
	}

	fmt.Println()
	fmt.Println("⏳ CURRENTLY PENDING REVIEWS (Open PRs)")
	fmt.Println()

	seen := make(map[int]bool)
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PR #\tAge\tTitle")
	fmt.Fprintln(w, "----\t---\t-----")

	for _, d := range details {
		if seen[d.PRNumber] {
			continue
		}
		seen[d.PRNumber] = true
		fmt.Fprintf(w, "#%d\t%s\t%s\n",
			d.PRNumber, formatDuration(d.PRAge), d.PRTitle)
	}
	w.Flush()
}

func printSummary(stats map[string]*ReviewerStats) {
	totalRequested := 0
	totalCompleted := 0
	totalPending := 0

	for _, s := range stats {
		totalRequested += s.Requested
		totalCompleted += s.Completed
		totalPending += s.Pending
	}

	fmt.Println()
	fmt.Println("📈 SUMMARY")
	fmt.Println()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.Debug)
	fmt.Fprintf(w, "Total unique reviewers:\t%d\n", len(stats))
	fmt.Fprintf(w, "Total review requests:\t%d\n", totalRequested)
	fmt.Fprintf(w, "Total completed reviews:\t%d\n", totalCompleted)
	fmt.Fprintf(w, "Total pending reviews:\t%d\n", totalPending)
	if totalRequested > 0 {
		rate := float64(totalCompleted) / float64(totalRequested) * 100
		fmt.Fprintf(w, "Overall completion rate:\t%.1f%%\n", rate)
	}
	w.Flush()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func formatDuration(d time.Duration) string {
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%.1fh", d.Hours())
	}
	days := d.Hours() / 24
	if days < 30 {
		return fmt.Sprintf("%.1fd", days)
	}
	return fmt.Sprintf("%.1fmo", days/30)
}
