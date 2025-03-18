package chrome

import (
	"context"
	"golang-remote-chrome/internal/logger"

	"github.com/google/uuid"
)

// Options represents the options for the Chrome browser
type Options struct {
	context    *context.Context
	chromePath string
	port       string
	args       []string
	headless   bool
	proxy      string
	user       string
	verbose    bool
	logger     *logger.LoggerInstance
}

// NewOptions creates a new Options
func NewOptions(ctx *context.Context, chromePath string, port string, headless bool, proxy, user string, verbose bool) *Options {
	return &Options{
		context:    ctx,
		chromePath: chromePath,
		port:       port,
		headless:   headless,
		proxy:      proxy,
		user:       user,
		verbose:    verbose,
		logger:     logger.NewLoggerInstance(uuid.New().String(), "options"),
	}
}

// GetContext returns the context
func (o *Options) GetContext() *context.Context {
	return o.context
}

// GetChromePath returns the chrome path
func (o *Options) GetChromePath() string {
	return o.chromePath
}

// GetPort returns the port
func (o *Options) GetPort() string {
	return o.port
}

// GetHeadless returns the headless flag
func (o *Options) GetHeadless() bool {
	return o.headless
}

// SetArgs sets the args
func (o *Options) SetArgs(args []string) {
	o.args = args
}

// GetArgs returns the args
func (o *Options) GetArgs() []string {
	return o.args
}

// GetProxy returns the proxy
func (o *Options) GetProxy() string {
	return o.proxy
}

// GetUser returns the user
func (o *Options) GetUser() string {
	return o.user
}

// GetVerbose returns the verbose flag
func (o *Options) GetVerbose() bool {
	return o.verbose
}

// GetLogger returns the logger
func (o *Options) GetLogger() *logger.LoggerInstance {
	return o.logger
}
