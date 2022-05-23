package util

import (
	"fmt"
	//_ "github.com/go-sql-driver/mysql"
	//"github.com/jmoiron/sqlx"
	//"log"
	//"time"
	"websocket/config"
)
//var DB *sqlx.DB
func initDB()(err error) {
	fmt.Printf("%+v",config.GlobalConfig)
	//dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8",
	//	config.GlobalConfig.Mysql_user,
	//	config.GlobalConfig.Mysql_pass,
	//	config.GlobalConfig.Mysql_host,
	//	config.GlobalConfig.Mysql_port,
	//	config.GlobalConfig.Mysql_db)
	//也可以使用MustConnect连接不成功就panic
	//DB,err = sqlx.Connect("mysql",dsn) // 这里可以校验是否连接上，本质调了一个Open和一个Ping
	//if err != nil {
	//	log.Printf("connect DB failed,err:%v\n",err)
	//	return err
	//}
	//DB.SetConnMaxLifetime(time.Second * 10)
	//DB.SetMaxOpenConns(20) // 设置与数据库建立连接的最大数目
	//DB.SetMaxIdleConns(10) // 设置连接池中的最大闲置连接数
	return
}
func init()  {
	initDB()
}
