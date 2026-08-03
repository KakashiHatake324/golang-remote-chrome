package recaptcha

import "fmt"

// stealthBase removes the most common headless-Chrome automation tells that drag
// down reCAPTCHA v3 scores. It is injected before any page script, in every
// frame (including the reCAPTCHA iframe). Every patch is wrapped in its own
// try/catch so one failure never aborts the rest, and it is idempotent.
//
// It deliberately does NOT touch WebGL / canvas / audio: on a real GPU those
// values are genuine and consistent with a host-matching UA, and forcing wrong
// values (e.g. a Windows GPU string on macOS) is worse than leaving them alone.
// WebGL spoofing is opt-in via buildStealthScript for headless-Linux/SwiftShader.
const stealthBase = `(function(){
  'use strict';
  var def = function(obj, prop, get){
    try { Object.defineProperty(obj, prop, { get: get, configurable: true }); } catch(e){}
  };

  // navigator.webdriver -> false (reinforces the launch flag).
  try { def(Navigator.prototype, 'webdriver', function(){ return false; }); } catch(e){}
  try { delete Object.getPrototypeOf(navigator).webdriver; } catch(e){}

  // Plausible locale + hardware.
  def(navigator, 'languages', function(){ return ['en-US','en']; });
  def(navigator, 'hardwareConcurrency', function(){ return 8; });
  def(navigator, 'deviceMemory', function(){ return 8; });

  // Non-empty plugins/mimeTypes (headless reports zero, a strong bot signal).
  try {
    var mkPlugin = function(name, desc, fn){ return { name:name, description:desc, filename:fn, length:1 }; };
    var plugins = [
      mkPlugin('PDF Viewer','Portable Document Format','internal-pdf-viewer'),
      mkPlugin('Chrome PDF Viewer','Portable Document Format','internal-pdf-viewer'),
      mkPlugin('Chromium PDF Viewer','Portable Document Format','internal-pdf-viewer'),
      mkPlugin('Microsoft Edge PDF Viewer','Portable Document Format','internal-pdf-viewer'),
      mkPlugin('WebKit built-in PDF','Portable Document Format','internal-pdf-viewer')
    ];
    plugins.item = function(i){ return this[i]; };
    plugins.namedItem = function(n){ return this.find(function(p){ return p.name===n; }) || null; };
    plugins.refresh = function(){};
    def(navigator, 'plugins', function(){ return plugins; });
    var mimes = [{ type:'application/pdf', suffixes:'pdf', description:'Portable Document Format' }];
    mimes.item = function(i){ return this[i]; };
    mimes.namedItem = function(n){ return this.find(function(m){ return m.type===n; }) || null; };
    def(navigator, 'mimeTypes', function(){ return mimes; });
  } catch(e){}

  // window.chrome runtime object (present in real Chrome, absent in headless).
  try {
    if (!window.chrome) { window.chrome = {}; }
    if (!window.chrome.runtime) { window.chrome.runtime = {}; }
    if (!window.chrome.app) { window.chrome.app = { isInstalled:false, InstallState:{ DISABLED:'disabled', INSTALLED:'installed', NOT_INSTALLED:'not_installed' }, RunningState:{ CANNOT_RUN:'cannot_run', READY_TO_RUN:'ready_to_run', RUNNING:'running' } }; }
    if (!window.chrome.csi) { window.chrome.csi = function(){ return {}; }; }
    if (!window.chrome.loadTimes) { window.chrome.loadTimes = function(){ return {}; }; }
  } catch(e){}

  // permissions.query for notifications should agree with Notification.permission.
  try {
    var q = window.navigator.permissions && window.navigator.permissions.query;
    if (q) {
      window.navigator.permissions.query = function(p){
        if (p && p.name === 'notifications') {
          return Promise.resolve({ state: (typeof Notification !== 'undefined' ? Notification.permission : 'default') });
        }
        return q.apply(window.navigator.permissions, arguments);
      };
    }
  } catch(e){}
})();`

// webglTemplate spoofs UNMASKED_VENDOR_WEBGL (37445) / UNMASKED_RENDERER_WEBGL
// (37446). Only injected when a vendor/renderer is configured.
const webglTemplate = `(function(){
  'use strict';
  try {
    var patchGL = function(proto){
      if (!proto || !proto.getParameter) return;
      var gp = proto.getParameter;
      proto.getParameter = function(param){
        if (param === 37445) return %q;
        if (param === 37446) return %q;
        return gp.apply(this, arguments);
      };
    };
    if (window.WebGLRenderingContext) patchGL(WebGLRenderingContext.prototype);
    if (window.WebGL2RenderingContext) patchGL(WebGL2RenderingContext.prototype);
  } catch(e){}
})();`

// buildStealthScript returns the stealth init script, optionally including a
// WebGL vendor/renderer spoof when both values are provided.
func buildStealthScript(webglVendor, webglRenderer string) string {
	if webglVendor != "" && webglRenderer != "" {
		return stealthBase + "\n;" + fmt.Sprintf(webglTemplate, webglVendor, webglRenderer)
	}
	return stealthBase
}
