package util

import (
	redigo "github.com/gomodule/redigo/redis"
	"log"
	"strconv"
	"time"
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
		IdleTimeout: 240 * time.Second,
	}
}

func GetSymbolConsumeData(conn redigo.Conn, symbol string,uid int) (string,string,error)  {

	groups,_ := conn.Do("xinfo","groups",symbol)
	groups_name := symbol+"_groups"
	if len(groups.([]interface{}))==0{
		//创建消费组
		conn.Do("xgroup","create",symbol,groups_name,0)
		//log.Printf("创建消费组 %v",groups_name)
	}else{
		var P1 struct{
			Name string `redis:"name"`
			Consumers int `redis:"consumers"`
			Pending int `redis:"pending"`
			Ldi string `redis:"last-delivered-id"`
		}
		flag := false
		for _,v := range groups.([]interface{}){
			vv, err := redigo.Values(v,nil)
			if err != nil {
				log.Println(err)
			}
			if err := redigo.ScanStruct(vv, &P1); err != nil {
				log.Println(err)
			}
			if P1.Name==groups_name{
				flag = true
			}
		}
		if !flag{
			//创建消费组
			conn.Do("xgroup","create",symbol,groups_name,0)
		}
	}

	//拉取信息到groups_user用户消费
	groups_user := "u" + strconv.Itoa(uid)
	conn.Do("xreadgroup","group",groups_name, groups_user,"count","2", "streams",symbol, ">")
	return groups_name,groups_user,nil
}
