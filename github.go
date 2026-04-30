package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

const (
	githubAPIBase = "https://api.github.com"
	perPage       = 100 // PRs per page (max 100)
)

var client = &http.Client{Timeout: 30 * time.Second}

// githubGet makes an HTTP GET request to the GitHub API
// SECURITY: Only makes GET requests, never modifies data
// Uses GITHUB_TOKEN env var for authentication (read-only)
func githubGet(url string) ([]byte, error) {
	// Try to load from cache first
	if data, found := loadFromCache(url); found {
		return data, nil
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	token := os.Getenv("GITHUB_TOKEN")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Handle rate limiting
	if resp.StatusCode == 403 || resp.StatusCode == 429 {
		resetHeader := resp.Header.Get("X-RateLimit-Reset")
		fmt.Println("⚠️  Rate limited. Consider setting GITHUB_TOKEN env variable.")
		if resetHeader != "" {
			fmt.Printf("   Rate limit resets at: %s\n", resetHeader)
		}
		return nil, fmt.Errorf("rate limited (HTTP %d)", resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d for %s", resp.StatusCode, url)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Save to cache
	if err := saveToCache(url, data); err != nil {
		log.Printf("Warning: failed to cache response: %v", err)
	}

	return data, nil
}

func fetchPullRequests(state string, cutoffDate time.Time) ([]PullRequest, error) {
	var allPRs []PullRequest
	page := 1

	for {
		url := fmt.Sprintf(
			"%s/repos/%s/%s/pulls?state=%s&per_page=%d&page=%d&sort=updated&direction=desc",
			githubAPIBase, *owner, *repo, state, perPage, page,
		)

		fmt.Printf("  Fetching %s PRs — page %d...\n", state, page)
		body, err := githubGet(url)
		if err != nil {
			return allPRs, err
		}

		var prs []PullRequest
		if err := json.Unmarshal(body, &prs); err != nil {
			return allPRs, fmt.Errorf("JSON parse error: %w", err)
		}

		if len(prs) == 0 {
			break
		}

		// Filter PRs by creation date and add to results
		oldestInPage := true
		for _, pr := range prs {
			if pr.CreatedAt.Before(cutoffDate) {
				continue
			}
			oldestInPage = false
			allPRs = append(allPRs, pr)
		}

		// If all PRs in this page are older than cutoff, stop fetching
		if oldestInPage && len(prs) > 0 {
			fmt.Printf("  Reached PRs older than cutoff date, stopping...\n")
			break
		}

		page++
	}

	return allPRs, nil
}

func fetchRequestedReviewers(prNumber int) ([]string, error) {
	url := fmt.Sprintf(
		"%s/repos/%s/%s/pulls/%d/requested_reviewers",
		githubAPIBase, *owner, *repo, prNumber,
	)

	body, err := githubGet(url)
	if err != nil {
		return nil, err
	}

	var rr RequestedReviewers
	if err := json.Unmarshal(body, &rr); err != nil {
		return nil, err
	}

	var reviewers []string
	for _, u := range rr.Users {
		reviewers = append(reviewers, u.Login)
	}
	for _, t := range rr.Teams {
		reviewers = append(reviewers, "team:"+t.Slug)
	}
	return reviewers, nil
}

func fetchReviews(prNumber int) ([]Review, error) {
	url := fmt.Sprintf(
		"%s/repos/%s/%s/pulls/%d/reviews",
		githubAPIBase, *owner, *repo, prNumber,
	)

	body, err := githubGet(url)
	if err != nil {
		return nil, err
	}

	var reviews []Review
	if err := json.Unmarshal(body, &reviews); err != nil {
		return nil, err
	}
	return reviews, nil
}
