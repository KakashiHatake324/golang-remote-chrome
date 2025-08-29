package chrome

import (
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
	inputScript := fmt.Sprintf(
		`
		(() => {
			const element = document.querySelector(%q);
			if (!element) {
				return false;
			}
			
			// Set the value
			element.value = %q;
			
			// Trigger input events
			const inputEvent = new Event('input', {
				bubbles: true,
				cancelable: true
			});
			element.dispatchEvent(inputEvent);
			
			const changeEvent = new Event('change', {
				bubbles: true,
				cancelable: true
			});
			element.dispatchEvent(changeEvent);
			
			return true;
		})()
	`, s.selector, text,
	)

	inputResult, err := s.page.Evaluate(inputScript)
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
