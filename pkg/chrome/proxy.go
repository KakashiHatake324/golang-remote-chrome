package chrome

import (
	"errors"
	"strings"
)

// formatProxy formats the proxy string
func formatProxy(proxyString string) (string, string, string, error) {
	if strings.Contains(proxyString, "http://") {
		proxyString = strings.ReplaceAll(proxyString, "http://", "")
		splitIt := strings.Split(proxyString, "@")
		if len(splitIt) == 2 {
			splitIt := strings.Split(proxyString, "@")
			splitAuth := strings.Split(splitIt[0], ":")
			proxyUser := splitAuth[0]
			proxyPass := splitAuth[1]
			return splitIt[1], proxyUser, proxyPass, nil
		}
		return proxyString, "", "", nil
	}
	var proxy string
	var proxyUser string
	var proxyPass string
	proxySplit := strings.Split(proxyString, ":")
	if len(proxySplit) == 4 {
		proxy = "http://" + proxySplit[0] + ":" + proxySplit[1]
		proxyUser = proxySplit[2]
		proxyPass = proxySplit[3]
	} else if len(proxySplit) == 2 {
		proxy = "http://" + proxySplit[0] + ":" + proxySplit[1]
	} else {
		return "", "", "", errors.New("error formating proxies")
	}
	return proxy, proxyUser, proxyPass, nil
}
