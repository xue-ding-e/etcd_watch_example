package main

import (
	"context"
	"database/sql"
	"fmt"
	v3 "go.etcd.io/etcd/client/v3"
	"ql_learnEtcd/concurrency"
	"sync"
	"time"
)

func main() {
	var (
		config v3.Config
		client *v3.Client
		err    error
	)
	//建立全局连接 和设置过期时间
	config = v3.Config{
		Endpoints:   []string{"192.168.1.223:2379"},
		DialTimeout: 5 * time.Second,
	}
	client, err = v3.New(config)
	if err != nil {
		fmt.Println("new client error")
		return
	}

	// 连接到MySQL数据库
	db, err := sql.Open("mysql", "user:password@/dbname")
	if err != nil {
		fmt.Println("failed to connect to mysql:", err)
		return
	}
	defer db.Close()

	productID := "product123"
	count := 10
	var wg sync.WaitGroup
	wg.Add(count)
	for i := range count {
		go func() {
			defer wg.Done()

			fmt.Println(i)
			//获取session会话 (内部自动开启一个goroutine自动续约和维持心跳)
			session, err := concurrency.NewSession(client)
			if err != nil {
				fmt.Println("new session error")
				return
			}
			defer session.Close()

			Locker := concurrency.NewMutex(session, "product_lock_"+productID)
			// 创建一个带有超时时间的上下文
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			// 使用带有超时时间的上下文来尝试获取锁
			if err := Locker.TryLock(ctx); err != nil {
				fmt.Println("failed to acquire lock:", err)
				return
			}
			defer Locker.Unlock(ctx)

			//TODO 这里实现具体的业务逻辑
			// 开始一个数据库事务
			tx, err := db.Begin()
			if err != nil {
				fmt.Println("failed to begin transaction:", err)
				return
			}

			// 从数据库中读取库存数量
			var stock int
			err = tx.QueryRow("SELECT stock FROM products WHERE id = ?", productID).Scan(&stock)
			if err != nil {
				fmt.Println("failed to get stock:", err)
				tx.Rollback()
				return
			}

			if stock > 0 {
				// 执行购买操作
				_, err = tx.Exec("UPDATE products SET stock = stock - 1 WHERE id = ?", productID)
				if err != nil {
					fmt.Println("failed to update stock:", err)
					tx.Rollback()
					return
				}

				// 提交事务
				err = tx.Commit()
				if err != nil {
					fmt.Println("failed to commit transaction:", err)
					return
				}

				fmt.Println("successfully purchased")
			} else {
				fmt.Println("failed to purchase due to insufficient stock")
			}
			//TODO END

		}()

	}
	wg.Wait()
}
