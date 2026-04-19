package tasks

import "os"

func podName() string {
	if v := os.Getenv("POD_NAME"); v != "" {
		return v
	}
	host, err := os.Hostname()
	if err != nil {
		return ""
	}
	return host
}
