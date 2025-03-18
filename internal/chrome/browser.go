package chrome

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/KakashiHatake324/golang-remote-chrome/internal/logger"
	"github.com/google/uuid"
)

// Browser represents a single instance of the Chrome browser
type Browser struct {
	context     *context.Context
	proc        *os.Process
	proxy       string
	Opts        *Options
	Pages       map[string]*Page
	CurrentPage *Page
	pageLock    sync.Mutex
	logger      *logger.LoggerInstance
	verbose     bool
}

// newBrowser creates a new Browser
func newBrowser(ctx *context.Context, proc *os.Process, proxy string, opts *Options, startPage *Page, startPageId string) *Browser {
	return &Browser{
		context:     ctx,
		proc:        proc,
		proxy:       proxy,
		Opts:        opts,
		Pages:       map[string]*Page{startPageId: startPage},
		CurrentPage: startPage,
		logger:      logger.NewLoggerInstance(uuid.New().String(), "browser"),
		verbose:     opts.GetVerbose(),
	}
}

// Close closes the browser and the process and removes the allocated memory
func (b *Browser) Close() error {
	b.pageLock.Lock()
	defer b.pageLock.Unlock()
	if b.verbose {
		b.logger.Info("closing browser")
	}
	for _, page := range b.Pages {
		page.close()
		delete(b.Pages, page.id)
	}
	if b.proc != nil {
		if b.verbose {
			b.logger.Info("killing browser process")
		}
		return b.proc.Kill()
	}
	b = nil
	return nil
}

// ClosePage closes a page and removes it from the Browser
func (b *Browser) ClosePage(page *Page) error {
	b.pageLock.Lock()
	defer b.pageLock.Unlock()
	if page == nil {
		return fmt.Errorf("page not found")
	}
	page.close()
	delete(b.Pages, page.id)
	return nil
}

// GetOptions returns the options of the Browser
func (o *Browser) GetOptions() *Options {
	return o.Opts
}

// GetPages returns the pages of the Browser
func (o *Browser) GetPages() map[string]*Page {
	o.pageLock.Lock()
	defer o.pageLock.Unlock()
	return o.Pages
}

// AddPage adds a page to the Browser
func (o *Browser) AddPage(page *Page) {
	o.pageLock.Lock()
	defer o.pageLock.Unlock()
	o.Pages[page.id] = page
}

// GetPage returns a page from the Browser
func (o *Browser) GetPage(id string) *Page {
	o.pageLock.Lock()
	defer o.pageLock.Unlock()
	for _, page := range o.Pages {
		if page.id == id {
			return page
		}
	}
	return nil
}
