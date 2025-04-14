package internals

import (
	"archive/zip"
	"embed"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

//go:embed Default.zip
var defaultProfile embed.FS

// DeleteProfileFolder deletes a folder and all its contents
func DeleteProfileFolder(folderPath string) error {
	go func() error {
		for range 10 {
			time.Sleep(10 * time.Second)
			// Ensure the path is absolute and clean
			absPath, err := filepath.Abs(folderPath)
			if err != nil {
				log.Println("failed to get absolute path", absPath, err)
				return fmt.Errorf("failed to get absolute path: %v", err)
			}

			// Check if the path exists and is a directory
			info, err := os.Stat(absPath)
			if err != nil {
				if os.IsNotExist(err) {
					log.Println("Folder does not exist", absPath, err)
					return nil // Folder doesn't exist, consider it successfully deleted
				}
				return fmt.Errorf("failed to check folder: %v", err)
			}
			if !info.IsDir() {
				log.Println("path is not a directory:", absPath)
				return fmt.Errorf("path is not a directory: %s", absPath)
			}

			// Remove the directory and all its contents
			if err := os.RemoveAll(absPath); err != nil {
				log.Println("failed to remove directory:", absPath, err)
				return fmt.Errorf("failed to remove directory: %v", err)
			}
		}
		return nil
	}()
	return nil
}

// UnzipDefaultProfile extracts the embedded Default.zip to the specified destination folder
func UnzipDefaultProfile(destFolder string) error {
	// Read the embedded zip file
	zipData, err := defaultProfile.ReadFile("Default.zip")
	if err != nil {
		return fmt.Errorf("failed to read embedded Default.zip: %v", err)
	}

	// Create a temporary file to store the zip data
	tmpFile, err := os.CreateTemp("", "chrome-profile-*.zip")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %v", err)
	}
	defer os.Remove(tmpFile.Name()) // Clean up the temporary file

	// Write the zip data to the temporary file
	if _, err := tmpFile.Write(zipData); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write zip data to temporary file: %v", err)
	}
	tmpFile.Close()

	// Open the zip file for reading
	reader, err := zip.OpenReader(tmpFile.Name())
	if err != nil {
		return fmt.Errorf("failed to open zip file: %v", err)
	}
	defer reader.Close()

	// Create the destination folder
	if err := os.MkdirAll(destFolder, 0755); err != nil {
		return fmt.Errorf("failed to create destination folder: %v", err)
	}

	// Extract each file from the zip
	for _, file := range reader.File {
		// Construct the full path for the file
		filePath := filepath.Join(destFolder, file.Name)

		// Check for path traversal
		cleanDest := filepath.Clean(destFolder)
		cleanPath := filepath.Clean(filePath)
		if !strings.HasPrefix(cleanPath, cleanDest+string(os.PathSeparator)) {
			return fmt.Errorf("invalid file path: %s", file.Name)
		}

		if file.FileInfo().IsDir() {
			// Create directory
			if err := os.MkdirAll(filePath, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %v", filePath, err)
			}
			continue
		}

		// Create the file
		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			return fmt.Errorf("failed to create directory for file %s: %v", filePath, err)
		}

		destFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
		if err != nil {
			return fmt.Errorf("failed to create file %s: %v", filePath, err)
		}

		// Open the source file in the zip
		srcFile, err := file.Open()
		if err != nil {
			destFile.Close()
			return fmt.Errorf("failed to open zip file %s: %v", file.Name, err)
		}

		// Copy the contents
		if _, err := io.Copy(destFile, srcFile); err != nil {
			destFile.Close()
			srcFile.Close()
			return fmt.Errorf("failed to copy file %s: %v", file.Name, err)
		}

		destFile.Close()
		srcFile.Close()
	}

	return nil
}
