package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"
)

func printStats(stats map[string]*ReviewerStats) {
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

	type kv struct {
		Key   string
		Value *ReviewerStats
	}
	var sorted []kv
	var filtered []string
	for k, v := range stats {
		// Filter out reviewers below threshold
		if v.Requested < minThreshold {
			filtered = append(filtered, k)
			continue
		}
		sorted = append(sorted, kv{k, v})
	}

	if len(filtered) > 0 {
		sort.Strings(filtered)
		fmt.Printf("(Filtered out %d reviewers with < %d reviews, %.2f%% of total: %s)\n\n",
			len(filtered), minThreshold, *minReviewsPercent, strings.Join(filtered, ", "))
	}

	sort.Slice(sorted, func(i, j int) bool {
		rateI := float64(0)
		if sorted[i].Value.Requested > 0 {
			rateI = float64(sorted[i].Value.Completed) / float64(sorted[i].Value.Requested)
		}
		rateJ := float64(0)
		if sorted[j].Value.Requested > 0 {
			rateJ = float64(sorted[j].Value.Completed) / float64(sorted[j].Value.Requested)
		}
		return rateI > rateJ
	})

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
