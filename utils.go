package main

import (
	"flag"
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

func CheckFlags() {
	flag.StringVar(&ETCD_HOSTS, "etcd-hosts", ETCD_HOSTS, "Specifies the etcd hosts")
	flag.Parse()
}
