package logger

import (
	"fmt"
	"os"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	colorReset = "\033[0m"
	colorInfo  = "\033[34m" // Blue
	colorWarn  = "\033[33m" // Yellow
	colorError = "\033[31m" // Red
	colorFatal = "\033[31m" // Red
	colorDebug = "\033[36m" // Cyan
	Logger     = NewMainLoggerInstance()
)

// create a new logger for out database
func newLogger() *zap.Logger {
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    CustomLevelEncoder,
		EncodeTime:     CustomTimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}
	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderConfig),
		zapcore.AddSync(zapcore.Lock(os.Stdout)),
		zap.InfoLevel,
	)
	return zap.New(core)
}

// change the time format for the logger
func CustomTimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.Format("2006-01-02 15:04:05.000"))
}

// CustomLevelEncoder colors the log level based on its severity.
func CustomLevelEncoder(level zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
	var color string
	switch level {
	case zapcore.DebugLevel:
		color = colorDebug
	case zapcore.InfoLevel:
		color = colorInfo
	case zapcore.WarnLevel:
		color = colorWarn
	case zapcore.ErrorLevel:
		color = colorError
	case zapcore.FatalLevel:
		color = colorFatal
	default:
		color = colorReset
	}
	enc.AppendString(fmt.Sprintf("%s%s%s", color, level.CapitalString(), colorReset))
}

// log info
func (d *LoggerInstance) Info(message string) {
	d.logger.Info(
		message,
		zap.String("id", d.id),
		zap.String("component", d.component),
	)
}

// log warn
func (d *LoggerInstance) Warn(message string) {
	d.logger.Warn(message,
		zap.String("id", d.id),
		zap.String("component", d.component),
	)
}

// log error
func (d *LoggerInstance) Error(message string, err error) {
	d.logger.Error(message,
		zap.String("id", d.id),
		zap.Error(err),
		zap.String("component", d.component),
	)
}

// log fatal
func (d *LoggerInstance) Fatal(message string, err error) {
	d.logger.Fatal(message,
		zap.String("id", d.id),
		zap.Error(err),
		zap.String("component", d.component),
	)
}

// log debug
func (d *LoggerInstance) Debug(message string) {
	d.logger.Debug(message,
		zap.String("id", d.id),
		zap.String("component", d.component),
	)
}
