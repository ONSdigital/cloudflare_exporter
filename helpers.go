package main

import "time"

func timeOperation(f func() error) (time.Duration, error) {
	start := time.Now()
	err := f()
	return time.Since(start), err
}

func keys(dict map[string]string) []string {
	var keys []string
	for key := range dict {
		keys = append(keys, key)
	}
	return keys
}
