// From https://gist.github.com/panta/2530672ca641d953ae452ecb5ef79d7d

package quicklog

import (
	"fmt"
	"io"
	"os"
	"path"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

type LogLevel int8

const defaultMaxBackups = 5
const defaultMaxSize = 50 // 50 MB

const (
	LogLevelTrace LogLevel = iota
	LogLevelDebug
	LogLevelInfo
	LogLevelError
	// LogLevelDisabled is not used to actually log anything.  Setting the log level to this
	// value will prevent any log messages from being written.
	LogLevelDisabled
)

func (level LogLevel) String() string {
	switch level {
	case LogLevelTrace:
		return "TRACE"
	case LogLevelDebug:
		return "DEBUG"
	case LogLevelInfo:
		return "INFO"
	case LogLevelError:
		return "ERROR"
	case LogLevelDisabled:
		return "DISABLED"
	}
	return "unknown"
}

// Configuration for logging
type ConfigT struct {
	// Directory to log to to when file logging is enabled
	Directory string
	// Filename is the name of the log file which will be placed inside the directory
	Filename string
	// MaxSize the max size in MB of the log file before it's rolled
	MaxSize int
	// MaxBackups the max number of rolled files to keep
	MaxBackups int
	// MaxAge the max age in days to keep a log file
	MaxAge int
	// Min LogLevel to log
	Level LogLevel
}

func (c *ConfigT) SetDefaults() {
	if c.MaxBackups == 0 {
		c.MaxBackups = defaultMaxBackups
	}
	if c.MaxSize == 0 {
		c.MaxSize = defaultMaxSize
	}
}

type LoggerT struct {
	RollingFile io.Writer
	Level       LogLevel
}

func ConfigureLogger(config ConfigT) *LoggerT {
	config.SetDefaults()
	var rollingFile io.Writer
	if config.Level == LogLevelDisabled {
		rollingFile = NullWriter{}
	} else {
		rollingFile = newRollingFile(config)
	}
	logger := &LoggerT{
		RollingFile: rollingFile,
		Level:       config.Level,
	}
	logger.Info("---------------------------- BEGIN ----------------------------")
	return logger
}

func (log LoggerT) Trace(msg string) {
	log.CreateLogEntry(msg, LogLevelTrace)
}

func (log LoggerT) Tracef(format string, a ...interface{}) {
	log.CreateLogEntry(fmt.Sprintf(format, a...), LogLevelTrace)
}

func (log LoggerT) Debug(msg string) {
	log.CreateLogEntry(msg, LogLevelDebug)
}

func (log LoggerT) Debugf(format string, a ...interface{}) {
	log.CreateLogEntry(fmt.Sprintf(format, a...), LogLevelDebug)
}

func (log LoggerT) Info(msg string) {
	log.CreateLogEntry(msg, LogLevelInfo)
}

func (log LoggerT) Infof(format string, a ...interface{}) {
	log.CreateLogEntry(fmt.Sprintf(format, a...), LogLevelInfo)
}

func (log LoggerT) InfoPrint(msg string) {
	fmt.Println(msg)
	log.Info(msg)
}

func (log LoggerT) InfoPrintf(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	log.InfoPrint(msg)
}

func (log LoggerT) Error(msg string) {
	log.CreateLogEntry(msg, LogLevelError)
}

func (log LoggerT) Errorf(format string, a ...interface{}) {
	log.CreateLogEntry(fmt.Sprintf(format, a...), LogLevelError)
}

func (log LoggerT) ErrorPrint(msg string) {
	fmt.Println("ERROR: " + msg)
	log.Error(msg)
}

func (log LoggerT) ErrorPrintf(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	log.ErrorPrint(msg)
}

func (log LoggerT) CreateLogEntry(msg string, level LogLevel) {
	if level < log.Level {
		return
	}

	// TODO: Use the pattern below to implement an option allowing
	//       a timezone location to be specified (e.g., to log as UTC)
	// ----------------------------------------------------------------
	// location, _ := time.LoadLocation("America/New_York")
	// now := time.Now().In(location)
	// ----------------------------------------------------------------

	timestamp := time.Now().Format("2006-01-02T15:04:05.000-0700")
	log.RollingFile.Write([]byte(fmt.Sprintf("%s | %-5s | %s\n", timestamp, level.String(), msg)))
}

func newRollingFile(config ConfigT) io.Writer {
	if err := os.MkdirAll(config.Directory, 0744); err != nil {
		fmt.Printf("Can't create log directory at: %s\n", config.Directory)
		return nil
	}

	return &lumberjack.Logger{
		Filename:   path.Join(config.Directory, config.Filename),
		MaxBackups: config.MaxBackups, // files
		MaxSize:    config.MaxSize,    // megabytes
		MaxAge:     config.MaxAge,     // days
	}
}

// -----------------------------
// NullWriter is an io.Writer that discards all input
// -----------------------------
type NullWriter struct{}

func (NullWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}
