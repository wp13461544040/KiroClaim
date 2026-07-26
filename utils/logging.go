package utils

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"gopkg.in/natefinch/lumberjack.v2"
)

type LoggingConfig struct {
	FileEnabled bool
	FilePath    string
	MaxSizeMB   int
	MaxBackups  int
	MaxAgeDays  int
	Compress    bool
}

type switchableLogWriter struct {
	mu sync.RWMutex
	w  io.Writer
}

func (s *switchableLogWriter) Write(p []byte) (int, error) {
	s.mu.RLock()
	w := s.w
	s.mu.RUnlock()
	if w == nil {
		w = os.Stdout
	}
	return w.Write(p)
}

func (s *switchableLogWriter) Set(w io.Writer) {
	if w == nil {
		w = os.Stdout
	}
	s.mu.Lock()
	s.w = w
	s.mu.Unlock()
}

var (
	logWriter       = &switchableLogWriter{w: os.Stdout}
	activeLogMu     sync.Mutex
	activeLogCloser io.Closer
)

func InitLogging() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	gin.DefaultWriter = logWriter
	gin.DefaultErrorWriter = logWriter
	log.SetOutput(logWriter)
	if err := ApplyLoggingConfig(DefaultLoggingConfigFromEnv()); err != nil {
		log.Printf("日志配置初始化失败，继续使用控制台日志: %v", err)
	}
}

func DefaultLoggingConfigFromEnv() LoggingConfig {
	logPath := strings.TrimSpace(os.Getenv("LOG_FILE_PATH"))
	if logPath == "" {
		logDir := strings.TrimSpace(os.Getenv("LOG_DIR"))
		if logDir == "" {
			logDir = "logs"
		}
		logName := strings.TrimSpace(os.Getenv("LOG_FILE_NAME"))
		if logName == "" {
			logName = "app.log"
		}
		logPath = filepath.Join(logDir, logName)
	}
	return NormalizeLoggingConfig(LoggingConfig{
		FileEnabled: envBool("LOG_FILE_ENABLED", true),
		FilePath:    logPath,
		MaxSizeMB:   envInt("LOG_MAX_SIZE_MB", 20),
		MaxBackups:  envInt("LOG_MAX_BACKUPS", 7),
		MaxAgeDays:  envInt("LOG_MAX_AGE_DAYS", 30),
		Compress:    envBool("LOG_COMPRESS", false),
	})
}

func NormalizeLoggingConfig(c LoggingConfig) LoggingConfig {
	c.FilePath = strings.TrimSpace(c.FilePath)
	if c.FilePath == "" {
		c.FilePath = filepath.Join("logs", "app.log")
	}
	if c.MaxSizeMB <= 0 {
		c.MaxSizeMB = 20
	}
	if c.MaxBackups <= 0 {
		c.MaxBackups = 7
	}
	if c.MaxAgeDays <= 0 {
		c.MaxAgeDays = 30
	}
	return c
}

func ApplyLoggingConfig(c LoggingConfig) error {
	c = NormalizeLoggingConfig(c)

	activeLogMu.Lock()
	defer activeLogMu.Unlock()

	var next io.Writer = os.Stdout
	var nextCloser io.Closer
	if c.FileEnabled {
		if err := os.MkdirAll(filepath.Dir(c.FilePath), 0755); err != nil {
			return err
		}
		fileWriter := &lumberjack.Logger{
			Filename:   c.FilePath,
			MaxSize:    c.MaxSizeMB,
			MaxBackups: c.MaxBackups,
			MaxAge:     c.MaxAgeDays,
			Compress:   c.Compress,
			LocalTime:  true,
		}
		next = io.MultiWriter(os.Stdout, fileWriter)
		nextCloser = fileWriter
	}

	oldCloser := activeLogCloser
	logWriter.Set(next)
	activeLogCloser = nextCloser
	if oldCloser != nil {
		_ = oldCloser.Close()
	}

	if c.FileEnabled {
		log.Printf("日志文件已启用: %s", c.FilePath)
	} else {
		log.Println("日志文件已关闭，仅输出到控制台")
	}
	return nil
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return fallback
	}
	return value
}

func envBool(key string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	switch strings.ToLower(raw) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}
