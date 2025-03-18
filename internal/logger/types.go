package logger

import "go.uber.org/zap"

// LoggerInstance is a struct that contains the id and logger
type LoggerInstance struct {
	id        string
	component string
	logger    *zap.Logger
}

func NewLoggerInstance(id string, component string) *LoggerInstance {
	return &LoggerInstance{
		id:        id,
		component: component,
		logger:    newLogger(),
	}
}

type MainLoggerInstance struct {
	logger *zap.Logger
}

func NewMainLoggerInstance() *MainLoggerInstance {
	return &MainLoggerInstance{
		logger: newLogger(),
	}
}
