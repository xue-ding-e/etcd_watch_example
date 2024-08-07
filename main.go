package main

import (
	"context"
	"fmt"
	"github.com/spf13/viper"
	v3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"log"
	"sync"
	"time"
)

var lockTimeout time.Duration
var client *v3.Client
var db *gorm.DB

func main() {
	v := viper.New()
	v.SetConfigFile("config.yaml")
	v.SetConfigType("yaml")

	err := v.ReadInConfig()
	if err != nil {
		panic(fmt.Errorf("fatal error config file: %w", err))
	}
	if err = v.Unmarshal(&global_config); err != nil {
		panic(err)
	}

	// 从配置文件中获取数据库连接字符串
	dsn := global_config.Database.DSN

	// 从配置文件中获取etcd配置信息
	etcdEndpoints := global_config.Etcd.Endpoints
	// 转换字符串为time.Duration
	etcdDialTimeout, err := time.ParseDuration(global_config.Etcd.DialTimeout)
	if err != nil {
		panic(fmt.Errorf("invalid dial timeout: %w", err))
	}
	lockTimeout, err = time.ParseDuration(global_config.Etcd.LockTimeout)
	if err != nil {
		panic(fmt.Errorf("invalid lock timeout: %w", err))
	}
	// 建立全局连接 和设置过期时间
	config := v3.Config{
		Endpoints:   etcdEndpoints,
		DialTimeout: etcdDialTimeout, // 初次连接etcd的超时时间
	}
	client, err = v3.New(config)
	if err != nil {
		fmt.Println("new client error")
		return
	}

	// 连接到MySQL服务器
	sqlDB, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		panic("failed to connect to MySQL server")
	}

	// 检查数据库是否存在，不存在则创建
	createDBIfNotExist(sqlDB, dsn)

	// 连接到MySQL数据库
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		panic("failed to connect to database")
	}
	db.AutoMigrate(&Product{})

	/*

		业务测试示例
		productID加锁id  通常为商品id
		模拟测试 并发量totalTasks
	*/
	productID := "1"
	totalTasks := 500

	// 调用封装的业务逻辑函数
	var wg sync.WaitGroup
	for i := 0; i < totalTasks; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := processTask(productID); err != nil {
				fmt.Printf("Task %d failed: %v\n", i, err)
			}
		}(i)
	}
	wg.Wait()
	fmt.Println("over...")
}

func processTask(productID string) error {
	// 获取session会话 (内部自动开启一个goroutine自动续约和维持心跳)
	session, err := concurrency.NewSession(client)
	if err != nil {
		return fmt.Errorf("new session error: %w", err)
	}
	defer session.Close()

	Locker := concurrency.NewMutex(session, "product_lock_"+productID)
	// 创建一个带有超时时间的上下文
	ctx, cancel := context.WithTimeout(context.Background(), lockTimeout) // 每次请求获取锁的超时时间
	defer cancel()

	// 使用带有超时时间的上下文来尝试获取锁
	if err := Locker.Lock(ctx); err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	defer Locker.Unlock(ctx)

	// 开始一个数据库事务
	tx := db.Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}
	// 从数据库中读取库存数量
	var product Product
	result := tx.First(&product, "id = ?", productID)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			product = Product{ID: productID, Stock: 100} // 初始化库存为100
			tx.Create(&product)
			if tx.Error != nil {
				tx.Rollback()
				return fmt.Errorf("failed to create product: %w", tx.Error)
			}
		} else {
			tx.Rollback()
			return fmt.Errorf("failed to get stock: %w", result.Error)
		}
	}

	if product.Stock > 0 {
		// 执行购买操作
		result = tx.Model(&product).Update("stock", gorm.Expr("stock - ?", 1))
		if result.Error != nil {
			tx.Rollback()
			return fmt.Errorf("failed to update stock: %w", result.Error)
		}

		// 提交事务
		tx.Commit()
		if tx.Error != nil {
			tx.Rollback()
			return fmt.Errorf("failed to commit transaction: %w", tx.Error)
		}
	} else {
		tx.Rollback()
		return fmt.Errorf("insufficient stock")
	}

	return nil
}

func createDBIfNotExist(db *gorm.DB, dsn string) {
	// 提取数据库名
	dbName := extractDBName(dsn)
	query := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s;", dbName)
	if err := db.Exec(query).Error; err != nil {
		log.Fatalf("failed to create database %s: %v", dbName, err)
	}
}

func extractDBName(dsn string) string {
	// 从dsn中提取数据库名的简单实现
	// 假设dsn格式为 "user:password@tcp(host:port)/dbname?params"
	var dbName string
	_, err := fmt.Sscanf(dsn, "%*[^/]/%s?%*s", &dbName)
	if err != nil {
		log.Fatalf("failed to parse dbname from dsn: %v", err)
	}
	return dbName
}
