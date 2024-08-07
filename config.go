package main

import (
	"gorm.io/gorm"
	"time"
)

type Etcd struct {
	Endpoints   []string      `mapstructure:"endpoints" json:"endpoints" yaml:"endpoints"`
	DialTimeout time.Duration `mapstructure:"dial_timeout" json:"dial_timeout" yaml:"dial_timeout"`
	LockTimeout time.Duration `mapstructure:"lock_timeout" json:"lock_timeout" yaml:"lock_timeout"`
	Username    string        `yaml:"username" mapstructure:"username"`
	Password    string        `yaml:"password" mapstructure:"password"`
}

type Config struct {
	Etcd     Etcd     `mapstructure:"etcd" json:"etcd" yaml:"etcd"`
	Database Database `mapstructure:"database" json:"database" yaml:"database"`
}

type Database struct {
	DSN string `mapstructure:"dsn" json:"dsn" yaml:"dsn"`
}
type Product struct {
	ID    string `gorm:"type:varchar(100);primary_key"`
	Stock int    `gorm:"type:int"`
	gorm.Model
}

var global_config Config
