package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	BaseURL      string
	APIUser      string
	APIKey       string
	OutputPath   string
	StatePath    string
	DryRun       bool
	Queries      []string
	MaxPages     int
	MaxRequests  int
	MinInterval  time.Duration
	Jitter       time.Duration
	HourlyCap    int
	DailyCap     int
	Timeout      time.Duration
	MaxRetries   int
	Backoff429   time.Duration
	BackoffError time.Duration
	MaxBackoff   time.Duration
}

type state struct {
	Version         int             `json:"version"`
	DayBucket       string          `json:"day_bucket"`
	DayCount        int             `json:"day_count"`
	HourBucket      string          `json:"hour_bucket"`
	HourCount       int             `json:"hour_count"`
	LastRequestUnix int64           `json:"last_request_unix"`
	SeenTorrentIDs  map[string]bool `json:"seen_torrent_ids"`
}

type candidate struct {
	Query        string `json:"query"`
	Page         int    `json:"page"`
	GroupID      int    `json:"group_id,omitempty"`
	TorrentID    int    `json:"torrent_id,omitempty"`
	Title        string `json:"title,omitempty"`
	Year         int    `json:"year,omitempty"`
	Media        string `json:"media,omitempty"`
	Container    string `json:"container,omitempty"`
	Codec        string `json:"codec,omitempty"`
	Resolution   string `json:"resolution,omitempty"`
	Source       string `json:"source,omitempty"`
	ReleaseGroup string `json:"release_group,omitempty"`
	SizeBytes    int64  `json:"size_bytes,omitempty"`
	Snatched     bool   `json:"snatched,omitempty"`
	Permalink    string `json:"permalink,omitempty"`
}

type queryPreset struct {
	Name   string
	Params map[string]string
}

type limiter struct {
	cfg   Config
	state *state
	rng   *rand.Rand
}

func main() {
	cfg := defaultConfig()
	if v := strings.TrimSpace(os.Getenv("PTP_BASE_URL")); v != "" {
		cfg.BaseURL = v
	}
	if v := strings.TrimSpace(os.Getenv("PTP_SCAN_OUTPUT")); v != "" {
		cfg.OutputPath = v
	}
	if v := strings.TrimSpace(os.Getenv("PTP_SCAN_STATE")); v != "" {
		cfg.StatePath = v
	}
	var queriesCSV string
	flag.StringVar(&cfg.BaseURL, "base-url", cfg.BaseURL, "PTP base URL")
	flag.StringVar(&cfg.APIUser, "api-user", "", "PTP ApiUser header (or env PTP_API_USER)")
	flag.StringVar(&cfg.APIKey, "api-key", "", "PTP ApiKey header (or env PTP_API_KEY)")
	flag.StringVar(&cfg.OutputPath, "output", cfg.OutputPath, "JSONL output path for candidate torrents")
	flag.StringVar(&cfg.StatePath, "state", cfg.StatePath, "scanner state path")
	flag.BoolVar(&cfg.DryRun, "dry-run", cfg.DryRun, "print planned requests only")
	flag.IntVar(&cfg.MaxPages, "max-pages", cfg.MaxPages, "max pages per query")
	flag.IntVar(&cfg.MaxRequests, "max-requests", cfg.MaxRequests, "hard cap requests for this run")
	flag.DurationVar(&cfg.MinInterval, "min-interval", cfg.MinInterval, "minimum delay between requests")
	flag.DurationVar(&cfg.Jitter, "jitter", cfg.Jitter, "extra random delay [0,jitter] per request")
	flag.IntVar(&cfg.HourlyCap, "hourly-cap", cfg.HourlyCap, "safety cap requests/hour")
	flag.IntVar(&cfg.DailyCap, "daily-cap", cfg.DailyCap, "safety cap requests/day")
	flag.DurationVar(&cfg.Timeout, "timeout", cfg.Timeout, "HTTP client timeout")
	flag.IntVar(&cfg.MaxRetries, "max-retries", cfg.MaxRetries, "max retries per request")
	flag.DurationVar(&cfg.Backoff429, "backoff-429", cfg.Backoff429, "initial backoff for 429")
	flag.DurationVar(&cfg.BackoffError, "backoff-error", cfg.BackoffError, "initial backoff for 400/500 class")
	flag.DurationVar(&cfg.MaxBackoff, "max-backoff", cfg.MaxBackoff, "max backoff delay")
	flag.StringVar(&queriesCSV, "queries", strings.Join(cfg.Queries, ","), "comma-separated preset names")
	flag.Parse()

	cfg.Queries = parseQueries(queriesCSV)
	if cfg.APIUser == "" {
		cfg.APIUser = strings.TrimSpace(os.Getenv("PTP_API_USER"))
	}
	if cfg.APIKey == "" {
		cfg.APIKey = strings.TrimSpace(os.Getenv("PTP_API_KEY"))
	}
	if err := validateConfig(cfg); err != nil {
		fatalf("config error: %v", err)
	}

	if cfg.DryRun {
		printPlan(cfg)
		return
	}

	st, err := loadState(cfg.StatePath)
	if err != nil {
		fatalf("load state: %v", err)
	}
	lm := &limiter{
		cfg:   cfg,
		state: st,
		rng:   rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	ctx := context.Background()
	client := &http.Client{Timeout: cfg.Timeout}
	presets := defaultPresets()

	out, err := openOutput(cfg.OutputPath)
	if err != nil {
		fatalf("open output: %v", err)
	}
	defer out.Close()

	totalRequests := 0
	totalCandidates := 0
	totalNew := 0

	for _, name := range cfg.Queries {
		preset, ok := presets[name]
		if !ok {
			fatalf("unknown query preset %q", name)
		}
		for page := 1; page <= cfg.MaxPages; page++ {
			if totalRequests >= cfg.MaxRequests {
				fmt.Printf("stop: max-requests reached (%d)\n", cfg.MaxRequests)
				break
			}
			params := cloneParams(preset.Params)
			params["page"] = strconv.Itoa(page)
			body, err := fetchJSON(ctx, client, lm, cfg, params)
			if err != nil {
				fatalf("query=%s page=%d: %v", preset.Name, page, err)
			}
			totalRequests++

			payload := map[string]any{}
			if err := json.Unmarshal(body, &payload); err != nil {
				fatalf("query=%s page=%d: decode json: %v", preset.Name, page, err)
			}

			cand := extractCandidates(payload, cfg.BaseURL, preset.Name, page)
			totalCandidates += len(cand)
			newCount := 0
			for _, c := range cand {
				if !isTargetCandidate(c) {
					continue
				}
				key := candidateKey(c)
				if key != "" && st.SeenTorrentIDs[key] {
					continue
				}
				if key != "" {
					st.SeenTorrentIDs[key] = true
				}
				if err := writeJSONLine(out, c); err != nil {
					fatalf("write output: %v", err)
				}
				newCount++
				totalNew++
			}
			fmt.Printf("query=%s page=%d raw=%d new=%d req_day=%d req_hour=%d\n",
				preset.Name, page, len(cand), newCount, st.DayCount, st.HourCount)

			if err := saveState(cfg.StatePath, st); err != nil {
				fatalf("save state: %v", err)
			}
			if len(cand) == 0 {
				break
			}
		}
	}

	fmt.Printf("done requests=%d raw_candidates=%d new_candidates=%d output=%s\n",
		totalRequests, totalCandidates, totalNew, cfg.OutputPath)
}

func defaultConfig() Config {
	return Config{
		BaseURL:      "https://passthepopcorn.me",
		OutputPath:   ".cache/ptp-scan/candidates.jsonl",
		StatePath:    ".cache/ptp-scan/state.json",
		DryRun:       true,
		Queries:      []string{"bd-m2ts", "file-m2ts", "file-ts"},
		MaxPages:     3,
		MaxRequests:  40,
		MinInterval:  20 * time.Second,
		Jitter:       5 * time.Second,
		HourlyCap:    100,
		DailyCap:     300,
		Timeout:      45 * time.Second,
		MaxRetries:   5,
		Backoff429:   2 * time.Minute,
		BackoffError: 45 * time.Second,
		MaxBackoff:   30 * time.Minute,
	}
}

func defaultPresets() map[string]queryPreset {
	return map[string]queryPreset{
		"bd-m2ts": {
			Name: "bd-m2ts",
			Params: map[string]string{
				"action":        "advanced",
				"json":          "noredirect",
				"grouping":      "0",
				"order_by":      "time",
				"order_way":     "desc",
				"media":         "Blu-ray",
				"encoding":      "m2ts",
				"filter_cat[1]": "1",
			},
		},
		"file-m2ts": {
			Name: "file-m2ts",
			Params: map[string]string{
				"action":        "advanced",
				"json":          "noredirect",
				"grouping":      "0",
				"order_by":      "time",
				"order_way":     "desc",
				"filelist":      ".m2ts",
				"filter_cat[1]": "1",
			},
		},
		"file-ts": {
			Name: "file-ts",
			Params: map[string]string{
				"action":        "advanced",
				"json":          "noredirect",
				"grouping":      "0",
				"order_by":      "time",
				"order_way":     "desc",
				"filelist":      ".ts",
				"filter_cat[1]": "1",
			},
		},
	}
}

func parseQueries(csv string) []string {
	items := strings.Split(csv, ",")
	out := make([]string, 0, len(items))
	for _, it := range items {
		it = strings.TrimSpace(it)
		if it == "" {
			continue
		}
		out = append(out, it)
	}
	return out
}

func validateConfig(cfg Config) error {
	if !cfg.DryRun && (cfg.APIUser == "" || cfg.APIKey == "") {
		return errors.New("missing API credentials (set -api-user/-api-key or PTP_API_USER/PTP_API_KEY)")
	}
	if cfg.MaxPages <= 0 || cfg.MaxRequests <= 0 {
		return errors.New("max-pages and max-requests must be > 0")
	}
	if cfg.MinInterval < 2*time.Second {
		return errors.New("min-interval must be >= 2s")
	}
	if cfg.HourlyCap <= 0 || cfg.DailyCap <= 0 {
		return errors.New("hourly-cap and daily-cap must be > 0")
	}
	if cfg.MaxRetries <= 0 {
		return errors.New("max-retries must be > 0")
	}
	if len(cfg.Queries) == 0 {
		return errors.New("no queries configured")
	}
	if _, err := url.Parse(cfg.BaseURL); err != nil {
		return fmt.Errorf("invalid base-url: %w", err)
	}
	return nil
}

func printPlan(cfg Config) {
	fmt.Println("dry-run: true")
	fmt.Printf("base-url=%s\n", cfg.BaseURL)
	fmt.Printf("queries=%s\n", strings.Join(cfg.Queries, ","))
	fmt.Printf("max-pages=%d max-requests=%d\n", cfg.MaxPages, cfg.MaxRequests)
	fmt.Printf("throttle min-interval=%s jitter<=%s hourly-cap=%d daily-cap=%d\n", cfg.MinInterval, cfg.Jitter, cfg.HourlyCap, cfg.DailyCap)
	fmt.Printf("state=%s output=%s\n", cfg.StatePath, cfg.OutputPath)
	fmt.Println("send requests with: ApiUser + ApiKey headers")
}

func loadState(path string) (*state, error) {
	st := &state{Version: 1, SeenTorrentIDs: map[string]bool{}}
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return st, nil
		}
		return nil, err
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		return st, nil
	}
	if err := json.Unmarshal(b, st); err != nil {
		return nil, err
	}
	if st.SeenTorrentIDs == nil {
		st.SeenTorrentIDs = map[string]bool{}
	}
	if st.Version == 0 {
		st.Version = 1
	}
	return st, nil
}

func saveState(path string, st *state) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func openOutput(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
}

func (l *limiter) beforeRequest(ctx context.Context) error {
	now := time.Now().UTC()
	day := now.Format("2006-01-02")
	hour := now.Format("2006-01-02T15")
	if l.state.DayBucket != day {
		l.state.DayBucket = day
		l.state.DayCount = 0
	}
	if l.state.HourBucket != hour {
		l.state.HourBucket = hour
		l.state.HourCount = 0
	}
	if l.state.DayCount >= l.cfg.DailyCap {
		return fmt.Errorf("daily-cap reached (%d)", l.cfg.DailyCap)
	}
	if l.state.HourCount >= l.cfg.HourlyCap {
		return fmt.Errorf("hourly-cap reached (%d)", l.cfg.HourlyCap)
	}

	if l.state.LastRequestUnix > 0 {
		last := time.Unix(l.state.LastRequestUnix, 0).UTC()
		gap := l.cfg.MinInterval
		if l.cfg.Jitter > 0 {
			gap += time.Duration(l.rng.Int63n(int64(l.cfg.Jitter) + 1))
		}
		wait := gap - now.Sub(last)
		if wait > 0 {
			t := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				t.Stop()
				return ctx.Err()
			case <-t.C:
			}
		}
	}

	l.state.LastRequestUnix = time.Now().UTC().Unix()
	l.state.DayCount++
	l.state.HourCount++
	return nil
}

func fetchJSON(ctx context.Context, client *http.Client, lm *limiter, cfg Config, params map[string]string) ([]byte, error) {
	values := url.Values{}
	for k, v := range params {
		values.Set(k, v)
	}

	for attempt := 1; attempt <= cfg.MaxRetries; attempt++ {
		if err := lm.beforeRequest(ctx); err != nil {
			return nil, err
		}

		u := strings.TrimRight(cfg.BaseURL, "/") + "/torrents.php?" + values.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("ApiUser", cfg.APIUser)
		req.Header.Set("ApiKey", cfg.APIKey)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "go-mediainfo-ptp-scan/1.0")

		resp, err := client.Do(req)
		if err != nil {
			if attempt == cfg.MaxRetries {
				return nil, err
			}
			if err := sleepBackoff(ctx, backoffDuration(cfg.BackoffError, cfg.MaxBackoff, attempt)); err != nil {
				return nil, err
			}
			continue
		}

		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		if readErr != nil {
			if attempt == cfg.MaxRetries {
				return nil, readErr
			}
			if err := sleepBackoff(ctx, backoffDuration(cfg.BackoffError, cfg.MaxBackoff, attempt)); err != nil {
				return nil, err
			}
			continue
		}

		switch resp.StatusCode {
		case http.StatusOK:
			return body, nil
		case http.StatusForbidden:
			return nil, fmt.Errorf("403 forbidden: halted")
		case http.StatusUnauthorized:
			return nil, fmt.Errorf("401 unauthorized: API disabled or invalid")
		case http.StatusTooManyRequests:
			if attempt == cfg.MaxRetries {
				return nil, fmt.Errorf("429 too many requests: %s", strings.TrimSpace(string(body)))
			}
			if err := sleepBackoff(ctx, backoffDuration(cfg.Backoff429, cfg.MaxBackoff, attempt)); err != nil {
				return nil, err
			}
			continue
		case http.StatusBadRequest, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			if attempt == cfg.MaxRetries {
				return nil, fmt.Errorf("status=%d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
			}
			if err := sleepBackoff(ctx, backoffDuration(cfg.BackoffError, cfg.MaxBackoff, attempt)); err != nil {
				return nil, err
			}
			continue
		default:
			return nil, fmt.Errorf("status=%d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
	}

	return nil, errors.New("unreachable")
}

func backoffDuration(base, max time.Duration, attempt int) time.Duration {
	if attempt <= 1 {
		if base > max {
			return max
		}
		return base
	}
	d := base
	for i := 1; i < attempt; i++ {
		d *= 2
		if d >= max {
			return max
		}
	}
	if d > max {
		return max
	}
	return d
}

func sleepBackoff(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func cloneParams(src map[string]string) map[string]string {
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func extractCandidates(payload map[string]any, baseURL, query string, page int) []candidate {
	movies := pickObjectArray(payload, "Movies", "movies", "Results", "results")
	out := make([]candidate, 0, len(movies)*3)
	for _, movie := range movies {
		groupID := pickInt(movie, "GroupId", "GroupID", "groupid", "group_id", "Id", "id")
		title := pickString(movie, "Title", "GroupName", "name", "title")
		year := pickInt(movie, "Year", "year")
		torrents := pickObjectArray(movie, "Torrents", "torrents")
		if len(torrents) == 0 {
			if hasTorrentishKeys(movie) {
				torrents = []map[string]any{movie}
			}
		}
		for _, tr := range torrents {
			c := candidate{
				Query:        query,
				Page:         page,
				GroupID:      groupID,
				Title:        title,
				Year:         year,
				TorrentID:    pickInt(tr, "TorrentId", "torrentid", "TorrentID", "torrent_id", "Id", "id"),
				Media:        pickString(tr, "Media", "media", "Source", "source"),
				Container:    pickString(tr, "Container", "container", "Encoding", "encoding"),
				Codec:        pickString(tr, "Codec", "codec", "Format", "format"),
				Resolution:   pickString(tr, "Resolution", "resolution"),
				Source:       pickString(tr, "Source", "source"),
				ReleaseGroup: pickString(tr, "ReleaseGroup", "releasegroup", "release_group", "Scene", "scene"),
				SizeBytes:    pickInt64(tr, "Size", "size", "SizeBytes", "size_bytes"),
				Snatched:     pickBool(tr, "Snatched", "snatched"),
			}
			if c.GroupID > 0 {
				c.Permalink = fmt.Sprintf("%s/torrents.php?id=%d", strings.TrimRight(baseURL, "/"), c.GroupID)
				if c.TorrentID > 0 {
					c.Permalink += "&torrentid=" + strconv.Itoa(c.TorrentID)
				}
			}
			out = append(out, c)
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].TorrentID != out[j].TorrentID {
			return out[i].TorrentID < out[j].TorrentID
		}
		if out[i].GroupID != out[j].GroupID {
			return out[i].GroupID < out[j].GroupID
		}
		return out[i].Title < out[j].Title
	})
	return out
}

func hasTorrentishKeys(m map[string]any) bool {
	keys := []string{"TorrentId", "torrentid", "Container", "Encoding", "Codec", "Resolution"}
	for _, k := range keys {
		if _, ok := m[k]; ok {
			return true
		}
	}
	return false
}

func pickObjectArray(m map[string]any, keys ...string) []map[string]any {
	for _, k := range keys {
		v, ok := m[k]
		if !ok {
			continue
		}
		slice, ok := v.([]any)
		if !ok {
			continue
		}
		out := make([]map[string]any, 0, len(slice))
		for _, item := range slice {
			obj, ok := item.(map[string]any)
			if ok {
				out = append(out, obj)
			}
		}
		return out
	}
	return nil
}

func pickString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch x := v.(type) {
			case string:
				if strings.TrimSpace(x) != "" {
					return strings.TrimSpace(x)
				}
			case float64:
				if x != 0 {
					return strconv.FormatFloat(x, 'f', -1, 64)
				}
			}
		}
	}
	return ""
}

func pickInt(m map[string]any, keys ...string) int {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch x := v.(type) {
			case float64:
				return int(x)
			case int:
				return x
			case int64:
				return int(x)
			case string:
				n, err := strconv.Atoi(strings.TrimSpace(x))
				if err == nil {
					return n
				}
			}
		}
	}
	return 0
}

func pickInt64(m map[string]any, keys ...string) int64 {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch x := v.(type) {
			case float64:
				return int64(x)
			case int64:
				return x
			case int:
				return int64(x)
			case string:
				n, err := strconv.ParseInt(strings.TrimSpace(x), 10, 64)
				if err == nil {
					return n
				}
			}
		}
	}
	return 0
}

func pickBool(m map[string]any, keys ...string) bool {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch x := v.(type) {
			case bool:
				return x
			case string:
				x = strings.ToLower(strings.TrimSpace(x))
				return x == "1" || x == "true" || x == "yes"
			case float64:
				return x != 0
			}
		}
	}
	return false
}

func isTargetCandidate(c candidate) bool {
	hay := strings.ToLower(strings.Join([]string{c.Media, c.Source, c.Container, c.Codec, c.Title, c.Permalink}, " "))
	if strings.Contains(hay, ".m2ts") || strings.Contains(hay, " m2ts") {
		return true
	}
	if strings.Contains(hay, ".ts") || strings.Contains(hay, " ts ") || strings.HasSuffix(hay, " ts") {
		return true
	}
	if strings.Contains(hay, "blu-ray") && (strings.Contains(hay, "bd25") || strings.Contains(hay, "bd50") || strings.Contains(hay, "bd66") || strings.Contains(hay, "bd100")) {
		return true
	}
	if strings.Contains(hay, "blu-ray") && strings.Contains(hay, "m2ts") {
		return true
	}
	return false
}

func candidateKey(c candidate) string {
	if c.TorrentID > 0 {
		return strconv.Itoa(c.TorrentID)
	}
	if c.GroupID > 0 && c.Title != "" {
		return fmt.Sprintf("%d:%s", c.GroupID, strings.ToLower(c.Title))
	}
	return ""
}

func writeJSONLine(w io.Writer, c candidate) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	if _, err := w.Write(b); err != nil {
		return err
	}
	_, err = w.Write([]byte("\n"))
	return err
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
