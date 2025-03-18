# golang-remote-chrome

A Go application for remote Chrome browser control.

## Project Structure

```
golang-remote-chrome/
├── cmd/
│   └── chrome/          # Main application entry point
├── pkg/                 # Public packages that can be used by external applications
├── internal/           # Private application and library code
└── go.mod             # Go module definition
```

## Prerequisites

- Go 1.21.5 or later
- Chrome browser installed

## Getting Started

1. Clone the repository:
   ```bash
   git clone https://github.com/yourusername/golang-remote-chrome.git
   cd golang-remote-chrome
   ```

2. Run the application:
   ```bash
   go run cmd/chrome/main.go
   ```

## Development

To add new dependencies:
```bash
go get github.com/username/package
```

## License

This project is licensed under the MIT License - see the LICENSE file for details. 