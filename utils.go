package main

import (
	"fmt"
	"os"
)

var (
	ETCD_HOSTS = os.Getenv("ETCD_HOSTS")
)

func GetEnvOrDefault(env, defaultVal string) string {
	e := os.Getenv(env)
	fmt.Printf("'%s'", e)
	if e == "" {
		return defaultVal
	} else {
		return e
	}
}
