package main

import (
	"log"
	"os"
	"websocket/socket_server"
	"websocket/util"
	"websocket/config"
)
func main() {
	util.RedisPoolInit()
	defer util.Redis_pool.Close()

	logFile, err := os.OpenFile("./a.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0766)
	if err != nil {
		log.Panic("打开日志文件异常")
	}
	defer logFile.Close()
	//log.New(logFile, "socket \t", log.Ldate|log.Ltime|log.Lshortfile)
	log.SetOutput(logFile) // 将文件设置为log输出的文件
	log.SetPrefix("[logTool]")
	log.SetFlags(log.Ldate|log.Ltime|log.Lshortfile)
	log.Print("asd")

	socket_server.StartWebsocket(config.GlobalConfig.Websocket_host +":"+ config.GlobalConfig.Websocket_port)


}
