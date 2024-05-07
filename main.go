package main

import (
	"context"
	"fmt"
	v3 "go.etcd.io/etcd/client/v3"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"ql_learnEtcd/concurrency"
	"sync"
	"time"
)

func main() {
	type Product struct {
		ID    string `gorm:"type:varchar(100);primary_key"`
		Stock int    `gorm:"type:int"`
		gorm.Model
	}
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
	dsn := "root:123456@tcp(192.168.1.223:13306)/etcd_goods_test?parseTime=true"
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		panic("failed to connect database")
	}
	db.AutoMigrate(&Product{})

	productID := "1"
	totalTasks := 10200
	concurrencyLimit := 50 // 设置并发限制为50
	taskChannel := make(chan int, concurrencyLimit)
	var wg sync.WaitGroup
	wg.Add(totalTasks)
	for i := range totalTasks {
		taskChannel <- 1 // 会阻塞，直到有空闲位置
		go func() {
			defer wg.Done()
			defer func() { <-taskChannel }() // 完成时释放位置
			//获取session会话 (内部自动开启一个goroutine自动续约和维持心跳)
			session, err := concurrency.NewSession(client)
			if err != nil {
				fmt.Println("new session error")
				return
			}
			defer session.Close()

			Locker := concurrency.NewMutex(session, "product_lock_"+productID)
			// 创建一个带有超时时间的上下文
			//ctx := context.Background()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			// 使用带有超时时间的上下文来尝试获取锁
			if err := Locker.Lock(ctx); err != nil {
				fmt.Println("failed to acquire lock:", err)
				return
			}
			defer Locker.Unlock(ctx)

			//TODO 这里实现具体的业务逻辑
			// 开始一个数据库事务
			fmt.Println(i)

			tx := db.Begin()
			if tx.Error != nil {
				fmt.Println("failed to begin transaction:", tx.Error)
				return
			}
			// 从数据库中读取库存数量
			var product Product
			result := tx.First(&product, "id = ?", productID)
			if result.Error != nil {
				fmt.Println("failed to get stock:", result.Error)
				tx.Rollback()
				return
			}

			if product.Stock > 0 {
				// 执行购买操作
				result = tx.Model(&product).Update("stock", gorm.Expr("stock - ?", 1))
				if result.Error != nil {
					fmt.Println("failed to update stock:", result.Error)
					tx.Rollback()
					return
				}

				// 提交事务
				tx.Commit()
				if tx.Error != nil {
					fmt.Println("failed to commit transaction:", tx.Error)
					tx.Rollback()
					return
				}
				//fmt.Println("successfully purchased")
			} else {
				//fmt.Println("failed to purchase due to insufficient stock")
				return
			}
			//TODO END

		}()

	}
	wg.Wait()
	fmt.Println("over...")
}
