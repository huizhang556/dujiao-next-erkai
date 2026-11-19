package version

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	urlpkg "net/url"
	"regexp"
	"strings"
	"time"
)

const (
	dockerHubRepository = "litehub/dujiao-next-erkai"
	dockerHubAPIURL     = "https://hub.docker.com/v2/repositories/litehub/dujiao-next-erkai/tags?page_size=100"
	releaseURL          = "https://github.com/huizhang556/dujiao-next-erkai/releases"
	releaseUserAgent    = "dujiao-next-docker-update-checker"
)

type dockerHubTagsPayload struct {
	Next    string             `json:"next"`
	Results []dockerHubTagItem `json:"results"`
}

type dockerHubTagItem struct {
	Name        string    `json:"name"`
	LastUpdated time.Time `json:"last_updated"`
}

// CheckResult 检测结果，已包含当前与最新版本以及是否需要更新
type CheckResult struct {
	CurrentVersion string     `json:"current_version"`
	LatestVersion  string     `json:"latest_version"`
	HasUpdate      bool       `json:"has_update"`
	ReleaseURL     string     `json:"release_url,omitempty"`
	ReleaseName    string     `json:"release_name,omitempty"`
	ReleaseNotes   string     `json:"release_notes,omitempty"`
	PublishedAt    *time.Time `json:"published_at,omitempty"`
	Source         string     `json:"source"`
}

// ErrRateLimited 触发 Docker Hub 调用限流时返回，便于上层映射成专用提示。
var ErrRateLimited = errors.New("docker hub api rate limit exceeded")

// CheckLatestRelease 查询 Docker Hub 上本项目的具体版本标签并与当前版本比较。
func CheckLatestRelease(ctx context.Context) (*CheckResult, error) {
	tags, err := FetchDockerHubTags(ctx)
	if err != nil {
		return nil, err
	}

	current := strings.TrimSpace(Version)
	latestTag, found := latestDockerHubTag(tags)
	if !found {
		return nil, fmt.Errorf("no valid version tags found for %s", dockerHubRepository)
	}

	latest := strings.TrimSpace(latestTag.Name)
	hasUpdate, compareErr := IsNewerVersion(latest, current)
	if compareErr != nil {
		return nil, fmt.Errorf("compare latest version %q with current %q: %w", latest, current, compareErr)
	}

	result := &CheckResult{
		CurrentVersion: current,
		LatestVersion:  latest,
		HasUpdate:      hasUpdate,
		ReleaseURL:     releaseURL,
		ReleaseName:    latest,
		Source:         dockerHubRepository,
	}
	if !latestTag.LastUpdated.IsZero() {
		t := latestTag.LastUpdated
		result.PublishedAt = &t
	}
	return result, nil
}

func latestDockerHubTag(tags []dockerHubTagItem) (dockerHubTagItem, bool) {
	var latestTag dockerHubTagItem
	var latestSemver semver
	found := false
	for _, tag := range tags {
		name := strings.TrimSpace(tag.Name)
		if name == "" || name == "latest" || name == "dev" {
			continue
		}
		parsed, parseErr := parseSemver(name)
		if parseErr != nil {
			continue
		}
		if !found || compareSemver(parsed, latestSemver) > 0 {
			latestTag = tag
			latestSemver = parsed
			found = true
		}
	}
	return latestTag, found
}

// FetchDockerHubTags 拉取 Docker Hub 标签，支持 API 分页。
func FetchDockerHubTags(ctx context.Context) ([]dockerHubTagItem, error) {
	url := dockerHubAPIURL
	var tags []dockerHubTagItem
	client := &http.Client{Timeout: 10 * time.Second}
	for page := 0; url != "" && page < 10; page++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("User-Agent", releaseUserAgent)
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("request docker hub api: %w", err)
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read response: %w", readErr)
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			return nil, ErrRateLimited
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("docker hub api returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		var payload dockerHubTagsPayload
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, fmt.Errorf("decode docker hub tags: %w", err)
		}
		tags = append(tags, payload.Results...)
		url = payload.Next
		if url != "" {
			nextURL, parseErr := urlpkg.Parse(url)
			if parseErr != nil || nextURL.Scheme != "https" || nextURL.Host != "hub.docker.com" {
				return nil, errors.New("docker hub api returned an invalid pagination URL")
			}
		}
	}
	return tags, nil
}

// semver 一个解析后的版本号。prerelease 为空表示正式版。
type semver struct {
	// core 也用十进制字符串保存：SemVer 对数字位数没有上限，major/minor/patch
	// 与预发布数字标识符一样不能依赖本机 int 宽度。
	core [3]string
	// prerelease 预发布标识按 "." 拆分后的各段，例如 "rc.1" -> ["rc", "1"]
	prerelease []string
}

// IsNewerVersion 判断 latest 是否比 current 更新。返回 (true, nil) 表示需要更新；
// 当任一版本号无法解析时 fail-closed：返回 false 和错误。这个结果会直接决定
// 是否允许自动替换二进制，不能把“字符串不同”猜成“版本更高”。
func IsNewerVersion(latest, current string) (bool, error) {
	l, lErr := parseSemver(latest)
	c, cErr := parseSemver(current)
	if lErr != nil || cErr != nil {
		return false, errors.Join(lErr, cErr)
	}

	return compareSemver(l, c) > 0, nil
}

// compareSemver 按 SemVer 2.0.0 的优先级规则比较，返回 -1 / 0 / 1。
func compareSemver(a, b semver) int {
	for i := range 3 {
		if c := compareNumericIdentifiers(a.core[i], b.core[i]); c != 0 {
			return c
		}
	}
	return comparePrerelease(a.prerelease, b.prerelease)
}

// comparePrerelease 比较预发布段的优先级（SemVer 2.0.0 §11.3、§11.4）。
//
// 核心规则：有预发布段的版本优先级**低于**同样核心版本号的正式版。
// 这正是之前直接丢弃预发布信息会出问题的地方 —— v1.2.3-rc.1 和 v1.2.3 都被解析成
// [1,2,3]，于是 RC 用户永远收不到同版本号正式版的升级提示。
//
// 段内比较：纯数字段按数值比，其余按 ASCII 字典序比，数字段优先级低于非数字段；
// 前面各段都相等时，段数多的一方优先级更高（rc.1.1 > rc.1）。
func comparePrerelease(a, b []string) int {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	// 正式版 > 预发布版
	if len(a) == 0 {
		return 1
	}
	if len(b) == 0 {
		return -1
	}

	for i := 0; i < len(a) && i < len(b); i++ {
		aNum := isNumericIdentifier(a[i])
		bNum := isNumericIdentifier(b[i])
		switch {
		case aNum && bNum:
			if c := compareNumericIdentifiers(a[i], b[i]); c != 0 {
				return c
			}
		case aNum != bNum:
			// 数字段优先级低于非数字段
			if aNum {
				return -1
			}
			return 1
		default:
			if c := strings.Compare(a[i], b[i]); c != 0 {
				return c
			}
		}
	}

	// 前缀相同时段数多的更大：rc.1.1 > rc.1
	switch {
	case len(a) > len(b):
		return 1
	case len(a) < len(b):
		return -1
	default:
		return 0
	}
}

// isNumericIdentifier 判断预发布段是否为纯数字标识符
func isNumericIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// compareNumericIdentifiers 按数值比较两个纯数字标识符，返回 -1 / 0 / 1。
//
// 不走 strconv.Atoi：SemVer 对数字标识符的位数没有上限，超出 int 范围时 Atoi 会报错，
// 于是两个合法的超大数字会被降级成字符串比较，给出错误的顺序。
// 去掉前导零之后先比长度再比字典序，对任意长度都成立。
func compareNumericIdentifiers(a, b string) int {
	a = strings.TrimLeft(a, "0")
	b = strings.TrimLeft(b, "0")
	if len(a) != len(b) {
		if len(a) > len(b) {
			return 1
		}
		return -1
	}
	return strings.Compare(a, b)
}

// semverIdentifier 是预发布和构建元数据标识符共同允许的字符集：
// ASCII 字母、数字与连字符。
var semverIdentifier = regexp.MustCompile(`^[0-9A-Za-z-]+$`)

// parseSemver 将 "v1.2.3" / "1.2.3" / "v1.2.3-rc.1" / "v1.2.3-rc.1+build.5" 等格式
// 解析为核心三段加预发布段。构建元数据（+ 之后）按 SemVer 规定不参与优先级比较，直接丢弃。
//
// 严格要求核心版本恰好是 X.Y.Z 三段：宽松接受 "1.2" 或 "1.2.3.4" 会把一个明显畸形的
// 标签悄悄解析成某个版本号，然后据此判断要不要自动替换二进制 —— 这种地方宁可报错，
// 让调用方拒绝自动升级，也不要猜。
func parseSemver(v string) (semver, error) {
	var out semver
	s := strings.TrimSpace(v)
	if s == "" {
		return out, errors.New("empty version")
	}
	s = strings.TrimPrefix(s, "v")
	s = strings.TrimPrefix(s, "V")

	// 构建元数据不参与优先级比较，但仍必须验证格式。直接把 "+" 后内容
	// 丢掉会把 "1.2.3+"、"1.2.3+bad_ id" 这类畸形发布标签当成合法版本。
	if i := strings.IndexByte(s, '+'); i >= 0 {
		build := s[i+1:]
		s = s[:i]
		if build == "" {
			return out, fmt.Errorf("empty build metadata in %s", v)
		}
		for _, id := range strings.Split(build, ".") {
			if id == "" {
				return out, fmt.Errorf("empty build metadata identifier in %s", v)
			}
			if !semverIdentifier.MatchString(id) {
				return out, fmt.Errorf("invalid build metadata identifier %q in %s", id, v)
			}
		}
	}
	if i := strings.IndexByte(s, '-'); i >= 0 {
		pre := s[i+1:]
		s = s[:i]
		// "1.2.3-" 不是正式版，而是一个畸形标签：SemVer 要求预发布段非空
		if pre == "" {
			return out, fmt.Errorf("empty prerelease in %s", v)
		}
		out.prerelease = strings.Split(pre, ".")
		for _, id := range out.prerelease {
			if id == "" {
				return out, fmt.Errorf("empty prerelease identifier in %s", v)
			}
			if !semverIdentifier.MatchString(id) {
				return out, fmt.Errorf("invalid prerelease identifier %q in %s", id, v)
			}
			// SemVer 2.0.0 §9：纯数字预发布标识符不能有前导零。
			if isNumericIdentifier(id) && len(id) > 1 && id[0] == '0' {
				return out, fmt.Errorf("numeric prerelease identifier %q has leading zeros in %s", id, v)
			}
		}
	}

	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return out, fmt.Errorf("core version must have exactly 3 segments, got %d in %s", len(parts), v)
	}
	for i, part := range parts {
		if !isNumericIdentifier(part) {
			return out, fmt.Errorf("invalid version segment %q in %s", part, v)
		}
		// SemVer 2.0.0 §2：major/minor/patch 不能有前导零。
		if len(part) > 1 && part[0] == '0' {
			return out, fmt.Errorf("version segment %q has leading zeros in %s", part, v)
		}
		out.core[i] = part
	}
	return out, nil
}
