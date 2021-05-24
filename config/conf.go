package config

import (
	"fmt"
	"github.com/BurntSushi/toml"
	log "github.com/cihub/seelog"
)

/*
配置文件初始化
*/
type ExporterCfgData struct {
	Common struct {
		RegiServer     string `toml:"regiServer"`
		EnableRegister bool   `toml:"enableRegister"`
		Platform       bool   `toml:"platform"`
		AccessPath     string `toml:"accessPath"`
	} `toml:"common"`

	Alarm struct {
		FragmentSize int `toml:"fragmentSize"`
		DelayTimes   int `toml:"delayTimes"`
	} `toml:"alarm"`

	RuleStatus struct {
		Threshold       int      `toml:"threshold"`
		WhiteListStatus []string `toml:"whiteListStatus"`
		BlacklistStatus []string `toml:"blacklistStatus"`
	} `toml:"ruleStatus"`

	RuleResponse struct {
		ResponseTime      int      `toml:"responseTime"`
		WhiteListResponse []string `toml:"whiteListResponse"`
		BlacklistResponse []string `toml:"blacklistResponse"`
	} `toml:"ruleResponse"`

	Wechat1 struct {
		Wechatserver string `toml:"wechatserver"`
		Corpid1  string `toml:"corpid1"`
		Secret1  string `toml:"secret1"`
		Agentid1 int    `toml:"agentid1"`
		Name    string `toml:"name"`
		NginxIp string `toml:"nginxIp"`
	} `toml:"wechat1"`

	Wechat2 struct {
		Corpid2  string `toml:"corpid2"`
		Secret2  string `toml:"secret2"`
		Agentid2 int    `toml:"agentid2"`
	} `toml:"wechat2"`


	Wechat3 struct {
		Threshold3 int `toml:"threshold3"`
		Corpid3  string `toml:"corpid3"`
		Secret3  string `toml:"secret3"`
		Agentid3 int    `toml:"agentid3"`
	} `toml:"wechat3"`


	Gateway struct {
		Enable    bool   `toml:"enable"`
		ServerURL string `toml:"serverURL"`
	} `toml:"gateway"`
}

var Instance *ExporterCfgData

func IatClientCfg() *ExporterCfgData {
	if Instance == nil {
		Instance = new(ExporterCfgData)
	}
	return Instance
}

//初始化配置文件
func (p *ExporterCfgData) Init(up bool) (err error) {
	defer func() {
		if e := recover(); e != nil {
			fmt.Printf("conf_Init|param=%v;error=%v\n", up, e)
			log.Errorf("conf_Init|param=%v;error=%v", up, e)
		}
	}()
	if up {
		_, err = toml.DecodeFile("./conf.toml", Instance)
		if err != nil {
			log.Errorf("config_P_init|err:%v", err)
			fmt.Println("Config init error " + err.Error())
		}

	} else {
//		fmt.Println("配置有变动,进行更新")
		var InstanceNex = new(ExporterCfgData)
		_, err = toml.DecodeFile("./conf.toml", InstanceNex)
		if err != nil {
			log.Errorf("config_P_init|err:%v", err)
			fmt.Println("Config init error " + err.Error())
		}
		Instance = InstanceNex
	}
	return
}

//初始化配置文件及本地日志服务初始化
func init() {
	IatClientCfg().Init(true)
	logger, err := log.LoggerFromConfigAsFile("./log_client.xml")
	if err != nil {
		log.Errorf("config_init|err:%v", err)
		return
	}
	log.ReplaceLogger(logger)
	log.Flush()
	return
}
