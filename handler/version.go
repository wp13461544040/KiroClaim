package handler

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/huey1in/KiroClaim/utils"

	"github.com/gin-gonic/gin"
)

const (
	versionCheckURL       = "https://api.github.com/repos/" + utils.GitHubRepo + "/releases/latest"
	versionCheckCacheTime = 10 * time.Minute
	autoUpdateInterval    = 30 * time.Minute
)

type versionState struct {
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	HasUpdate      bool   `json:"hasUpdate"`
	ReleaseURL     string `json:"releaseUrl"`
	DockerImage    string `json:"dockerImage"`
	CheckedAt      string `json:"checkedAt"`
	Error          string `json:"error,omitempty"`
}

var (
	versionMu           sync.Mutex
	cachedVersionState  versionState
	cachedVersionAt     time.Time
	updateMu            sync.Mutex
	updateInProgress    bool
	updateLastMessage   string
	updateLastStartedAt time.Time
	updateSchedulerOnce sync.Once
)

func AdminVersion(c *gin.Context) {
	force := c.Query("force") == "1"
	state := getVersionState(force)
	state.AutoUpdateEnabled = GetCurrentSettings().AutoUpdateEnabled
	state.UpdateSupported = dockerUpdateSupported()
	state.UpdateInProgress = isUpdateInProgress()
	state.LastUpdateMessage = getUpdateLastMessage()
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": state})
}

func AdminVersionUpdate(c *gin.Context) {
	state := getVersionState(true)
	if state.Error != "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "版本检查失败: " + state.Error, "data": state})
		return
	}
	if !state.HasUpdate {
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "当前已是最新版本", "data": state})
		return
	}
	message, err := startDockerUpdate("manual")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": err.Error(), "data": state})
		return
	}
	AddOpLogWithCtx(c, "settings", "手动触发 Docker 更新: "+state.CurrentVersion+" -> "+state.LatestVersion, "admin")
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": message, "data": state})
}

type adminVersionPayload struct {
	CurrentVersion    string `json:"currentVersion"`
	LatestVersion     string `json:"latestVersion"`
	HasUpdate         bool   `json:"hasUpdate"`
	ReleaseURL        string `json:"releaseUrl"`
	DockerImage       string `json:"dockerImage"`
	CheckedAt         string `json:"checkedAt"`
	Error             string `json:"error,omitempty"`
	AutoUpdateEnabled bool   `json:"autoUpdateEnabled"`
	UpdateSupported   bool   `json:"updateSupported"`
	UpdateInProgress  bool   `json:"updateInProgress"`
	LastUpdateMessage string `json:"lastUpdateMessage,omitempty"`
}

func (s versionState) toPayload() adminVersionPayload {
	return adminVersionPayload{
		CurrentVersion: s.CurrentVersion,
		LatestVersion:  s.LatestVersion,
		HasUpdate:      s.HasUpdate,
		ReleaseURL:     s.ReleaseURL,
		DockerImage:    s.DockerImage,
		CheckedAt:      s.CheckedAt,
		Error:          s.Error,
	}
}

func getVersionState(force bool) adminVersionPayload {
	versionMu.Lock()
	if !force && time.Since(cachedVersionAt) < versionCheckCacheTime && cachedVersionState.CurrentVersion != "" {
		state := cachedVersionState
		versionMu.Unlock()
		return state.toPayload()
	}
	versionMu.Unlock()

	state := fetchLatestVersion()

	versionMu.Lock()
	cachedVersionState = state
	cachedVersionAt = time.Now()
	versionMu.Unlock()
	return state.toPayload()
}

func fetchLatestVersion() versionState {
	current := normalizeVersion(utils.AppVersion)
	state := versionState{
		CurrentVersion: current,
		DockerImage:    utils.DockerImage,
		CheckedAt:      time.Now().Format(time.RFC3339),
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(http.MethodGet, versionCheckURL, nil)
	if err != nil {
		state.Error = err.Error()
		return state
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "KiroClaim/"+current)

	resp, err := client.Do(req)
	if err != nil {
		state.Error = err.Error()
		return state
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		state.Error = fmt.Sprintf("GitHub 返回 HTTP %d", resp.StatusCode)
		return state
	}

	var payload struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		state.Error = err.Error()
		return state
	}
	state.LatestVersion = normalizeVersion(payload.TagName)
	state.ReleaseURL = payload.HTMLURL
	state.HasUpdate = compareVersions(state.LatestVersion, current) > 0
	return state
}

func normalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || v == "dev" {
		return "dev"
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	return v
}

var versionNumberRe = regexp.MustCompile(`\d+`)

func compareVersions(a, b string) int {
	if a == b {
		return 0
	}
	if a == "dev" {
		return -1
	}
	if b == "dev" {
		return 1
	}
	aa := versionNumberRe.FindAllString(a, -1)
	bb := versionNumberRe.FindAllString(b, -1)
	max := len(aa)
	if len(bb) > max {
		max = len(bb)
	}
	for i := 0; i < max; i++ {
		var av, bv int
		if i < len(aa) {
			av, _ = strconv.Atoi(aa[i])
		}
		if i < len(bb) {
			bv, _ = strconv.Atoi(bb[i])
		}
		if av > bv {
			return 1
		}
		if av < bv {
			return -1
		}
	}
	return strings.Compare(a, b)
}

func dockerUpdateSupported() bool {
	if strings.TrimSpace(os.Getenv("APP_UPDATE_COMMAND")) != "" {
		return true
	}
	if _, err := exec.LookPath("docker"); err != nil {
		return false
	}
	if _, err := os.Stat("/var/run/docker.sock"); err != nil {
		return false
	}
	if _, err := hostComposeFile(); err != nil {
		return false
	}
	return true
}

func startDockerUpdate(source string) (string, error) {
	updateMu.Lock()
	if updateInProgress {
		updateMu.Unlock()
		return "", errors.New("已有更新任务正在执行")
	}
	updateInProgress = true
	updateLastStartedAt = time.Now()
	updateLastMessage = "更新任务已启动"
	updateMu.Unlock()

	cmdText, err := dockerUpdateCommand()
	if err != nil {
		updateMu.Lock()
		updateInProgress = false
		updateLastMessage = err.Error()
		updateMu.Unlock()
		return "", err
	}

	go runDockerUpdate(cmdText, source)
	return "更新任务已启动，Docker 将拉取最新镜像并重建容器", nil
}

func runDockerUpdate(cmdText, source string) {
	log.Printf("Docker 更新任务启动 source=%s command=%s", source, cmdText)
	cmd := exec.Command("sh", "-c", cmdText)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		finishDockerUpdate("启动更新命令失败: "+err.Error(), false)
		return
	}

	var lines []string
	var linesMu sync.Mutex
	var wg sync.WaitGroup
	readPipe := func(scanner *bufio.Scanner) {
		defer wg.Done()
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" {
				linesMu.Lock()
				lines = append(lines, line)
				linesMu.Unlock()
			}
		}
	}
	wg.Add(2)
	go readPipe(bufio.NewScanner(stdout))
	go readPipe(bufio.NewScanner(stderr))

	err := cmd.Wait()
	wg.Wait()
	msg := "Docker 更新完成"
	if len(lines) > 0 {
		if len(lines) > 6 {
			lines = lines[len(lines)-6:]
		}
		msg += ": " + strings.Join(lines, " | ")
	}
	if err != nil {
		msg = "Docker 更新失败: " + err.Error()
		if len(lines) > 0 {
			msg += " | " + strings.Join(lines, " | ")
		}
	}
	finishDockerUpdate(msg, err == nil)
}

func finishDockerUpdate(message string, ok bool) {
	updateMu.Lock()
	updateInProgress = false
	updateLastMessage = message
	updateMu.Unlock()
	if ok {
		log.Println(message)
	} else {
		log.Println(message)
	}
}

func dockerUpdateCommand() (string, error) {
	if custom := strings.TrimSpace(os.Getenv("APP_UPDATE_COMMAND")); custom != "" {
		return custom, nil
	}
	if _, err := exec.LookPath("docker"); err != nil {
		return "", errors.New("当前运行环境没有 docker 命令，无法在应用内更新")
	}
	if _, err := os.Stat("/var/run/docker.sock"); err != nil {
		return "", errors.New("当前容器未挂载 /var/run/docker.sock，无法操作宿主 Docker")
	}
	composeFile, err := hostComposeFile()
	if err != nil {
		return "", errors.New("未找到 docker-compose.yml；如需应用内更新，请挂载 compose 文件或配置 APP_UPDATE_COMMAND")
	}
	hostDir := filepath.Dir(composeFile)
	fileName := filepath.Base(composeFile)
	return fmt.Sprintf(
		"docker run --rm -v /var/run/docker.sock:/var/run/docker.sock -v %s:%s -w %s docker:29-cli sh -c %s",
		shellQuote(hostDir),
		shellQuote(hostDir),
		shellQuote(hostDir),
		shellQuote("docker compose -f "+fileName+" pull && docker compose -f "+fileName+" up -d"),
	), nil
}

func hostComposeFile() (string, error) {
	if explicit := strings.TrimSpace(os.Getenv("APP_UPDATE_COMPOSE_FILE")); explicit != "" {
		return explicit, nil
	}

	hostname, err := os.Hostname()
	if err != nil || strings.TrimSpace(hostname) == "" {
		return "", errors.New("无法获取当前容器 ID")
	}
	raw, err := exec.Command("docker", "inspect", hostname).Output()
	if err != nil {
		return "", err
	}
	var containers []struct {
		Mounts []struct {
			Source      string `json:"Source"`
			Destination string `json:"Destination"`
		} `json:"Mounts"`
	}
	if err := json.Unmarshal(raw, &containers); err != nil {
		return "", err
	}
	for _, container := range containers {
		for _, mount := range container.Mounts {
			if mount.Destination == "/app/docker-compose.yml" && strings.TrimSpace(mount.Source) != "" {
				return mount.Source, nil
			}
		}
	}
	return "", errors.New("当前容器未挂载 /app/docker-compose.yml")
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

func isUpdateInProgress() bool {
	updateMu.Lock()
	defer updateMu.Unlock()
	return updateInProgress
}

func getUpdateLastMessage() string {
	updateMu.Lock()
	defer updateMu.Unlock()
	return updateLastMessage
}

func StartAutoUpdateScheduler() {
	updateSchedulerOnce.Do(func() {
		go func() {
			timer := time.NewTimer(2 * time.Minute)
			defer timer.Stop()
			for {
				<-timer.C
				runAutoUpdateTick()
				timer.Reset(autoUpdateInterval)
			}
		}()
	})
}

func runAutoUpdateTick() {
	if !GetCurrentSettings().AutoUpdateEnabled {
		return
	}
	state := getVersionState(true)
	if state.Error != "" || !state.HasUpdate {
		return
	}
	if _, err := startDockerUpdate("auto"); err != nil {
		log.Printf("自动更新未执行: %v", err)
		return
	}
	log.Printf("检测到新版本，已触发自动更新: %s -> %s", state.CurrentVersion, state.LatestVersion)
}
