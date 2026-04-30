package main

import "time"

// GitHub API types
type PullRequest struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	HTMLURL   string    `json:"html_url"`
	State     string    `json:"state"`
	CreatedAt time.Time `json:"created_at"`
	User      User      `json:"user"`
}

type User struct {
	Login string `json:"login"`
}

type Review struct {
	User        User      `json:"user"`
	State       string    `json:"state"`
	SubmittedAt time.Time `json:"submitted_at"`
}

type RequestedReviewers struct {
	Users []User `json:"users"`
	Teams []Team `json:"teams"`
}

type Team struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// Statistics types
type ReviewerStats struct {
	Requested        int
	Completed        int
	Pending          int
	Approved         int
	Commented        int
	ChangesRequested int
	TotalResponse    time.Duration
	ResponseCount    int
}

type PendingReviewDetail struct {
	PRNumber int
	PRTitle  string
	PRURL    string
	PRAge    time.Duration
}

type PRResult struct {
	PR                 PullRequest
	CompletedReviewers map[string]ReviewDetail
	PendingReviewers   []string
	Err                error
}

type ReviewDetail struct {
	SubmittedAt time.Time
	State       string
}
