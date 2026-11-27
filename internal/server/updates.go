package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/shenjiecode/keyfolio/internal/buildinfo"
)

const releaseAPI = "https://api.github.com/repos/shenjiecode/foliochest/releases/latest"
const releasesPage = "https://github.com/shenjiecode/foliochest/releases/latest"

var releaseVersion = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-(dev|alpha\.(?:0|[1-9][0-9]*)|beta\.(?:0|[1-9][0-9]*)|rc\.(?:0|[1-9][0-9]*)))?$`)

type updateResult struct {
	Latest string `json:"latest"`
	Status string `json:"status"`
	URL    string `json:"url"`
}

// Compare only our documented release versions; unknown builds are never called current.
func updateStatus(current, latest string) string {
	a, b := releaseVersion.FindStringSubmatch(current), releaseVersion.FindStringSubmatch(latest)
	if a == nil || b == nil {
		return "unknown"
	}
	for i := 1; i <= 3; i++ {
		x, ex := strconv.ParseUint(a[i], 10, 64)
		y, ey := strconv.ParseUint(b[i], 10, 64)
		if ex != nil || ey != nil {
			return "unknown"
		}
		if x < y {
			return "available"
		}
		if x > y {
			return "ahead"
		}
	}
	if a[4] != "" {
		return "available"
	}
	return "current"
}

func latestRelease(ctx context.Context, client *http.Client, current string) (updateResult, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", releaseAPI, nil)
	if err != nil {
		return updateResult{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "FolioChest-update-check")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	// No cookies, credentials, version, vault identifiers or contents leave the device.
	res, err := client.Do(req)
	if err != nil {
		return updateResult{}, errors.New("无法连接 GitHub，请检查网络或直接打开下载页面")
	}
	defer res.Body.Close()
	if res.StatusCode == 404 {
		return updateResult{}, errors.New("GitHub 暂无可用的正式版本")
	}
	if res.StatusCode == 403 || res.StatusCode == 429 {
		return updateResult{}, errors.New("GitHub 请求受限，请稍后重试或直接打开下载页面")
	}
	if res.StatusCode != 200 {
		return updateResult{}, errors.New("GitHub 检查失败，请稍后重试或直接打开下载页面")
	}
	const limit = 1 << 20
	raw, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil || len(raw) > limit {
		return updateResult{}, errors.New("GitHub 响应不完整或过大")
	}
	var release struct {
		Tag        string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	if json.Unmarshal(raw, &release) != nil {
		return updateResult{}, errors.New("GitHub 返回了无效的版本信息")
	}
	version := releaseVersion.FindStringSubmatch(release.Tag)
	if version == nil || version[4] != "" || release.Draft || release.Prerelease {
		return updateResult{}, errors.New("GitHub 返回的不是有效正式版本")
	}
	return updateResult{Latest: strings.TrimPrefix(release.Tag, "v"), Status: updateStatus(current, release.Tag), URL: "https://github.com/shenjiecode/foliochest/releases/tag/" + release.Tag}, nil
}

func (s *Server) checkUpdate(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, "POST") {
		return
	}
	var in struct {
		Confirmed bool `json:"confirmed"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !in.Confirmed {
		apiError(w, errors.New("请确认主动连接 GitHub 检查更新"), 400)
		return
	}
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	result, err := latestRelease(r.Context(), client, buildinfo.Version)
	if err != nil {
		apiError(w, err, 502)
		return
	}
	writeJSON(w, result)
}
