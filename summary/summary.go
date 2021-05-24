package summary

import (
	"bytes"
	"fmt"
	log "github.com/cihub/seelog"
	cron "github.com/robfig"
	"nginx_exporterV2/comm"
	"nginx_exporterV2/config"
	"nginx_exporterV2/exporter"
	"nginx_exporterV2/lib"
	"nginx_exporterV2/websend"
	"sort"
	"strings"
	"sync"
	"time"
)

/*
每天数据汇总
1、0点数据汇总发送
2、当天分段汇总

*/
var (
	lc sync.Mutex
	//[]{path,responseTime}
	 responArr []responEntity
)
type responEntity struct {
	path string
	responseTime int
}

func Analysis() {
	for i := 0; i < 8; i++ {
		go func() {
			for reqLine := range exporter.HistoryChan {
				//发送报警序列
				inserToMap(comm.GetEntityNginx(reqLine))
				//统计接口响应时间大于阈值的
			}
		}()
	}
}

func inserToMap(entity lib.EntityNginx) {
	lc.Lock()
	defer lc.Unlock()
	//500 每日统计
	if _, bo := exporter.HistoryMap[entity.NginxPath]; !bo {
		tempmap := make(map[string]int)
		tempmap[entity.NginxStatus] = 1
		exporter.HistoryMap[entity.NginxPath] = tempmap
	} else {
		exporter.HistoryMap[entity.NginxPath][entity.NginxStatus] = exporter.HistoryMap[entity.NginxPath][entity.NginxStatus] + 1
	}
	//200 接口响应时间统计
	if entity.NginxStatus=="200"{
		responArr = append(responArr, responEntity{entity.NginxPath,entity.RespTime})
	}
}

//接口 5xx 大于阈值的
func CronSUmmary(cronString string) {
	defer func() {
		if e := recover(); e != nil {
			fmt.Printf("func_CronSUmmary_cronError_cronsCript(%s)_recover:%v \n", cronString, e)
			log.Errorf("func_CronSUmmary_cronError_cronsCript(%s)_recover:%v \n", cronString, e)
		}
	}()
	cr := cron.New()
	cr.AddFunc(cronString, func() {
		//释放每日积累告警接口数据
		lib.AcclaPath.Release()
		//开始汇总，当天接口5xx 数 >200 进行统计
		tempMap := exporter.HistoryMap
		//重置HistoryMap
		exporter.HistoryMap = make(map[string]map[string]int)
		tempmap := make(map[string]int)
		for k, v := range tempMap {
			tempTotal := 0
			for k1, v1 := range v {
				if strings.HasPrefix(k1, "5") {
					tempTotal = tempTotal + v1
				}
			}
			if tempTotal >= 200 {
				tempmap[k] = tempTotal
			}
		}
		//发送报告
		if len(tempmap) == 0 {
			return
		}
		var bB bytes.Buffer
		var sortUr lib.PathStatusArr
		for k1, v1 := range tempmap {
			sortUr = append(sortUr, lib.PathStatus{k1, v1})
		}
		sort.Sort(sortUr)
		var onceSend sync.Once
		var SEND=false
		for i := 0; i < len(sortUr); i++ {
			if bB.Len()>=1900{
				onceSend.Do(func() {
					websend.SendSummaryWechat(bB,1, nil,false)
					SEND=true
					bB.Reset()
				})
				websend.SendSummaryWechat(bB,1, nil,SEND)
				bB.Reset()
			}
			bB.WriteString(fmt.Sprintf("URL=%s, SUM=%d \n", sortUr[i].Path, sortUr[i].Num))
		}
		bB.WriteString("-----------------------------------------------")
		websend.SendSummaryWechat(bB,1,nil,SEND)
		bB.Reset()
		log.Infof("Time%s,0点统计5xx>200 的接口,涉及到的接口数%d",time.Now().Format("2006-01-02 15:04:05"),len(sortUr))
		tempmap = nil
	})
	cr.Start()
	for {
		time.Sleep(time.Second)
	}
}
//定时任务 > 响应时间阈值的接口
func CronSUmmaryResponse(cronString string) {
	defer func() {
		if e := recover(); e != nil {
			fmt.Printf("func_CronSUmmaryResponse_cronError_cronsCript(%s)_recover:%v \n", cronString, e)
			log.Errorf("func_CronSUmmaryResponse_cronError_cronsCript(%s)_recover:%v \n", cronString, e)
		}
	}()
	cr := cron.New()
	cr.AddFunc(cronString, func() {
		//重置数据
		lib.AHPath.Release()
		//开始汇总，当天接口5xx 数 >200 进行统计
		tempArr:=responArr
		//重置responArr
		responArr=*new([]responEntity)
		tempMap:=make(map[string]int)
		for i:=0;i<len(tempArr);i++{
			if tempArr[i].responseTime>=config.Instance.RuleResponse.ResponseTime{
				tempMap[tempArr[i].path]=tempMap[tempArr[i].path]+1
			}
		}
		//发送报告
		if len(tempMap) == 0 {
			return
		}
		var bB bytes.Buffer
		var sortUr lib.PathStatusArr
		for k1, v1 := range tempMap {
			sortUr = append(sortUr, lib.PathStatus{k1, v1})
		}
		sort.Sort(sortUr)
		var onceSend sync.Once
		SEND:=false
		for i := 0; i < len(sortUr); i++ {
			if bB.Len()>=1900{
				onceSend.Do(func() {
					websend.SendSummaryWechat(bB,2,config.Instance.RuleResponse.ResponseTime,SEND)
					SEND=true
					bB.Reset()
				})
				websend.SendSummaryWechat(bB,2,config.Instance.RuleResponse.ResponseTime,SEND)
				bB.Reset()
			}
			bB.WriteString(fmt.Sprintf("URL=%s, SUM=%d \n", sortUr[i].Path, sortUr[i].Num))
		}
		bB.WriteString("-----------------------------------------------")
		websend.SendSummaryWechat(bB,2,config.Instance.RuleResponse.ResponseTime,SEND)
		bB.Reset()
		log.Infof("Time%s,0点统计200的接口响应时间大于%d ms,涉及到的接口数%d",time.Now().Format("2006-01-02 15:04:05"),config.Instance.RuleResponse.ResponseTime,len(tempMap))
	})
	cr.Start()
	for {
		time.Sleep(time.Second)
	}
}
