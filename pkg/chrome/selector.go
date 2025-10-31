package chrome

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

// Selector represents a DOM element selector
type Selector struct {
	page     *Page
	selector string
}

// NewSelector creates a new Selector
func (p *Page) NewSelector(selector string) *Selector {
	return &Selector{
		page:     p,
		selector: selector,
	}
}

// FrameSelector represents a selector scoped within an iframe
type FrameSelector struct {
	page           *Page
	iframeSelector string
	selector       string
}

// NewFrameSelector creates a new FrameSelector scoped to a specific iframe
// Note: Works only for same-origin iframes. Cross-origin iframes are restricted by the browser.
func (p *Page) NewFrameSelector(iframeSelector string, selector string) *FrameSelector {
	return &FrameSelector{
		page:           p,
		iframeSelector: iframeSelector,
		selector:       selector,
	}
}

// NewFrameSelectorByIndex selects an iframe by its index in the document
func (p *Page) NewFrameSelectorByIndex(index int, selector string) *FrameSelector {
	iframeSelector := fmt.Sprintf("iframe:nth-of-type(%d)", index+1)
	return p.NewFrameSelector(iframeSelector, selector)
}

// NewFrameSelectorBySrcContains selects an iframe by a substring in its src attribute
func (p *Page) NewFrameSelectorBySrcContains(substring string, selector string) *FrameSelector {
	iframeSelector := fmt.Sprintf("iframe[src*=%q]", substring)
	return p.NewFrameSelector(iframeSelector, selector)
}

// NewFrameSelectorByAriaLabel selects an iframe by its aria-label attribute
func (p *Page) NewFrameSelectorByAriaLabel(label string, selector string) *FrameSelector {
	iframeSelector := fmt.Sprintf("iframe[aria-label=%q]", label)
	return p.NewFrameSelector(iframeSelector, selector)
}

// GetText gets the inner text of the element matching the selector
// Returns the text content of the element, or an error if the element is not found
func (s *Selector) GetText() (string, error) {
	s.page.handleVerbose(fmt.Sprintf("getting text from element with selector: %s", s.selector))

	// Get the inner text of the element
	textScript := fmt.Sprintf(
		`
		(() => {
			const element = document.querySelector(%q);
			if (!element) {
				return null;
			}
			return element.innerText || element.textContent || '';
		})()
	`, s.selector,
	)

	result, err := s.page.Evaluate(textScript)
	if err != nil {
		return "", fmt.Errorf("error getting element text: %w", err)
	}

	// Check if element was not found (null value)
	if result.Value == nil {
		return "", fmt.Errorf("element with selector %s not found", s.selector)
	}

	// Safely extract string value
	var text string
	switch v := result.Value.(type) {
	case string:
		text = v
	case map[string]any:
		// If it's a map, check if there's a "value" field (nested structure)
		if valueStr, exists := v["value"].(string); exists {
			text = valueStr
		} else {
			// Try to convert the whole map to string
			text = result.StringValue()
		}
	case nil:
		return "", fmt.Errorf("element with selector %s not found", s.selector)
	default:
		// Try to convert to string as fallback
		text = result.StringValue()
	}

	s.page.handleVerbose(fmt.Sprintf("successfully got text from element with selector: %s (text: %s)", s.selector, text))
	return text, nil
}

// GetText gets the inner text of the element inside the iframe matching the selector
func (fs *FrameSelector) GetText() (string, error) {
	fs.page.handleVerbose(fmt.Sprintf("getting text from element with selector: %s inside iframe: %s", fs.selector, fs.iframeSelector))

	textScript := fmt.Sprintf(
		`
		(() => {
			const iframe = document.querySelector(%q);
			if (!iframe || !iframe.contentWindow) return null;
			const doc = iframe.contentDocument || iframe.contentWindow.document;
			const element = doc.querySelector(%q);
			if (!element) return null;
			return element.innerText || element.textContent || '';
		})()
	`, fs.iframeSelector, fs.selector,
	)

	result, err := fs.page.Evaluate(textScript)
	if err != nil {
		return "", fmt.Errorf("error getting element text in iframe: %w", err)
	}
	if result.Value == nil {
		return "", fmt.Errorf("element with selector %s not found in iframe %s", fs.selector, fs.iframeSelector)
	}

	var text string
	switch v := result.Value.(type) {
	case string:
		text = v
	case map[string]any:
		if valueStr, exists := v["value"].(string); exists {
			text = valueStr
		} else {
			text = result.StringValue()
		}
	case nil:
		return "", fmt.Errorf("element with selector %s not found in iframe %s", fs.selector, fs.iframeSelector)
	default:
		text = result.StringValue()
	}

	fs.page.handleVerbose(fmt.Sprintf("successfully got text from element: %s inside iframe: %s (text: %s)", fs.selector, fs.iframeSelector, text))
	return text, nil
}

// Click clicks on the element matching the selector
func (s *Selector) Click() error {
	s.page.handleVerbose(fmt.Sprintf("human-like clicking on element with selector: %s", s.selector))

	// Ensure the element exists and is visible
	visibilityScript := fmt.Sprintf(`
	(() => {
		const element = document.querySelector(%q);
		if (!element) return { ok: false, reason: "not found" };
		
		const rect = element.getBoundingClientRect();
		const visible = !!(rect.width && rect.height && element.offsetParent !== null);
		if (!visible) return { ok: false, reason: "not visible" };
		
		element.scrollIntoView({ behavior: "auto", block: "center" });
		return { ok: true };
	})()
	`, s.selector)

	visibilityResult, err := s.page.Evaluate(visibilityScript)
	if err != nil {
		return fmt.Errorf("error checking element visibility: %w", err)
	}

	switch v := visibilityResult.Value.(type) {
	case map[string]any:
		if !v["ok"].(bool) {
			return fmt.Errorf("failed to click element %s", s.selector)
		}
	case bool:
		if !v {
			return fmt.Errorf("failed to click element %s", s.selector)
		}
	}

	time.Sleep(120 * time.Millisecond)

	// Simulate human-like mouse move and click
	clickScript := fmt.Sprintf(`
	(async () => {
		function sleep(ms) { return new Promise(r => setTimeout(r, ms)); }

		async function moveMouseHumanlyTo(x, y) {
			if (!window._mouse) window._mouse = { x: 0, y: 0 };
			const mouse = window._mouse;
			const steps = 25 + Math.floor(Math.random() * 15);
			const dx = (x - mouse.x) / steps;
			const dy = (y - mouse.y) / steps;

			for (let i = 0; i < steps; i++) {
				const jx = (Math.random() - 0.5) * 3;
				const jy = (Math.random() - 0.5) * 3;
				mouse.x += dx + jx;
				mouse.y += dy + jy;

				document.dispatchEvent(new MouseEvent("mousemove", {
					clientX: mouse.x,
					clientY: mouse.y,
					bubbles: true
				}));

				await sleep(5 + Math.random() * 15);
			}

			mouse.x = x;
			mouse.y = y;
			document.dispatchEvent(new MouseEvent("mousemove", { clientX: x, clientY: y, bubbles: true }));
		}

		const el = document.querySelector(%q);
		if (!el) return false;

		const rect = el.getBoundingClientRect();
		const cx = rect.left + rect.width / 2 + (Math.random() - 0.5) * 8;
		const cy = rect.top + rect.height / 2 + (Math.random() - 0.5) * 5;

		await moveMouseHumanlyTo(cx, cy);
		await sleep(80 + Math.random() * 120);

		document.dispatchEvent(new MouseEvent("mouseover", { clientX: cx, clientY: cy, bubbles: true }));
		document.dispatchEvent(new MouseEvent("mousemove", { clientX: cx, clientY: cy, bubbles: true }));
		document.dispatchEvent(new MouseEvent("mousedown", { clientX: cx, clientY: cy, bubbles: true }));
		await sleep(50 + Math.random() * 120);
		document.dispatchEvent(new MouseEvent("mouseup", { clientX: cx, clientY: cy, bubbles: true }));

		// Fire real click event
		el.dispatchEvent(new MouseEvent("click", {
			clientX: cx,
			clientY: cy,
			bubbles: true,
			cancelable: true
		}));

		return true;
	})();
	`, s.selector)

	clickResult, err := s.page.Evaluate(clickScript)
	if err != nil {
		return fmt.Errorf("error executing human-like click: %w", err)
	}

	switch v := clickResult.Value.(type) {
	case map[string]any:
		if !v["ok"].(bool) {
			return fmt.Errorf("failed to click element %s", s.selector)
		}
	case bool:
		if !v {
			return fmt.Errorf("failed to click element %s", s.selector)
		}
	}

	s.page.handleVerbose(fmt.Sprintf("successfully performed human-like click on selector: %s", s.selector))
	return nil
}

// Click clicks on the element inside the iframe matching the selector
func (fs *FrameSelector) Click() error {
	fs.page.handleVerbose(fmt.Sprintf("clicking on element with selector: %s inside iframe: %s", fs.selector, fs.iframeSelector))

	// Ensure element exists and is visible within the iframe
	visibilityScript := fmt.Sprintf(
		`
		(() => {
			const iframe = document.querySelector(%q);
			if (!iframe || !iframe.contentWindow) return false;
			const doc = iframe.contentDocument || iframe.contentWindow.document;
			const element = doc.querySelector(%q);
			if (!element) return false;

			const rect = element.getBoundingClientRect();
			const isVisible = !!(rect.width && rect.height && element.offsetParent !== null);
			if (!isVisible) return false;

			element.scrollIntoView({ behavior: "auto", block: "center" });
			return true;
		})()
	`, fs.iframeSelector, fs.selector,
	)

	visibilityResult, err := fs.page.Evaluate(visibilityScript)
	if err != nil {
		return fmt.Errorf("error checking element visibility in iframe: %w", err)
	}
	if !visibilityResult.BoolValueOrDefault() {
		return fmt.Errorf("element with selector %s not visible or not found in iframe %s", fs.selector, fs.iframeSelector)
	}

	// Small delay to ensure element is in view
	time.Sleep(100 * time.Millisecond)

	// Dispatch a click event within the iframe
	clickScript := fmt.Sprintf(
		`
		(() => {
			const iframe = document.querySelector(%q);
			if (!iframe || !iframe.contentWindow) return false;
			const doc = iframe.contentDocument || iframe.contentWindow.document;
			const element = doc.querySelector(%q);
			if (!element) return false;

			const clickEvent = new MouseEvent('click', { bubbles: true, cancelable: true, view: iframe.contentWindow });
			return element.dispatchEvent(clickEvent);
		})()
	`, fs.iframeSelector, fs.selector,
	)

	clickResult, err := fs.page.Evaluate(clickScript)
	if err != nil {
		return fmt.Errorf("error clicking element in iframe: %w", err)
	}
	if !clickResult.BoolValueOrDefault() {
		return fmt.Errorf("failed to click element %s in iframe %s", fs.selector, fs.iframeSelector)
	}

	fs.page.handleVerbose(fmt.Sprintf("successfully clicked on element: %s inside iframe: %s", fs.selector, fs.iframeSelector))
	return nil
}

// Input types text into the element matching the selector
func (s *Selector) Input(text string) error {
	s.page.handleVerbose(fmt.Sprintf("inputting text into element with selector: %s", s.selector))

	// First ensure the element exists and is an input or textarea
	validationScript := fmt.Sprintf(
		`
		(() => {
			const element = document.querySelector(%q);
			if (!element) {
				return { valid: false, message: "Element not found" };
			}
			
			const isInput = element.tagName === 'INPUT' || element.tagName === 'TEXTAREA';
			if (!isInput) {
				return { valid: false, message: "Element is not an input or textarea" };
			}
			
			// Focus the element
			element.focus();
			
			return { valid: true };
		})()
	`, s.selector,
	)

	validationResult, err := s.page.Evaluate(validationScript)
	if err != nil {
		return fmt.Errorf("error validating input element: %w", err)
	}

	// If the element is not valid, return an error
	if !validationResult.BoolValueOr(true) {
		return fmt.Errorf("element with selector %s is not a valid input element", s.selector)
	}

	// Small delay after focusing
	time.Sleep(50 * time.Millisecond)

	// Clear the existing value first
	clearScript := fmt.Sprintf(
		`
		(() => {
			const element = document.querySelector(%q);
			if (!element) {
				return false;
			}
			
			// Clear existing value
			element.value = '';
			
			// Trigger an input event to notify any listeners
			const inputEvent = new Event('input', {
				bubbles: true,
				cancelable: true
			});
			element.dispatchEvent(inputEvent);
			
			return true;
		})()
	`, s.selector,
	)

	clearResult, err := s.page.Evaluate(clearScript)
	if err != nil {
		return fmt.Errorf("error clearing input: %w", err)
	}

	if !clearResult.BoolValueOr(true) {
		return fmt.Errorf("failed to clear input with selector %s", s.selector)
	}

	// Input the new text
	inputScript := fmt.Sprintf(`
	(async () => {
	  const element = document.querySelector(%q);
	  if (!element) return false;
	
	  // === Utility functions ===
	  function sleep(ms) { return new Promise(r => setTimeout(r, ms)); }
	
	  // === Mouse movement simulation ===
	  async function moveMouseHumanlyTo(x, y) {
		const steps = 25 + Math.floor(Math.random() * 20); // 25–45 small movements
		let mouse = window._mouse || { x: 0, y: 0 };
		window._mouse = mouse;
	
		const dx = (x - mouse.x) / steps;
		const dy = (y - mouse.y) / steps;
	
		for (let i = 0; i < steps; i++) {
		  const jx = (Math.random() - 0.5) * 3;
		  const jy = (Math.random() - 0.5) * 3;
		  mouse.x += dx + jx;
		  mouse.y += dy + jy;
	
		  document.dispatchEvent(new MouseEvent('mousemove', {
			clientX: mouse.x,
			clientY: mouse.y,
			bubbles: true
		  }));
	
		  await sleep(4 + Math.random() * 14);
		}
	
		mouse.x = x;
		mouse.y = y;
		document.dispatchEvent(new MouseEvent('mousemove', { clientX: x, clientY: y, bubbles: true }));
	  }
	
	  // === Focus and click ===
	  const rect = element.getBoundingClientRect();
	  const targetX = rect.left + rect.width / 2 + (Math.random() - 0.5) * 10;
	  const targetY = rect.top + rect.height / 2 + (Math.random() - 0.5) * 6;
	
	  await moveMouseHumanlyTo(targetX, targetY);
	  await sleep(80 + Math.random() * 120);
	
	  document.dispatchEvent(new MouseEvent('mousedown', { clientX: targetX, clientY: targetY, bubbles: true }));
	  document.dispatchEvent(new MouseEvent('mouseup', { clientX: targetX, clientY: targetY, bubbles: true }));
	  element.focus();
	  element.dispatchEvent(new MouseEvent('click', { clientX: targetX, clientY: targetY, bubbles: true }));
	
	  await sleep(150 + Math.random() * 200);
	
	  // === Typing simulation ===
	  const text = %q;
	  const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, "value")?.set ||
					 Object.getOwnPropertyDescriptor(window.HTMLTextAreaElement.prototype, "value")?.set;
	
	  function getHumanDelay(char) {
		let base = 40 + Math.random() * 120;
		if (" .,\\n".includes(char)) base += 100 + Math.random() * 200;
		if (char === char.toUpperCase() && char !== char.toLowerCase()) base += 40;
		if (Math.random() < 0.1) base += 300 + Math.random() * 500;
		return base;
	  }
	
	  for (let i = 0; i < text.length; i++) {
		const char = text[i];
		const keyCode = char.charCodeAt(0);
	
		element.dispatchEvent(new KeyboardEvent("keydown", { key: char, code: char, keyCode, charCode: keyCode, bubbles: true }));
		element.dispatchEvent(new KeyboardEvent("keypress", { key: char, code: char, keyCode, charCode: keyCode, bubbles: true }));
	
		setter.call(element, (element.value || "") + char);
		element.dispatchEvent(new Event("input", { bubbles: true }));
		element.dispatchEvent(new KeyboardEvent("keyup", { key: char, code: char, keyCode, charCode: keyCode, bubbles: true }));
	
		await sleep(getHumanDelay(char));
	
		// occasional backspace correction (1–3%)
		if (Math.random() < 0.03 && element.value.length > 2) {
		  element.dispatchEvent(new KeyboardEvent("keydown", { key: "Backspace", keyCode: 8, bubbles: true }));
		  setter.call(element, element.value.slice(0, -1));
		  element.dispatchEvent(new Event("input", { bubbles: true }));
		  element.dispatchEvent(new KeyboardEvent("keyup", { key: "Backspace", keyCode: 8, bubbles: true }));
		  await sleep(150 + Math.random() * 200);
	
		  // retype same char after correction
		  setter.call(element, element.value + char);
		  element.dispatchEvent(new Event("input", { bubbles: true }));
		  await sleep(80 + Math.random() * 180);
		}
	  }
	
	  setTimeout(() => {
		element.dispatchEvent(new Event("blur", { bubbles: true }));
		element.dispatchEvent(new Event("change", { bubbles: true }));
	  }, 200 + Math.random() * 300);
	
	  return true;
	})();
	`, s.selector, text)

	inputResult, err := s.page.EvaluateAsync(inputScript)
	if err != nil {
		return fmt.Errorf("error inputting text: %w", err)
	}

	if !inputResult.BoolValueOrDefault() {
		return fmt.Errorf("failed to input text into element with selector %s", s.selector)
	}

	s.page.handleVerbose(fmt.Sprintf("successfully input text into element with selector: %s", s.selector))
	return nil
}

// Input types text into the element inside the iframe matching the selector
func (fs *FrameSelector) Input(text string) error {
	fs.page.handleVerbose(fmt.Sprintf("inputting text into element with selector: %s inside iframe: %s", fs.selector, fs.iframeSelector))

	// Validate the element exists and is an input/textarea
	validationScript := fmt.Sprintf(
		`
		(() => {
			const iframe = document.querySelector(%q);
			if (!iframe || !iframe.contentWindow) return { valid: false, message: "iframe not accessible" };
			const doc = iframe.contentDocument || iframe.contentWindow.document;
			const element = doc.querySelector(%q);
			if (!element) return { valid: false, message: "element not found" };
			const isInput = element.tagName === 'INPUT' || element.tagName === 'TEXTAREA';
			if (!isInput) return { valid: false, message: "element not input/textarea" };
			element.focus();
			return { valid: true };
		})()
	`, fs.iframeSelector, fs.selector,
	)

	validationResult, err := fs.page.Evaluate(validationScript)
	if err != nil {
		return fmt.Errorf("error validating input element in iframe: %w", err)
	}
	if !validationResult.BoolValueOr(true) {
		return fmt.Errorf("element %s is not a valid input inside iframe %s", fs.selector, fs.iframeSelector)
	}

	// Clear and type text with events
	inputScript := fmt.Sprintf(
		`
		(async () => {
			const iframe = document.querySelector(%q);
			if (!iframe || !iframe.contentWindow) return false;
			const doc = iframe.contentDocument || iframe.contentWindow.document;
			const element = doc.querySelector(%q);
			if (!element) return false;

			// Clear existing value
			element.value = '';
			element.dispatchEvent(new Event('input', { bubbles: true }));

			const text = %q;
			let i = 0;
			const nativeInputValueSetter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set;

			return new Promise((resolve) => {
				function typeChar() {
					if (i >= text.length) {
						element.dispatchEvent(new Event('blur', { bubbles: true }));
						element.dispatchEvent(new Event('change', { bubbles: true }));
						resolve(true);
						return;
					}
					const char = text[i];
					const keyCode = char.charCodeAt(0);
					element.dispatchEvent(new KeyboardEvent('keydown', { key: char, code: char, charCode: keyCode, keyCode, bubbles: true }));
					element.dispatchEvent(new KeyboardEvent('keypress', { key: char, code: char, charCode: keyCode, keyCode, bubbles: true }));
					nativeInputValueSetter.call(element, (element.value || '') + char);
					element.dispatchEvent(new Event('input', { bubbles: true }));
					element.dispatchEvent(new KeyboardEvent('keyup', { key: char, code: char, charCode: keyCode, keyCode, bubbles: true }));
					i++;
					setTimeout(typeChar, 50 + Math.random() * 100);
				}
				typeChar();
			});
		})()
	`, fs.iframeSelector, fs.selector, "%s")

	// Fill in the text argument placeholder safely
	inputScript = fmt.Sprintf(inputScript, text)

	inputResult, err := fs.page.EvaluateAsync(inputScript)
	if err != nil {
		return fmt.Errorf("error inputting text in iframe: %w", err)
	}
	if !inputResult.BoolValueOrDefault() {
		return fmt.Errorf("failed to input text into element %s in iframe %s", fs.selector, fs.iframeSelector)
	}

	fs.page.handleVerbose(fmt.Sprintf("successfully input text into element: %s inside iframe: %s", fs.selector, fs.iframeSelector))
	return nil
}

// WaitForSelector waits for the selector to appear in the DOM
func (s *Selector) WaitForSelector(timeout time.Duration) error {
	s.page.handleVerbose(fmt.Sprintf("waiting for selector: %s", s.selector))

	startTime := time.Now()
	for {
		// Check if the timeout has been exceeded
		if time.Since(startTime) > timeout {
			return fmt.Errorf("timeout waiting for selector %s", s.selector)
		}

		// Check if the element exists
		existsScript := fmt.Sprintf(
			`
			document.querySelector(%q) !== null
		`, s.selector,
		)

		result, err := s.page.Evaluate(existsScript)
		if err != nil {
			return fmt.Errorf("error checking for selector: %w", err)
		}

		if result.BoolValueOrDefault() {
			s.page.handleVerbose(fmt.Sprintf("selector found: %s", s.selector))
			return nil
		}

		// Wait a bit before checking again
		time.Sleep(100 * time.Millisecond)
	}
}

// Screenshot takes a screenshot of the element matching the selector
// Returns the screenshot as bytes in PNG format
func (s *Selector) Screenshot() ([]byte, error) {
	s.page.handleVerbose(fmt.Sprintf("taking screenshot of element with selector: %s", s.selector))

	// First, get the bounding box of the element
	boundingBoxScript := fmt.Sprintf(
		`
		(() => {
			const element = document.querySelector(%q);
			if (!element) {
				return null;
			}
			
			const rect = element.getBoundingClientRect();
			const scrollX = window.pageXOffset || window.scrollX || 0;
			const scrollY = window.pageYOffset || window.scrollY || 0;
			
			return {
				x: rect.left + scrollX,
				y: rect.top + scrollY,
				width: rect.width,
				height: rect.height
			};
		})()
	`, s.selector,
	)

	result, err := s.page.Evaluate(boundingBoxScript)
	if err != nil {
		return nil, fmt.Errorf("error getting element bounding box: %w", err)
	}

	if result.Value == nil {
		return nil, fmt.Errorf("element with selector %s not found", s.selector)
	}

	// Parse the bounding box from the response
	resultMap, ok := result.Value.(map[string]any)
	if !ok {
		// Try to parse from string if it's returned as JSON string
		if resultStr := result.StringValue(); resultStr != "" && resultStr != "null" {
			// The result might be a JSON string, let's try to parse it
			return nil, fmt.Errorf("element with selector %s not found or bounding box could not be determined", s.selector)
		}
		return nil, fmt.Errorf("element with selector %s not found", s.selector)
	}

	var x, y, width, height float64
	var okX, okY, okW, okH bool

	if xVal, exists := resultMap["x"]; exists {
		x, okX = xVal.(float64)
		if !okX {
			return nil, fmt.Errorf("invalid x coordinate in bounding box")
		}
	} else {
		return nil, fmt.Errorf("missing x coordinate in bounding box")
	}

	if yVal, exists := resultMap["y"]; exists {
		y, okY = yVal.(float64)
		if !okY {
			return nil, fmt.Errorf("invalid y coordinate in bounding box")
		}
	} else {
		return nil, fmt.Errorf("missing y coordinate in bounding box")
	}

	if wVal, exists := resultMap["width"]; exists {
		width, okW = wVal.(float64)
		if !okW {
			return nil, fmt.Errorf("invalid width in bounding box")
		}
	} else {
		return nil, fmt.Errorf("missing width in bounding box")
	}

	if hVal, exists := resultMap["height"]; exists {
		height, okH = hVal.(float64)
		if !okH {
			return nil, fmt.Errorf("invalid height in bounding box")
		}
	} else {
		return nil, fmt.Errorf("missing height in bounding box")
	}

	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("element with selector %s has invalid dimensions (width: %f, height: %f)", s.selector, width, height)
	}

	// Scroll element into view to ensure it's visible
	scrollScript := fmt.Sprintf(
		`(() => {
			const element = document.querySelector(%q);
			if (element) {
				element.scrollIntoView({ behavior: "auto", block: "center" });
				return true;
			}
			return false;
		})()`, s.selector,
	)

	_, err = s.page.Evaluate(scrollScript)
	if err != nil {
		return nil, fmt.Errorf("error scrolling element into view: %w", err)
	}

	// Small delay to ensure element is in view after scrolling
	time.Sleep(100 * time.Millisecond)

	// Capture screenshot using Chrome DevTools Protocol
	// Page.captureScreenshot with clip parameter
	command := s.page.NewCommand("Page.captureScreenshot", map[string]any{
		"clip": map[string]any{
			"x":      x,
			"y":      y,
			"width":  width,
			"height": height,
			"scale":  1.0,
		},
	})

	response, err := s.page.sendAndReceive(command)
	if err != nil {
		return nil, fmt.Errorf("error capturing screenshot: %w", err)
	}

	// Extract the base64 image data
	if response.Value == nil {
		return nil, fmt.Errorf("screenshot response is empty")
	}

	var screenshotData string
	if resultMap, ok := response.Value.(map[string]any); ok {
		if data, exists := resultMap["data"]; exists {
			screenshotData, ok = data.(string)
			if !ok {
				return nil, fmt.Errorf("invalid screenshot data format")
			}
		} else {
			return nil, fmt.Errorf("missing data field in screenshot response")
		}
	} else if resultStr, ok := response.Value.(string); ok {
		// Try parsing as JSON string if needed
		var resultMap map[string]any
		if err := json.Unmarshal([]byte(resultStr), &resultMap); err == nil {
			if data, exists := resultMap["data"]; exists {
				screenshotData, ok = data.(string)
				if !ok {
					return nil, fmt.Errorf("invalid screenshot data format")
				}
			} else {
				return nil, fmt.Errorf("missing data field in screenshot response")
			}
		} else {
			// Response value might be the base64 string directly
			screenshotData = resultStr
		}
	} else {
		return nil, fmt.Errorf("invalid screenshot response format")
	}

	// Decode base64 to bytes
	imageBytes, err := base64.StdEncoding.DecodeString(screenshotData)
	if err != nil {
		return nil, fmt.Errorf("error decoding base64 screenshot: %w", err)
	}

	s.page.handleVerbose(fmt.Sprintf("successfully took screenshot of element with selector: %s (size: %d bytes)", s.selector, len(imageBytes)))
	return imageBytes, nil
}
