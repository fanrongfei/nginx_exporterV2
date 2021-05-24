package register

import (
	log "github.com/cihub/seelog"
	"io/ioutil"
	"net/rpc"
	"nginx_exporterV2/config"
	"os"
	"strings"
	"time"
)

var (
	//配置中心 配置文件版本
	Version string
	//rpc 连接
	rpcClint *rpc.Client
)

/*
版本请求
Platform 平台，true 省平台，false 市平台
version  目前平台版本（其实文件的md5sum值）
新增自己平台信息
*/
type ReqVersion struct {
	Platform bool
	Version  string
	//新增平台信息（用来表征调用者信息）name+ip 信息
	Infor    string
}

/*
响应请求
Version   版本号
change    是否发生变更，true 发生变更；false 未发生变更
Template  模板文件
*/
type TemplVersion struct {
	Version  string
	Change   bool
	Template []byte
}

func RunRegister() {
	for {
		if config.Instance.Common.EnableRegister {
			GetTemplate()
		}
		time.Sleep(time.Second * 5)
	}
}
func GetTemplate() {
	defer func() {
		if e := recover(); e != nil {
			log.Errorf("GetTemplate recover error|error=%v", e)
			rpcClint = nil
		}
	}()
	client := getRcpClint()
	req := ReqVersion{config.Instance.Common.Platform, Version,config.Instance.Wechat1.Name+config.Instance.Wechat1.NginxIp}
	result := TemplVersion{}
	err := client.Call("GetTemplate.GetTemplate", &req, &result)
	if err != nil {
		log.Errorf("register_GetTemplate_RPCCall error|error=%v", err)
		return
	}
	if !result.Change {
		log.Critical("register no change")
		return
	}
	Version = result.Version
	versionOld := getCentent(result)
	WriteFile(versionOld)
	return
}
func getCentent(result TemplVersion) (content string) {
	fconf, err := os.Open("./conf.toml")
	if err != nil {
		log.Errorf("register_getCentent error|error=%v", err)
		return
	}
	defer fconf.Close()
	data, _ := ioutil.ReadAll(fconf)
	versionOld := string(data)
	versionNew := string(result.Template)
	var target []string
	verArr := strings.Split(versionNew, "\n")
	for i := 0; i < len(verArr); i++ {
		if strings.HasPrefix(verArr[i], "#") || strings.HasPrefix(verArr[i], "[") || len(strings.Replace(verArr[i], " ", "", -1)) == 1 || len(strings.Replace(verArr[i], " ", "", -1)) == 0 {
			continue
		}
		target = append(target, verArr[i])
	}
	for i := 0; i < len(target); i++ {
		ssArr := strings.Split(target[i], "=")
		targetOld := strings.Split(versionOld, "\n")
		var targetV string
		for _, v := range targetOld {
			if strings.HasPrefix(v, ssArr[0]) {
				targetV = v
				break
			}
		}
		versionOld = strings.Replace(versionOld, targetV, target[i], -1)
	}
	return versionOld
}
func WriteFile(content string) error {
	f, err := os.OpenFile("./conf.toml", os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0644)
	if err != nil {
		log.Infof("register_writeFile error|error=%v ", err)
	} else {
		defer f.Close()
		n, _ := f.Seek(0, os.SEEK_END)
		_, err = f.WriteAt([]byte(content), n)
		log.Critical("register_writerFile sucess")
	}
	return err
}
func getRcpClint() *rpc.Client {
	if rpcClint == nil {
		rpcC, err := rpc.DialHTTP("tcp", config.Instance.Common.RegiServer)
		if err != nil {
			log.Errorf("rpc.DialHTTP error=%v", err)
			//重置连接
			rpcClint=nil
			return nil
		}
		rpcClint = rpcC
	}
	return rpcClint
}
