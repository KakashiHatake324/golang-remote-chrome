package chrome

import (
	"context"
	"log"
	"net"
	"strconv"

	"github.com/KakashiHatake324/golang-remote-chrome/internal/logger"

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
	proxyUser  string
	proxyPass  string
}

// find an open port
func findPort() (string, error) {
	l, close, err := createListener()
	if err != nil {
		return "", err
	}
	port := l.Addr().(*net.TCPAddr).Port
	close()
	return strconv.Itoa(port), nil
}

// create a listener to find an open port
func createListener() (l net.Listener, close func(), newerr error) {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Println(err)
		return nil, nil, err
	}
	return l, func() {
		_ = l.Close()
	}, nil
}

// NewOptions creates a new Options
func NewOptions(ctx *context.Context, chromePath string, headless bool, proxy, user string, verbose bool) (*Options, error) {
	var proxyUser string
	var proxyPass string
	port, err := findPort()
	if err != nil {
		return nil, err
	}
	if proxy != "" {
		proxy, proxyUser, proxyPass, err = formatProxy(proxy)
		if err != nil {
			return nil, err
		}
	}
	return &Options{
		context:    ctx,
		chromePath: chromePath,
		port:       port,
		headless:   headless,
		proxy:      proxy,
		user:       user,
		proxyUser:  proxyUser,
		proxyPass:  proxyPass,
		verbose:    verbose,
		logger:     logger.NewLoggerInstance(uuid.New().String(), "options"),
	}, nil
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

// GetProxyUser returns the proxy user
func (o *Options) GetProxyUser() string {
	return o.proxyUser
}

// GetProxyPass returns the proxy pass
func (o *Options) GetProxyPass() string {
	return o.proxyPass
}
