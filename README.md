# GitHub Review Stats

A command-line tool to analyze GitHub pull request review statistics for a repository.

- Track review requests and completion rates for all reviewers
- Calculate average response times and break down review types (approved,
commented, changes requested)
- Cache GitHub API responses for faster re-runs
- Optionally exclude bot-authored PRs from analysis

> [!WARNING]
> This tool was almost completely generated using Claude Sonnet 4.5. The GitHub
> interaction can be mostly reviewed in `github.go::githubGet`, which is a
> simple HTTP get Go wrapper using standard library to call
> `https://api.github.com/` endpoints.
>
> As a general advice, take a look at what any tool could be doing with your
> GitHub token before executing them. This tool should be easy to audit.

> [!CAUTION]
> This tool is only gathering publicly available information for research and
> re-organizing GitHub teams. Implementing an incentive designed to encourage
> contributors to complete their reviews as quickly as possible could
> compromise the quality of the reviews and create an  unhealthy work
> environment.

## Usage

Build or directly run with Go, preferably use your GITHUB_TOKEN for
[higher rate limit](https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api),
but not strictly needed for checking stats on public repos.

```bash
export GITHUB_TOKEN=$(gh auth token)
go run . -cache .my-repo-cache -owner myorg -repo myrepo -days 365
```

### Flags

| Flag                    | Default              | Description                                                           |
| ------                  | ---------            | -------------                                                         |
| `--owner`               | ``                   | GitHub repository owner (required)                                    |
| `--repo`                | ``                   | GitHub repository name (required)                                     |
| `--days`                | `365`                | Number of days to look back for PRs                                   |
| `--workers`             | `50`                 | Number of parallel workers for processing                             |
| `--cache`               | `review-stats-cache` | Directory to cache GitHub API responses                               |
| `--min-reviews-percent` | `0.5`                | Minimum percentage of total reviews to include a reviewer             |
| `--exclude-authors`     | ``                   | Comma-separated list of PR author usernames to exclude (e.g for bots) |

### Cache

API responses are automatically cached in `review-stats-cache/` directory.
Subsequent runs will use cached data, making analysis instant and avoiding
GitHub API rate limits.

The cache entries are compressed using gzip to save space. Use `zcat` (or
`gzcat` on macOS) to view them:

```bash
zcat review-stats-cache/<hash>.json.gz
```

To refresh data, simply delete the cache directory:

```bash
rm -rf review-stats-cache
```
