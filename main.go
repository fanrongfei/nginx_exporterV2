package main

import (
	"fmt"
	"nginx_exporterV2/config"
	"nginx_exporterV2/exporter"
	"nginx_exporterV2/lib"
	"nginx_exporterV2/monitor"
	"nginx_exporterV2/register"
	"nginx_exporterV2/summary"
	"runtime"
	"sync"
	"time"
)

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	var Once sync.Once
	var startSign = new(lib.TaskSign)
	startSign.Init()
	go exporter.Analysis(startSign)
	//汇总统计
	go summary.Analysis()
	//遍历解析access.log
	go exporter.TraverseAccess()
	//更新配置
	go lib.UpdateConf()
	//零点统计数据一天5xx 错误码>200 的
	go summary.CronSUmmary("0 0 0 * * ?")
	//零点统计200接口响应时间大于阈值的
	go summary.CronSUmmaryResponse("0 0 0 * * ?")
	//等待access.log进行日志切割，暂时释放句柄，不然无法进行日志切割
	go monitor.Monitor("0 59 23 * * ?")
	//新增（2020/12/4 14点46分）处理每日历史告警path
	go exporter.AnalysisTodayCumulativePath()
	//新增(2020/12/4 16点13分)处理每个报警信息
	go exporter.AnalysisCyclealarmPath()
	//新增(2020/12/4 16点13分)处理每个报警信息
	go exporter.AlarmPath()
	//注册中心服务
	go register.RunRegister()
	tit := time.NewTimer(time.Minute * time.Duration(config.Instance.Alarm.FragmentSize))
	for {
		select {
		case <-tit.C:
			start := time.Now()
			exporter.Timetask()
			fmt.Println(time.Now().Sub(start).Seconds())
			//首次启动开关(优化cpu 防止第二次循环再进行时间区间判断)
			Once.Do(func() {
				startSign.Reset()
			})
			tit.Reset(time.Minute * time.Duration(config.Instance.Alarm.FragmentSize))
		}
	}
}
