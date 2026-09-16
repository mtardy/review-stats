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

func cacheStatusIcon(trusted, fromCache bool) string {
	if trusted && fromCache {
		return "📁" // trusted cache (closed PR, used local data)
	}
	if fromCache {
		return "🏷 " // ETag validated (304) - note: space added for alignment
	}
	return "🌐" // fresh fetch from network
}

// githubGet makes an HTTP GET request to the GitHub API
// SECURITY: Only makes GET requests, never modifies data
// Uses GITHUB_TOKEN env var for authentication (read-only)
// Uses ETags for efficient cache validation
// trustCache: if true, return cached data without validation (useful for immutable resources)
func githubGet(url string, trustCache bool) (data []byte, fromCache bool, err error) {
	var cachedData []byte
	var cachedETag string

	// Try to load from cache first
	var found bool
	cachedData, cachedETag, found = loadFromCache(url)
	if found {
		// If trustCache is true, return cached data immediately without validation
		if trustCache {
			return cachedData, true, nil
		}
		// We have cached data, but we'll validate it with a conditional
		// request using the ETag if we have one. Note this won't count
		// on the rate limit if the response is 304 and request is
		// authenticated.
		if cachedETag == "" {
			// No ETag stored, just return cached data
			return cachedData, true, nil
		}
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, false, err
	}

	token := os.Getenv("GITHUB_TOKEN")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	// Add If-None-Match header for conditional request if we have a cached ETag
	if cachedETag != "" {
		req.Header.Set("If-None-Match", cachedETag)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()

	// Handle 304 Not Modified - cached data is still valid (ETag matched)
	if resp.StatusCode == http.StatusNotModified {
		if cachedData != nil {
			return cachedData, true, nil
		}
		// This shouldn't happen, but if it does, fall through to error
		return nil, false, fmt.Errorf("received 304 but no cached data available")
	}

	// Handle rate limiting
	if resp.StatusCode == 403 || resp.StatusCode == 429 {
		resetHeader := resp.Header.Get("X-RateLimit-Reset")
		fmt.Println("⚠️  Rate limited. Consider setting GITHUB_TOKEN env variable.")
		if resetHeader != "" {
			fmt.Printf("   Rate limit resets at: %s\n", resetHeader)
		}
		return nil, false, fmt.Errorf("rate limited (HTTP %d)", resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("unexpected status: %d for %s", resp.StatusCode, url)
	}

	data, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, err
	}

	// Save to cache with ETag
	etag := resp.Header.Get("ETag")
	if err := saveToCache(url, data, etag); err != nil {
		log.Printf("Warning: failed to cache response: %v", err)
	}

	return data, false, nil
}

func fetchPullRequests(state string, cutoffDate time.Time) ([]PullRequest, error) {
	var allPRs []PullRequest
	page := 1

	for {
		url := fmt.Sprintf(
			"%s/repos/%s/%s/pulls?state=%s&per_page=%d&page=%d&sort=created&direction=desc",
			githubAPIBase, *owner, *repo, state, perPage, page,
		)

		// Always validate PR lists with ETag (they change frequently)
		body, fromCache, err := githubGet(url, false)
		if err != nil {
			return allPRs, err
		}

		fmt.Printf("  %s Fetching %s PRs — page %d...\n", cacheStatusIcon(false, fromCache), state, page)

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

func fetchRequestedReviewers(prNumber int, prState string) ([]string, bool, bool, error) {
	url := fmt.Sprintf(
		"%s/repos/%s/%s/pulls/%d/requested_reviewers",
		githubAPIBase, *owner, *repo, prNumber,
	)

	// Trust cache for closed PRs
	trustCache := prState == "closed"
	body, fromCache, err := githubGet(url, trustCache)
	if err != nil {
		return nil, false, false, err
	}

	var rr RequestedReviewers
	if err := json.Unmarshal(body, &rr); err != nil {
		return nil, false, false, err
	}

	var reviewers []string
	for _, u := range rr.Users {
		reviewers = append(reviewers, u.Login)
	}
	for _, t := range rr.Teams {
		reviewers = append(reviewers, "team:"+t.Slug)
	}
	return reviewers, trustCache, fromCache, nil
}

func fetchReviews(prNumber int, prState string) ([]Review, bool, bool, error) {
	url := fmt.Sprintf(
		"%s/repos/%s/%s/pulls/%d/reviews",
		githubAPIBase, *owner, *repo, prNumber,
	)

	// Trust cache for closed PRs
	trustCache := prState == "closed"
	body, fromCache, err := githubGet(url, trustCache)
	if err != nil {
		return nil, false, false, err
	}

	var reviews []Review
	if err := json.Unmarshal(body, &reviews); err != nil {
		return nil, false, false, err
	}
	return reviews, trustCache, fromCache, nil
}
