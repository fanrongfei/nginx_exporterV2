package lib

import (
	"container/list"
	"crypto/md5"
	"encoding/base64"
	"fmt"
	log "github.com/cihub/seelog"
	"io"
	"math"
	"nginx_exporterV2/config"
	"os"
	"strings"
	"sync"
	"time"
)

var (

	/*
	存请求信息流的
	key:  url:响应码
	value:统计次数
	*/
	GatWayMap map[string]int
	AcclaPath =new(AccumulatedPath)
	AHPath =new(alreadyHavePath)
)
// 定义企业微信token格式
type TokenC struct {
	Access_token string `json:"access_token"`
}

// 定义企业微信发送模板格式
type MESSAGES struct {
	Touser string `json:"touser"`
	Toparty string `json:"toparty"`
	Msgtype string `json:"msgtype"`
	Agentid int `json:"agentid"`
	Text struct {
		//Subject string `json:"subject"`
		Content string `json:"content"`
	} `json:"text"`
	Safe int `json:"safe"`
}
func init() {
	GatWayMap = make(map[string]int)
	AcclaPath.Init()
	AHPath.Init()
}
//配置更新
func UpdateConf() {
	oldMdS := getMd5Sum()
	for {
		newMdS := getMd5Sum()
		if oldMdS != newMdS {
			config.IatClientCfg().Init(false)
			oldMdS = newMdS
		}
		time.Sleep(time.Second)
	}
}

//计算文件md5sum值
func getMd5Sum() string {
	f, err := os.Open("./conf.toml")
	if err != nil {
		log.Errorf("lib_GetMd5Sum|err=%v", err)
		return ""
	}
	defer f.Close()
	md5hash := md5.New()
	if _, err := io.Copy(md5hash, f); err != nil {
		log.Errorf("lib_GetMd5Sum|copy_file_to_md5 error=%v", err)
		return ""
	}
	return base64.StdEncoding.EncodeToString(md5hash.Sum(nil))
}

/*

每个请求行 实体
eable        是否有效,排除第一次启动导致的
nginxPath    请求行
RespTime     接口响应时间(ms)
nginxStatus  状态码
nginxCode    状态码归类 1xx；2xx；3xx；4xx；5xx

*/
type  EntityNginx struct {
	Eable bool
	NginxPath string
	RespTime int
	NginxStatus string
	NginxCode string
}

type EntityNginxList struct {
	 sync.RWMutex
	 Version bool
	 EntityNginxArr []EntityNginx
}
func (Entity *EntityNginxList)Insert(entiy EntityNginx){
	Entity.Lock()
	defer Entity.Unlock()
	if entiy.Eable{
		Entity.EntityNginxArr = append(Entity.EntityNginxArr, entiy)
	}
	return
}
func (Entity *EntityNginxList)Reset(){
	Entity.Lock()
	defer Entity.Unlock()
	Entity=new(EntityNginxList)
	return
}
func (Entity *EntityNginxList)GetEntityArr()(EntityArr []EntityNginx){
	Entity.Lock()
	defer Entity.Unlock()
	EntityArr=Entity.EntityNginxArr
	Entity.EntityNginxArr=*new([]EntityNginx)
	return
}
/*
运行时间节点
todayTheTime 今日时间节点
eable 是否可用；缓存处理结果,加速处理

*/
type TimeSign struct {
	TodayTheTime string
	Eable bool
}
func (t *TimeSign)Reset(){
	t.TodayTheTime=time.Now().Format("02/Jan/2006")
	t.Eable=false
}
func (t *TimeSign)Compare(reqLine string)bool{
	if t.Eable{
		return true
	}
	//排除因日志切割延迟导致 日志混有昨天的数据
	 if t.TodayTheTime==timeInRange(reqLine){
	 	t.Eable=true
	 	return true
	 }else{
	 	return false
	 }
}
func (t *TimeSign)CompareNow(){
	if t.TodayTheTime!=time.Now().Format("02/Jan/2006"){

	}
}
func timeInRange(reqline string)string{
	defer func() {
		if e:=recover();e!=nil{
			log.Errorf("timeInRange error=%v",e)
			fmt.Printf("timeInRange error=%v \n",e)
		}
	}()
	timeRang := strings.Split(reqline, " ")
	timeStr := timeRang[3]
	return timeStr[1:strings.Index(timeStr,":")]
}
/*
用来控制第一次任务
Eable 是否是第一次执行
*/
type TaskSign struct {
	StartTime time.Time
	Eable bool
}
func(timeS *TaskSign)Init(){
	timeS.Eable=true
	//timeS.StartTime=time.Now().Add(-time.Duration(config.Instance.Alarm.FragmentSize) * time.Minute)
	timeS.StartTime=time.Now()
}
func(timeS *TaskSign)Reset(){
	timeS=new(TaskSign)
}

//排序实体
type PathStatus struct {
	//接口名
	Path  string
	//总和
	Num int
}
//定义排序数据结构
type PathStatusArr []PathStatus
//Len()
func (s PathStatusArr) Len() int {
	return len(s)
}
//Less()
func (s PathStatusArr) Less(i, j int) bool {
	return s[i].Num > s[j].Num
}
//Swap()
func (s PathStatusArr) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
//path和code（4xx/5xx）
type RecordPath struct{
	Path string `json:"path"`
	Code string `json:"code"`
}

//存当日已经告警过的path  list.list 存"4xx"或者"5xx"……
type alreadyHavePath struct {
	HavePath map[string]*list.List `json:"havePath"`
	sync.Mutex
}
func (ah *alreadyHavePath)Init(){
		ah.HavePath=make(map[string]*list.List)
}
func  (ah *alreadyHavePath)Add(path,code string){
	ah.Lock()
	defer ah.Unlock()
	  if l,b:=ah.HavePath[path];b{
	  	for i:=l.Front();i!=nil;i=i.Next(){
	  		if i.Value==code{
				return
			}
		}
		  l.PushBack(code)
	  }else{
	  	ln:=list.New()
	  	ln.PushBack(code)
	  	ah.HavePath[path]=ln
	  }
}
func (ah *alreadyHavePath) Isin(path,code string)bool{
	defer func() {
		if e:=recover();e!=nil{
			fmt.Sprintf("func_Isin_recover_err:=%v \n",e)
		}
	}()
	  ah.Lock()
	  defer ah.Unlock()
	  if l,b:=ah.HavePath[path];b{
		  for i:=l.Front();i!=nil;i=i.Next(){
			  if i.Value==code{
				  return true
			  }
		  }
	  }
	  	return false
}
func (ah *alreadyHavePath)Release(){
	ah.Lock()
	defer ah.Unlock()
	ah.HavePath=make(map[string]*list.List)
}

//定义累计path 错误码结构
type nodealarm struct{
	NowNumber int `json:"nowNumber"`     //当前数据
	HeadNumber int `json:"headNumber"`   //上一次发送数据量
	IsSend bool `json:"isSend"`          //是否已经发送
	SendTime time.Time `json:"sendTime"` //上一次发送告警时间
}
type AccumulatedPath struct{
	alarmMap map[string]map[string]*nodealarm
	sync.Mutex
}
func (ap *AccumulatedPath)Init(){
	  ap.alarmMap=make(map[string]map[string]*nodealarm)
}
func (ap *AccumulatedPath)Release(){
	ap.Lock()
	defer ap.Unlock()
	ap.alarmMap=make(map[string]map[string]*nodealarm)
}
func (ap *AccumulatedPath)Add(path,code string){
	defer func() {
		if e:=recover();e!=nil{
			fmt.Sprintf("func_AccumulatedPath(add)_recover_err:=%v \n",e)
		}
	}()
	ap.Lock()
	defer ap.Unlock()
	//todo 不知道要不要加一些过滤规则呢？比如 过滤掉 .png/.gif/.wav/.exe…… 等接口呢？（哎不管了，最好在白黑名单干吧，我这就不加了，后来人应该比我聪明，就交给你了）
	if !AHPath.Isin(path,code){
		if tempm,b:=ap.alarmMap[path];b{
			if na,b:=tempm[code];b{
				na.NowNumber=na.NowNumber+1
			}else{
				na=new(nodealarm)
				na.NowNumber=1
				tempm[code]=na
			}
		}else{
			temp:=make(map[string]*nodealarm)
			na:=new(nodealarm)
			na.NowNumber=1
			tempm[code]=na
			ap.alarmMap[path]=temp
		}
	}else{
		//如果存在，并且自己也存在，则删除之（不要问我为啥：主程序阈值期延迟报了，之前的历史应该要干掉）
		if tempm,b:=ap.alarmMap[path];b{
			if _,bo:=tempm[code];bo{
				delete(tempm, code)
			}
		}
	}
}
//获得告警序列
func (ab *AccumulatedPath)GetalarmPathMess()(resultArr []string){
	ab.Lock()
	defer ab.Unlock()
	for path,tempm:=range ab.alarmMap {
		for codeP,nodem:=range tempm{
			numberAlarm:=nodem.NowNumber
			//第一次告警
			if numberAlarm>=config.Instance.Wechat3.Threshold3&&!nodem.IsSend{
				nodem.NowNumber=0
				nodem.IsSend=true
				nodem.HeadNumber=numberAlarm
				nodem.SendTime=time.Now()
				//纳入报警序列
				resultArr = append(resultArr, fmt.Sprintf("%s 今日%s告警值%d,超过阈值%d \n",path,codeP,numberAlarm,config.Instance.Wechat3.Threshold3))
			}else if numberAlarm>=config.Instance.Wechat3.Threshold3&&nodem.IsSend{
				//非第一次告警,增长率较快
				if math.Round(float64(numberAlarm)/float64(nodem.HeadNumber))>=2{
					nodem.NowNumber=0
					nodem.IsSend=true
					nodem.HeadNumber=numberAlarm
					nodem.SendTime=time.Now()
					resultArr = append(resultArr, fmt.Sprintf("%s 今日%s告警值%d,超过阈值%d \n",path,codeP,numberAlarm,config.Instance.Wechat3.Threshold3))
				}
			}
		}
	}
	return
}
//创建消息格式  path 今日4xx告警超过xx；
