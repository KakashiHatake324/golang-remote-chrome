package chrome

import (
	"fmt"
	"strings"

	"github.com/KakashiHatake324/mockjs"
)

type FlagType string

const (
	Incognito                                     FlagType = "--incognito"
	DisableExtentions                             FlagType = "--disable-extensions"
	IgnoreCertificateErrors                       FlagType = "--ignore-certificate-errors"
	SingleProcess                                 FlagType = "--single-process"
	DisableIsolationTrials                        FlagType = "--disable-site-isolation-trials"
	DisableWebSecurity                            FlagType = "--disable-web-security"
	DisableWebResources                           FlagType = "--disable-web-resources"
	DisableAutomations                            FlagType = "--disable-animations"
	ForceDeviceScaleFactor1                       FlagType = "--force-device-scale-factor=1"
	ForceDeviceScaleFactor2                       FlagType = "--force-device-scale-factor=2"
	EnableLowEndMode                              FlagType = "--enable-low-end-device-mode"
	DisableLogging                                FlagType = "--disable-logging"
	DisableInfoBars                               FlagType = "--disable-infobars"
	DisableSmoothScrolling                        FlagType = "--disable-smooth-scrolling"
	DisablePreConnect                             FlagType = "--disable-preconnect"
	DisableGPU                                    FlagType = "--disable-gpu"
	PurgeMemoryButton                             FlagType = "--purge-memory-button"
	DisableTranslate                              FlagType = "--disable-translate"
	NoSandbox                                     FlagType = "--no-sandbox"
	DisableBackgroundMode                         FlagType = "--disable-background-mode"
	DisableBlinkAutomationControlled              FlagType = "--disable-blink-features=AutomationControlled"
	HideCrashRestoreBubble                        FlagType = "--hide-crash-restore-bubble"
	HideSavePasswordBubble                        FlagType = "--hide-save-password-bubble"
	RestoreLastSession                            FlagType = "--restore-last-session"
	DisableSync                                   FlagType = "--disable-sync"
	EnableSmoothScrolling                         FlagType = "--enable-smooth-scrolling"
	TurnOffStreamingMediaCaching                  FlagType = "--turn-off-streaming-media-caching"
	EnableQuic                                    FlagType = "--enable-quic"
	EnableZeroCopy                                FlagType = "--enable-zero-copy"
	EnableGPUResterization                        FlagType = "--enable-gpu-rasterization"
	NoFirstRun                                    FlagType = "--no-first-run"
	TestType                                      FlagType = "--test-type"
	DisableBackgroundNetworking                   FlagType = "--disable-background-networking"
	EnableRemoteDebugging                         FlagType = "--enable-remote-debugging"
	MuteAudio                                     FlagType = "--mute-audio"
	DisableBackgroundTimerThrottling              FlagType = "--disable-background-timer-throttling"
	DisableBackgroundOccludedWindows              FlagType = "--disable-backgrounding-occluded-windows"
	DisableComponentExtensionsWithBackgroundPages FlagType = "--disable-component-extensions-with-background-pages"
	NoDefaultBrowserCheck                         FlagType = "--no-default-browser-check"
	DisableRendererBackgrounding                  FlagType = "--disable-renderer-backgrounding"
	DisableInProcessStackTraces                   FlagType = "--disable-in-process-stack-traces"
	DisableDevSHM                                 FlagType = "--disable-dev-shm-usage"
	DisableSoftwareRasterizer                     FlagType = "--disable-software-rasterizer"
	DisableWebGl                                  FlagType = "--disable-webgl"
)

// generate random window size flag
func RandomWindowSize() FlagType {
	return FlagType(
		fmt.Sprintf(
			"--window-size=%.f,%.f", mockjs.Math.NumberBetween(800, 1400), mockjs.Math.NumberBetween(600, 1000),
		),
	)
}

// set the window size flag
func SetWindowSize(width, height int) FlagType {
	return FlagType(fmt.Sprintf("--window-size=%d,%d", width, height))
}

// generate the disable blink features flag
func DisableBlinkFeatures(features []string) FlagType {
	return FlagType(fmt.Sprintf("--disable-blink-features=%s", strings.Join(features, ",")))
}

// generate the enable blink features flag
func EnableBlinkFeatures(features []string) FlagType {
	return FlagType(fmt.Sprintf("--enable-blink-features=%s", strings.Join(features, ",")))
}

// generate the enable features flag
func EnableFeatures(features []string) FlagType {
	return FlagType(fmt.Sprintf("--enable-features=%s", strings.Join(features, ",")))
}

// generate the disable features flag
func DisableFeatures(features []string) FlagType {
	return FlagType(fmt.Sprintf("--disable-features=%s", strings.Join(features, ",")))
}

func StackTraceLimit(limit int) FlagType {
	return FlagType(fmt.Sprintf("--stack-trace-limit %d", limit))
}

func StackTraceLimitV8(limit int) FlagType {
	return FlagType(fmt.Sprintf("--js-flags=\x27--stack-trace-limit %d\x27", limit))
}

func SetUserAgent(ua string) FlagType {
	return FlagType(fmt.Sprintf("--user-agent=%s", ua))
}
