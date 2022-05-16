package main

import (
	"websocket/socket_server"
	"websocket/util"
	"websocket/config"
)
func main() {
	util.RedisPoolInit()
	socket_server.StartWebsocket(config.GlobalConfig.Websocket_host +":"+ config.GlobalConfig.Websocket_port)
	defer util.Redis_pool.Close()
}
