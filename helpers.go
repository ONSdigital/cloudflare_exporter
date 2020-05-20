package main

import "time"

func timeOperation(f func() error) (time.Duration, error) {
	start := time.Now()
	err := f()
	return time.Since(start), err
}

func contains(list []string, str string) bool {
	for _, e := range list {
		if e == str {
			return true
		}
	}
	return false
}
