package chrome

import (
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
)

// create a session with a folder name
func (o *Options) CreateSession(name string) error {
	name = strings.ToLower(name)
	sessionPath := path.Join("o.GetProfilesPath()", fmt.Sprintf("profile-%s", name))
	if _, err := os.Stat(sessionPath); os.IsNotExist(err) {
		if err := os.Mkdir(sessionPath, os.ModePerm); err != nil {
			return err
		} else {
			return nil
		}
	} else {
		return errors.New("session already exists")
	}
}

// create a session with a folder name
func (o *Options) DeleteSession(name string) error {
	name = strings.ToLower(name)
	sessionPath := path.Join("o.GetProfilesPath()", fmt.Sprintf("profile-%s", name))
	if err := os.RemoveAll(sessionPath); os.IsNotExist(err) {
		return err
	}
	return nil
}

// get a session string to pass through the args settings
func (o *Options) GetSessionName(name string) (string, error) {
	name = strings.ToLower(name)
	sessionPath := path.Join("o.GetProfilesPath()", fmt.Sprintf("profile-%s", o.GetUser()))
	if _, err := os.Stat(sessionPath); os.IsNotExist(err) {
		return sessionPath, o.CreateSession(name)
	} else {
		return sessionPath, nil
	}
}
