package config
import (
	"fmt"
	"github.com/spf13/viper"
	"log"
	"github.com/fsnotify/fsnotify"
)
type globalConfig struct {
	Websocket_host string
	Websocket_port string
	Redis_host string
	Redis_port string
	Redis_pass string
	Mysql_host string
	Mysql_port string
	Mysql_user string
	Mysql_pass string
	Mysql_db string
	Api_addr string
}
var GlobalConfig *globalConfig
func init(){
	v := viper.New()
	v.SetConfigName("config")
	v.AddConfigPath("./config")
	v.SetConfigType("json")
	if err := v.ReadInConfig();err != nil {
		log.Printf("err:%s\n",err)
	}

	GlobalConfig = &globalConfig{}
	if err := v.Unmarshal(GlobalConfig) ; err != nil{
		log.Printf("err:%s\n",err)
	}
	v.OnConfigChange(func(e fsnotify.Event) {
		fmt.Printf("config is change :%s  %s\n", e.String(),e.Name)
		if err := v.ReadInConfig();err != nil {
			log.Printf("err:%s\n",err)
		}
		if err := v.Unmarshal(GlobalConfig) ; err != nil{
			log.Printf("err:%s\n",err)
		}
		fmt.Printf("up config %+v \n",GlobalConfig)
		//cancel()
	})
	v.WatchConfig()
}
