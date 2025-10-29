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

// Click clicks on the element matching the selector
func (s *Selector) Click() error {
	s.page.handleVerbose(fmt.Sprintf("clicking on element with selector: %s", s.selector))

	// First ensure the element is visible and clickable
	visibilityScript := fmt.Sprintf(
		`
		(() => {
			const element = document.querySelector(%q);
			if (!element) {
				return false;
			}
			
			const rect = element.getBoundingClientRect();
			const isVisible = !!(rect.width && rect.height && element.offsetParent !== null);
			
			if (!isVisible) {
				return false;
			}
			
			// Scroll element into view if needed
			element.scrollIntoView({ behavior: "auto", block: "center" });
			
			return true;
		})()
	`, s.selector,
	)

	visibilityResult, err := s.page.Evaluate(visibilityScript)
	if err != nil {
		return fmt.Errorf("error checking element visibility: %w", err)
	}

	// If the element is not visible, return an error
	if !visibilityResult.BoolValueOrDefault() {
		return fmt.Errorf("element with selector %s is not visible or not found", s.selector)
	}

	// Small delay to ensure element is properly in view after scrolling
	time.Sleep(100 * time.Millisecond)

	// Click on the element
	clickScript := fmt.Sprintf(
		`
		(() => {
			const element = document.querySelector(%q);
			if (!element) {
				return false;
			}
			
			// Simulate a real click
			const clickEvent = new MouseEvent('click', {
				bubbles: true,
				cancelable: true,
				view: window
			});
			
			element.dispatchEvent(clickEvent);
			return true;
		})()
	`, s.selector,
	)

	clickResult, err := s.page.Evaluate(clickScript)
	if err != nil {
		return fmt.Errorf("error clicking element: %w", err)
	}

	if !clickResult.BoolValueOrDefault() {
		return fmt.Errorf("failed to click on element with selector %s", s.selector)
	}

	s.page.handleVerbose(fmt.Sprintf("successfully clicked on element with selector: %s", s.selector))
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
	  if (!element) {
		return false;
	  }
	
	  const text = %q;
	  let i = 0;
	
	  const nativeInputValueSetter = Object.getOwnPropertyDescriptor(
		window.HTMLInputElement.prototype,
		"value"
	  ).set;
	
	  return new Promise((resolve) => {
		function typeChar() {
		  if (i >= text.length) {
			element.dispatchEvent(new Event("blur", { bubbles: true }));
			element.dispatchEvent(new Event("change", { bubbles: true }));
			resolve(true); // ✅ finished typing
			return;
		  }
	
		  const char = text[i];
		  const keyCode = char.charCodeAt(0);
	
		  element.dispatchEvent(new KeyboardEvent("keydown", { key: char, code: char, charCode: keyCode, keyCode, bubbles: true }));
		  element.dispatchEvent(new KeyboardEvent("keypress", { key: char, code: char, charCode: keyCode, keyCode, bubbles: true }));
	
		  nativeInputValueSetter.call(element, (element.value || "") + char);
	
		  element.dispatchEvent(new Event("input", { bubbles: true }));
		  element.dispatchEvent(new KeyboardEvent("keyup", { key: char, code: char, charCode: keyCode, keyCode, bubbles: true }));
	
		  i++;
		  setTimeout(typeChar, 50 + Math.random() * 100);
		}
	
		typeChar();
	  });
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
