package util

import (
	redigo "github.com/gomodule/redigo/redis"
	"websocket/config"
)

var redispass string = config.GlobalConfig.Redis_pass
var redishost string = config.GlobalConfig.Redis_host+":"+config.GlobalConfig.Redis_port
var Redis_pool *redigo.Pool


func GetRedisPool() redigo.Conn{
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
