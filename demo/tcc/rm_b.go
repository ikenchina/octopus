package main

import "fmt"

var (
	serviceB_listen = ":8088"
	serviceB_domain = fmt.Sprintf("http://localhost%s", serviceB_listen)
)

type RmServiceB struct {
	RmServiceA
}
