package main

import (
	"fmt"
	v3 "go.etcd.io/etcd/client/v3"
	"ql_learnEtcd/concurrency"
	"time"
)

func main() {
	cfg := v3.Config{
		Endpoints:   []string{"<your_etcd_server_ip>:<your_etcd_server_port>"},
		DialTimeout: 5 * time.Second,
	}
	client, err := v3.New(cfg)
	if err != nil {
		// handle error
	}

	session, err := concurrency.NewSession(client)
	if err != nil {
		// handle error
	}
	fmt.Println(session)
}
