package main

import (
	"context"
	"fmt"
	"go.etcd.io/etcd/client/v3"
	"sync"
	"time"
)

func main() {
	var (
		config clientv3.Config
		client *clientv3.Client
		err    error
	)

	// 客户端配置
	config = clientv3.Config{
		Endpoints:   []string{"192.168.1.223:2379"},
		DialTimeout: 5 * time.Second,
	}

	// 建立连接
	if client, err = clientv3.New(config); err != nil {
		fmt.Println(err)
		return
	}

	// 创建一个WaitGroup来等待所有的goroutine完成
	var wg sync.WaitGroup

	// 模拟10个并发请求
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			acquireLock(client, fmt.Sprintf("goroutine-%d", i))
		}(i)
	}

	// 等待所有的goroutine完成
	wg.Wait()
}

func acquireLock(client *clientv3.Client, id string) {
	var (
		kv      = clientv3.NewKV(client)
		txn     = kv.Txn(context.Background())
		err     error
		resp    *clientv3.TxnResponse
		watcher = clientv3.NewWatcher(client)
	)

	// 尝试获取锁
	txn.If(clientv3.Compare(clientv3.CreateRevision("/cron/lock/job9"), "=", 0)).
		Then(clientv3.OpPut("/cron/lock/job9", id)).
		Else(clientv3.OpGet("/cron/lock/job9"))

	// 提交事务
	if resp, err = txn.Commit(); err != nil {
		fmt.Println(err)
		return
	}

	// 判断是否获取到锁
	if !resp.Succeeded {
		// 如果获取失败，获取当前的全局Revision
		rev := resp.Header.Revision
		// 监听大于当前Revision的修改
		watchChan := watcher.Watch(context.Background(), "/cron/lock/job9", clientv3.WithRev(rev+1))
		for watchResp := range watchChan {
			for _, event := range watchResp.Events {
				if event.Type == clientv3.EventTypeDelete {
					// 锁被释放，再次尝试获取
					acquireLock(client, id)
					return
				}
			}
		}
	} else {
		fmt.Printf("%s acquired the lock\n", id)

		// 模拟处理任务
		time.Sleep(1 * time.Second)

		// 释放锁
		if _, err = kv.Delete(context.Background(), "/cron/lock/job9"); err != nil {
			fmt.Println(err)
			return
		}

		fmt.Printf("%s released the lock\n", id)
	}
}
