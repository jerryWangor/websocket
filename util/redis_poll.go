package main

import (
	redigo "github.com/gomodule/redigo/redis"
)

var redispass string = "crimoon2015"
var redishost string = "10.0.111.154:52311"
var Redis_pool *redigo.Pool


func GetPool() redigo.Conn{
	return Redis_pool.Get()
}
func RedisPoolInit(){
	Redis_pool = &redigo.Pool{
		// Other pool configuration not shown in this example.
		Dial: func () (redigo.Conn, error) {
			c, err := redigo.Dial("tcp", redishost)
			if err != nil {
				return nil, err
			}
			if _, err := c.Do("AUTH", redispass); err != nil {
				c.Close()
				return nil, err
			}
			return c, nil
		},
		MaxIdle:100,
	}
}
