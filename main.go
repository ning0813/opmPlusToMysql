package main

import (
	"crypto/tls"
	"fmt"
	"github.com/go-redis/redis"
	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/robfig/cron/v3"
	"github.com/tidwall/gjson"
	"github.com/valyala/fasthttp"
	"github.com/xuri/excelize/v2"
	"gopkg.in/yaml.v3"
	"log"
	"os"
	"regexp"
	"strings"
	"time"
)

var mysqlDb *sqlx.DB

// 连接 mysql 参数变量
//var (
//	usename  string = "root"
//	password string = "123456"
//	serverIp string = "localhost"
//	port     int    = 3306
//	dbname   string = "test"
//)

var (
	usename  string = "root"
	password string = "123456"
	serverIp string = "10.48.8.24"
	port     int    = 3306
	dbname   string = "dio"
)

func initMysql(uName string, pwd string, ipAddr string, pt int, dName string) *sqlx.DB {
	fmt.Println("init........mysql")
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8", uName, pwd, ipAddr, pt, dName)
	//fmt.Printf("dsn: %s\n", dsn)
	Db, err := sqlx.Open("mysql", dsn)
	if err != nil {
		log.Println("mysql connect fail, datail is [%s]", err.Error())
	}
	return Db
}

func init() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	logFile, err := os.OpenFile("./out.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Println("打开日志文件异常")
	}
	log.SetOutput(logFile)
	loadConf()
	mysqlDb = initMysql(usename, password, serverIp, port, dbname)
}

type Cfg struct {
	ExcelPath         string `yaml:"excelPath""`
	CronStr           string `yaml:"cronStr""`
	ApmUrl            string `yaml:"apmUrl""`
	ApmToken          string `yaml:"apmToken""`
	ApmTopic          string `yaml:"apmTopic""`
	OpmUrl            string `yaml:"opmUrl""`
	OpmToken          string `yaml:"opmToken""`
	OpmInterfaceTopic string `yaml:"opmInterfaceTopic""`
	OpmSummaryTopic   string `yaml:"opmSummaryTopic""`
	OpmMonitorTopic   string `yaml:"opmMonitorTopic""`
	OpmPlusUrl        string `yaml:"opmPlusUrl""`
	OpmPlusToken      string `yaml:"opmPlusToken""`
	OpmPlusTopic      string `yaml:"opmPlusTopic""`
	KafkaServers      string `yaml:"kafkaServers""`
}

var cfg Cfg

// 加载配置文件
func loadConf() {
	file, err := os.ReadFile("conf.yaml")
	if err != nil {
		log.Println("failed to read yaml file : %v", err)
		return
	}

	err = yaml.Unmarshal(file, &cfg)
	if err != nil {
		panic(err)
	}
}

type dio_object struct {
	AppId           string `json:"appid"`
	Type            string `json:"type"`
	Name            string `json:"name"`
	UnifiedObjectID string `json:"unified_object_id"`
	OriObjectID     string `json:"ori_object_id"`
}

type dio_object_relation struct {
	AppId        string `json:"appid"`
	UObjid       string `json:"u-objid"`
	UObjParentID string `json:"u-obj-parent-id"`
}

type dio_object_server struct {
	AppId       string `json:"appid"`
	OriObjectID string `json:"ori_object_id"`
	Ip          string `json:"ip"`
	Host        string `json:"host"`
}

type dio_metric struct {
	AppId        string `json:"appid"`
	Type         string `json:"type"`
	Name         string `json:"name"`
	MetricKey    string `json:"metricKey"`
	OriMetricID  string `json:"ori_metric_id"`
	UnifMetricID string `json:"unif_metric_id"`
}

type dio_object_metric_relation struct {
	AppId        string `json:"appid"`
	UniObjID     string `json:"uni_obj_id"`
	UnifMetricID string `json:"unif-metric_id"`
}

type MonitorToAiops struct {
	Type        string `json:"type"`
	DisplayName string `json:"displayname""`
	ResourceId  string `json:"resourceId""`
	GroupsName  string `json:"applications1"`
	HostName    string
	MetricKey   string
	Kpi         string `json:"kpi_name"`
}

func createObject(appid, _type, displayName, resourceId string) {
	o1 := dio_object{appid, _type, displayName, appid + "##" + resourceId, appid + "##" + resourceId}
	add_record_object(o1)
}

func createObjectRelation(appid, objectId string) {
	or1 := dio_object_relation{appid, appid + "##" + objectId, appid}
	add_record_object_relation(or1)
}

func createObjectServer(appid, host string) {
	os1 := dio_object_server{appid, appid + "##" + host, host, host}
	add_record_object_server(os1)
}

func createMetric(appid, _type, metricName, metricKey, ori_metricId, unif_metric_id string) {
	o1 := dio_metric{appid, _type, metricName, appid + "##" + metricKey, appid + "##" + ori_metricId, appid + "##" + unif_metric_id}
	add_record_metric(o1)
}
func createObjectMetricRelation(appid, objId, merticId string) {
	o1 := dio_object_metric_relation{appid, appid + "##" + objId, appid + "##" + merticId}
	add_record_metric_relation(o1)
}

var redisdb *redis.Client

// 初始化redis
//pro
func initRedis() (err error) {
	fmt.Println("load........redis")
	redisdb = redis.NewClient(&redis.Options{
		Addr:     "192.168.100.140:6379", // 指定 154.211.14.206:6379
		Password: "",
		DB:       2, // redis一共16个库，指定其中一个库即可
	})
	//redisdb = redis.NewClient(&redis.Options{
	//	Addr:     "154.211.14.206:6379", // 指定
	//	Password: "123456",
	//	DB:       2, // redis一共16个库，指定其中一个库即可
	//})
	_, err = redisdb.Ping().Result()
	return
}

func add_record_object(obj dio_object) int {
	tx, err := mysqlDb.Begin()
	if err != nil {
		log.Println("open mysql database fail", err)
	}
	cmd := fmt.Sprintf("insert into dio_object(`appid`, `type`, `name`, `ori_object_id`, `unified_object_id`,`create_time`) select '%s','%s','%s','%s','%s',NOW() from DUAL where not exists (select id from dio_object where unified_object_id = '%s')", obj.AppId, obj.Type, obj.Name, obj.OriObjectID, obj.UnifiedObjectID, obj.UnifiedObjectID)
	//fmt.Printf("add cmd: %s\n", cmd)
	_, err = mysqlDb.Exec(cmd)
	if err != nil {
		log.Println("mysql insert fail, datail is ", err.Error())
		time.Sleep(1 * time.Second)
		cmd := fmt.Sprintf("insert into dio_object(`appid`, `type`, `name`, `ori_object_id`, `unified_object_id`,`create_time`) select '%s','%s','%s','%s','%s',NOW() from DUAL where not exists (select id from dio_object where unified_object_id = '%s')", obj.AppId, obj.Type, obj.Name, obj.OriObjectID, obj.UnifiedObjectID, obj.UnifiedObjectID)
		_, err = mysqlDb.Exec(cmd)
		if err != nil {
			tx.Rollback()
			return -1
		}
		tx.Commit()
	}
	tx.Commit()
	return 0
}

func add_record_object_relation(obj dio_object_relation) int {
	tx, err := mysqlDb.Begin()
	if err != nil {
		log.Println("open mysql database fail", err)
	}

	cmd := fmt.Sprintf("insert into dio_object_relation(`appid`, `unified_object_id`, `unified_object_parent_id`, `create_time`) select '%s','%s','%s',NOW() from DUAL where not exists (select id from dio_object_relation where unified_object_id = '%s' and unified_object_parent_id = '%s')", obj.AppId, obj.UObjid, obj.UObjParentID, obj.UObjid, obj.UObjParentID)
	//fmt.Printf("add cmd: %s\n", cmd)
	_, err = mysqlDb.Exec(cmd)
	if err != nil {
		log.Println("mysql insert fail, datail is ", err.Error())
		time.Sleep(1 * time.Second)
		cmd := fmt.Sprintf("insert into dio_object_relation(`appid`, `unified_object_id`, `unified_object_parent_id`, `create_time`) select '%s','%s','%s',NOW() from DUAL where not exists (select id from dio_object_relation where unified_object_id = '%s' and unified_object_parent_id = '%s')", obj.AppId, obj.UObjid, obj.UObjParentID, obj.UObjid, obj.UObjParentID)
		_, err = mysqlDb.Exec(cmd)
		if err != nil {
			tx.Rollback()
			return -1
		}
		tx.Commit()
	}
	tx.Commit()
	return 0
}

func add_record_object_server(obj dio_object_server) int {
	tx, err := mysqlDb.Begin()
	if err != nil {
		log.Println("open mysql database fail", err)
	}

	cmd := fmt.Sprintf("insert into dio_object_server(`appid`, `unified_object_id`, `ip`,`host`, `create_time`) select '%s','%s','%s','%s',NOW() from DUAL where not exists (select id from dio_object_server where unified_object_id = '%s' )", obj.AppId, obj.OriObjectID, obj.Ip, obj.Host, obj.OriObjectID)
	//fmt.Printf("add cmd: %s\n", cmd)
	_, err = mysqlDb.Exec(cmd)
	if err != nil {
		log.Println("mysql insert fail, datail is ", err.Error())
		time.Sleep(1 * time.Second)
		cmd := fmt.Sprintf("insert into dio_object_server(`appid`, `unified_object_id`, `ip`,`host`, `create_time`) select '%s','%s','%s','%s',NOW() from DUAL where not exists (select id from dio_object_server where unified_object_id = '%s' )", obj.AppId, obj.OriObjectID, obj.Ip, obj.Host, obj.OriObjectID)
		_, err = mysqlDb.Exec(cmd)
		if err != nil {
			tx.Rollback()
			return -1
		}
		tx.Commit()
	}
	tx.Commit()
	return 0
}

func add_record_metric(obj dio_metric) int {
	tx, err := mysqlDb.Begin()
	if err != nil {
		log.Println("open mysql database fail", err)
	}

	cmd := fmt.Sprintf("insert into dio_metric(`appid`, `type`, `metric_name`, `metric_key`, `ori_metric_id`, `unified_metric_id`, `create_time`) select '%s','%s','%s','%s','%s','%s',NOW() from DUAL where not exists (select id from dio_metric where metric_key = '%s' )", obj.AppId, obj.Type, obj.Name, obj.MetricKey, obj.OriMetricID, obj.UnifMetricID, obj.MetricKey)
	//fmt.Printf("add cmd: %s\n", cmd)
	_, err = mysqlDb.Exec(cmd)
	if err != nil {
		log.Println("mysql insert fail, datail is [%s]\n", err.Error())
		time.Sleep(1 * time.Second)
		cmd := fmt.Sprintf("insert into dio_metric(`appid`, `type`, `metric_name`, `metric_key`, `ori_metric_id`, `unified_metric_id`, `create_time`) select '%s','%s','%s','%s','%s','%s',NOW() from DUAL where not exists (select id from dio_metric where metric_key = '%s' )", obj.AppId, obj.Type, obj.Name, obj.MetricKey, obj.OriMetricID, obj.UnifMetricID, obj.MetricKey)
		//fmt.Printf("add cmd: %s\n", cmd)
		_, err = mysqlDb.Exec(cmd)
		if err != nil {
			tx.Rollback()
			return -1
		}
		tx.Commit()
	}
	tx.Commit()
	return 0
}

func add_record_metric_relation(obj dio_object_metric_relation) int {
	tx, err := mysqlDb.Begin()
	if err != nil {
		log.Println("open mysql database fail", err)
	}
	cmd := fmt.Sprintf("insert into dio_object_metric_relation(`appid`, `unified_object_id`, `unified_metric_id`, `create_time`) select '%s','%s','%s',NOW() from DUAL where not exists (select id from dio_object_metric_relation where unified_object_id = '%s' and unified_metric_id = '%s')", obj.AppId, obj.UniObjID, obj.UnifMetricID, obj.UniObjID, obj.UnifMetricID)
	//fmt.Printf("add cmd: %s\n", cmd)
	_, err = mysqlDb.Exec(cmd)
	if err != nil {
		log.Println("mysql insert fail, datail is [%s]\n", err.Error())
		time.Sleep(1 * time.Second)
		cmd := fmt.Sprintf("insert into dio_object_metric_relation(`appid`, `unified_object_id`, `unified_metric_id`, `create_time`) select '%s','%s','%s',NOW() from DUAL where not exists (select id from dio_object_metric_relation where unified_object_id = '%s' and unified_metric_id = '%s')", obj.AppId, obj.UniObjID, obj.UnifMetricID, obj.UniObjID, obj.UnifMetricID)
		//fmt.Printf("add cmd: %s\n", cmd)
		_, err = mysqlDb.Exec(cmd)
		if err != nil {
			tx.Rollback()
			return -1
		}
		tx.Commit()
	}
	tx.Commit()
	return 0
}

func httpGet(url string) string {

	client := &fasthttp.Client{} //发起请求的对象
	client.TLSConfig = &tls.Config{InsecureSkipVerify: true}
	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req) // 用完需要释放资源

	// 默认是application/x-www-form-urlencoded
	req.Header.SetContentType("application/json")
	req.Header.SetMethod("GET")
	req.SetRequestURI(url)
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp) // 用完需要释放资源

	if err := client.Do(req, resp); err != nil {
		log.Println(err)
	}
	b := resp.Body()
	return string(b)
	//fmt.Println("result:", string(b))
}

// 监控指标数据
func getDeviceAssociatedMonitors(objType, deviceName string) {
	//	获取监控指标数据
	detailUrl := cfg.OpmUrl + "/api/json/device/getDeviceAssociatedMonitors?apiKey=" + cfg.OpmToken + "&name=" + deviceName
	data := httpGet(detailUrl)

	if data != "" {
		// 对象关系
		createObject("102", objType, deviceName, deviceName)
		createObjectRelation("102", deviceName)
		createObjectServer("102", deviceName)
		array := gjson.Get(data, "performanceMonitors.monitors").Array()
		for _, arr := range array {
			policyName := gjson.Get(arr.Raw, "policyName").String()
			displayName := gjson.Get(arr.Raw, "DISPLAYNAME").String()
			pollId := gjson.Get(arr.Raw, "pollId").String()
			createMetric("102", policyName, deviceName+"_"+displayName, deviceName+"_"+pollId+"_"+policyName, deviceName, deviceName+"_"+policyName)
			createObjectMetricRelation("102", deviceName, deviceName+"_"+policyName)
		}
	}
}

// 接口数据
func getInterfaceData(deviceName string) {
	//	获取监控指标数据
	detailUrl := cfg.OpmUrl + "/api/json/device/getInterfaces?apiKey=" + cfg.OpmToken + "&name=" + deviceName
	data := httpGet(detailUrl)
	if data != "" {

	}
}

func getSummaryData(deviceName string) {
	//	获取监控指标数据
	detailUrl := cfg.OpmUrl + "/api/json/device/getDeviceSummary?apiKey=" + cfg.OpmToken + "&name=" + deviceName
	data := httpGet(detailUrl)

	if data != "" {

	}
}

func getOpmDetail(objType, deviceName string) {
	getDeviceAssociatedMonitors(objType, deviceName)
	//getInterfaceData(deviceName)
	//getSummaryData(deviceName)
}

func getMonitorData(resourceId string) {
	//	获取监控指标数据
	detailUrl := cfg.OpmPlusUrl + "/AppManager/json/GetMonitorData?apikey=" + cfg.OpmPlusToken + "&resourceid=" + resourceId
	data := httpGet(detailUrl)
	if data != "" {
		//	生成对象关系
		array := gjson.Get(data, "response.result").Array()
		for _, arr := range array {
			displayName := gjson.Get(arr.Raw, "DISPLAYNAME").String()
			_type := gjson.Get(arr.Raw, "TYPE").String()
			resourceId := gjson.Get(arr.Raw, "RESOURCEID").String()
			createObject("101", _type, displayName, resourceId)
			attributes := gjson.Get(arr.Raw, "Attribute").Array()
			for _, attribute := range attributes {
				attributeName := strings.Replace(gjson.Get(attribute.Raw, "DISPLAYNAME").Raw, "\"", "", 2)
				attributeId := gjson.Get(attribute.Raw, "AttributeID").String()
				createMetric("101", attributeName, displayName+"_"+attributeName, resourceId+"_"+attributeId, attributeId, resourceId+"_"+attributeId)
				createObjectMetricRelation("101", resourceId, resourceId+"_"+attributeId)
			}
			childMonitors := gjson.Get(arr.Raw, "CHILDMONITORS").Array()
			for _, childMonitor := range childMonitors {
				monitorName := gjson.Get(childMonitor.Raw, "DISPLAYNAME").String()
				childMonitorInfos := gjson.Get(childMonitor.Raw, "CHILDMONITORINFO").Array()
				for _, childMonitorInfo := range childMonitorInfos {
					childMonitorInfoName := strings.Replace(gjson.Get(childMonitorInfo.Raw, "DISPLAYNAME").Raw, "\"", "", 2)
					childMonitorInfoResourceId := gjson.Get(childMonitorInfo.Raw, "RESOURCEID").String()
					childAttributes := gjson.Get(childMonitorInfo.Raw, "CHILDATTRIBUTES").Array()
					for _, childAttribute := range childAttributes {
						childAttributeName := strings.Replace(gjson.Get(childAttribute.Raw, "DISPLAYNAME").Raw, "\"", "", 2)
						childAttributeId := gjson.Get(childAttribute.Raw, "AttributeID").String()
						type1 := monitorName + "_" + childAttributeName
						name := displayName + "_" + childMonitorInfoName + "_" + monitorName + "_" + childAttributeName
						key := resourceId + "_" + childMonitorInfoResourceId + "_" + childAttributeId
						metricId := childMonitorInfoResourceId + "_" + childAttributeId
						unMetricid := resourceId + "_" + childMonitorInfoResourceId + "_" + childAttributeId
						createMetric("101", type1, name, key, metricId, unMetricid)
						createObjectMetricRelation("101", resourceId, resourceId+"_"+childMonitorInfoResourceId+"_"+childAttributeId)
					}
				}
			}
		}
	} else {
		log.Println(resourceId)
	}
}

// 获取excel中的IP
func readExcel() {
	f, err := excelize.OpenFile(cfg.ExcelPath)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() {
		if err := f.Close(); err != nil {
			log.Println(err)
		}
	}()
	// 获取 Sheet1 上所有单元格
	sheetList := f.GetSheetList()
	for _, sheet := range sheetList {
		cols, err := f.GetCols(sheet)
		if err != nil {
			log.Println(err)
			return
		}
		for _, col := range cols {
			if col[0] == "Resourceid" || col[0] == "DeviceName" {
				for i := 1; i < len(col); i++ {
					// 调用接口
					matchString, _ := regexp.MatchString("\\d+\\.\\d+\\.\\d+\\.\\d+", col[i])
					matched, _ := regexp.MatchString("\\d{4,50}", col[i])
					if matchString {
						// 设备接口数据
						//fmt.Println("====", col[i])
						getOpmDetail(sheet, col[i])
					}
					if matched {
						//fmt.Println("+++++++", col[i])
						getMonitorData(col[i])
					}

				}
			}
		}
	}

}

func Schedule() {
	c := cron.New()
	c.AddFunc(cfg.CronStr, func() {
		readExcel()
	})
	c.Start()
	select {}
}

func main() {

	//Schedule()

	// 1.获取excel数据
	startT := time.Now()
	//加载excel
	readExcel()
	tc := time.Since(startT)
	fmt.Println(" 耗时: ", tc)

	//
	//mysqlDb = initMysql(usename, password, serverIp, port, dbname)
	//
	//data := "{\"performanceMonitors\":{\"total\":6,\"statusIcon\":\"images/servicestatus5.gif\",\"down\":0,\"monitors\":[{\"DISPLAYNAME\":\"磁盘利用率\",\"graphType\":\"multiple\",\"checkNumeric\":\"true\",\"data\":[{\"instance\":\"-1\",\"value\":48}],\"policyName\":\"DiskUtilization\",\"Instance\":\"\",\"THRESHOLDENABLED\":\"false\",\"GRAPHID\":\"37\",\"thresholdStatus\":\"未启用\",\"isMultiple\":\"true\",\"sSave\":\"true\",\"type\":\"multiple\",\"thresholdImg\":\"images/thresholdNonConfigured.gif\",\"lastCollectedTime\":\"2022-9-29 10:54:05\",\"DisplayColumn\":\"\",\"pollId\":\"42577\",\"name\":\"DiskUtilization\",\"numericType\":\"1\",\"interval\":\"60\",\"Protocol\":\"SNMP\"},{\"DISPLAYNAME\":\"设备的分区明细(%)-C:\\\\ Label:  Serial Number 62efda9a\",\"graphType\":\"multiplenode\",\"checkNumeric\":\"true\",\"data\":[{\"instance\":\"-1\",\"value\":62}],\"policyName\":\"PartitionWiseDiskDetails\",\"Instance\":\"\",\"THRESHOLDENABLED\":\"true\",\"GRAPHID\":\"280\",\"thresholdStatus\":\"正常\",\"isMultiple\":\"false\",\"sSave\":\"true\",\"type\":\"node\",\"thresholdImg\":\"images/thresholdConOK.gif\",\"lastCollectedTime\":\"2022-9-29 11:07:08\",\"DisplayColumn\":\".1.3.6.1.2.1.25.2.3.1.3\",\"pollId\":\"42579\",\"name\":\"C:\\\\ Label:  Serial Number 62efda9a\",\"numericType\":\"1\",\"interval\":\"5\",\"Protocol\":\"SNMP\"},{\"DISPLAYNAME\":\"进程计数\",\"graphType\":\"node\",\"checkNumeric\":\"true\",\"data\":[{\"instance\":\"-1\",\"value\":62}],\"policyName\":\"ProcessCount\",\"Instance\":\"\",\"THRESHOLDENABLED\":\"false\",\"GRAPHID\":\"375\",\"thresholdStatus\":\"未启用\",\"isMultiple\":\"false\",\"sSave\":\"true\",\"type\":\"node\",\"thresholdImg\":\"images/thresholdNonConfigured.gif\",\"lastCollectedTime\":\"2022-9-29 11:03:56\",\"DisplayColumn\":\"\",\"pollId\":\"42578\",\"name\":\"ProcessCount\",\"numericType\":\"1\",\"interval\":\"15\",\"Protocol\":\"SNMP\"},{\"DISPLAYNAME\":\"设备的分区明细(%)-D:\\\\ Label:新加卷  Serial Number c6375300\",\"graphType\":\"multiplenode\",\"checkNumeric\":\"true\",\"data\":[{\"instance\":\"-1\",\"value\":21}],\"policyName\":\"PartitionWiseDiskDetails\",\"Instance\":\"\",\"THRESHOLDENABLED\":\"true\",\"GRAPHID\":\"280\",\"thresholdStatus\":\"正常\",\"isMultiple\":\"false\",\"sSave\":\"true\",\"type\":\"node\",\"thresholdImg\":\"images/thresholdConOK.gif\",\"lastCollectedTime\":\"2022-9-29 11:07:08\",\"DisplayColumn\":\".1.3.6.1.2.1.25.2.3.1.3\",\"pollId\":\"42580\",\"name\":\"D:\\\\ Label:新加卷  Serial Number c6375300\",\"numericType\":\"1\",\"interval\":\"5\",\"Protocol\":\"SNMP\"},{\"DISPLAYNAME\":\"CPU利用率\",\"graphType\":\"multiple\",\"checkNumeric\":\"true\",\"data\":[{\"instance\":\"-1\",\"value\":1}],\"policyName\":\"Win-CPUUtilization\",\"Instance\":\"\",\"THRESHOLDENABLED\":\"true\",\"GRAPHID\":\"35\",\"thresholdStatus\":\"正常\",\"isMultiple\":\"true\",\"sSave\":\"true\",\"type\":\"multiple\",\"thresholdImg\":\"images/thresholdConOK.gif\",\"lastCollectedTime\":\"2022-9-29 11:07:08\",\"DisplayColumn\":\"\",\"pollId\":\"42581\",\"name\":\"Win-CPUUtilization\",\"numericType\":\"1\",\"interval\":\"5\",\"Protocol\":\"SNMP\"},{\"DISPLAYNAME\":\"内存利用率\",\"graphType\":\"multiple\",\"checkNumeric\":\"true\",\"data\":[{\"instance\":\"-1\",\"value\":20}],\"policyName\":\"Win-MemoryUtilization\",\"Instance\":\"\",\"THRESHOLDENABLED\":\"true\",\"GRAPHID\":\"36\",\"thresholdStatus\":\"正常\",\"isMultiple\":\"true\",\"sSave\":\"true\",\"type\":\"multiple\",\"thresholdImg\":\"images/thresholdConOK.gif\",\"lastCollectedTime\":\"2022-9-29 11:07:08\",\"DisplayColumn\":\"\",\"pollId\":\"42585\",\"name\":\"Win-MemoryUtilization\",\"numericType\":\"1\",\"interval\":\"5\",\"Protocol\":\"SNMP\"}]},\"isVM\":false,\"isHost\":false,\"serverMonitors\":{\"total\":0,\"down\":0,\"monitors\":[]},\"ntServiceMonitors\":{\"total\":0,\"statusIcon\":\"images/servicestatus5.gif\",\"down\":0,\"monitors\":[]},\"urlMonitors\":{\"total\":0,\"statusIcon\":\"images/servicestatus5.gif\",\"down\":0,\"monitors\":[]},\"processMonitors\":{\"total\":0,\"statusIcon\":\"images/servicestatus5.gif\",\"down\":0,\"monitors\":[]},\"fileMonitors\":{\"total\":0,\"statusIcon\":\"images/servicestatus5.gif\",\"down\":0,\"monitors\":[]},\"eventlogMonitors\":{\"period\":\"300\",\"total\":0,\"statusIcon\":\"images/servicestatus5.gif\",\"isActive\":\"false\",\"down\":0,\"monitors\":[]},\"folderMonitors\":{\"total\":0,\"statusIcon\":\"images/servicestatus5.gif\",\"down\":0,\"monitors\":[]},\"scriptMonitors\":{\"total\":0,\"statusIcon\":\"images/servicestatus5.gif\",\"down\":0,\"monitors\":[]}}\n"
	//m := make(map[string]interface{})
	//err := json.Unmarshal([]byte(data), &m)
	//if err != nil {
	//	fmt.Printf("反序列化错误 err=%v\n", err)
	//}
	//m["host"] = "100.100.13"
	//jsonStr, err := json.Marshal(m)
	//
	//fmt.Println(jsonStr)
	//for _, arr := range array {
	//	policyName := gjson.Get(arr.Raw, "policyName").String()
	//	displayName := gjson.Get(arr.Raw, "DISPLAYNAME").String()
	//	createMetric("102",policyName,displayName,policyName,"ip","ip"+policyName)
	//	createObjectMetricRelation("102","ip", "ip_"+policyName)
	//	_type := gjson.Get(arr.Raw, "TYPE").String()
	//	resourceId := gjson.Get(arr.Raw, "RESOURCEID").String()
	//	createObject(_type, displayName, resourceId)
	//	attributes := gjson.Get(arr.Raw, "Attribute").Array()
	//	for _, attribute := range attributes {
	//		attributeName := strings.Replace(gjson.Get(attribute.Raw, "DISPLAYNAME").Raw,"\"","",2)
	//		attributeId := gjson.Get(attribute.Raw, "AttributeID").String()
	//		createMetric(attributeName,displayName+"_"+attributeName,resourceId+"_"+attributeId,attributeId,resourceId+"_"+attributeId)
	//		createObjectMetricRelation(resourceId, resourceId+"_"+attributeId)
	//	}
	//	childMonitors := gjson.Get(arr.Raw, "CHILDMONITORS").Array()
	//	for _, childMonitor := range childMonitors {
	//		monitorName := gjson.Get(childMonitor.Raw, "DISPLAYNAME").String()
	//		childMonitorInfos := gjson.Get(childMonitor.Raw, "CHILDMONITORINFO").Array()
	//		for _, childMonitorInfo := range childMonitorInfos {
	//			childMonitorInfoName := strings.Replace(gjson.Get(childMonitorInfo.Raw, "DISPLAYNAME").Raw,"\"","",2)
	//			childMonitorInfoResourceId := gjson.Get(childMonitorInfo.Raw, "RESOURCEID").String()
	//			childAttributes := gjson.Get(childMonitorInfo.Raw, "CHILDATTRIBUTES").Array()
	//			for _, childAttribute := range childAttributes {
	//				childAttributeName := strings.Replace(gjson.Get(childAttribute.Raw, "DISPLAYNAME").Raw,"\"","",2)
	//				childAttributeId := gjson.Get(childAttribute.Raw, "AttributeID").String()
	//				type1 := monitorName+"_"+childAttributeName
	//				name := displayName+"_"+childMonitorInfoName+"_"+monitorName+"_"+childAttributeName
	//				key := resourceId+"_"+childMonitorInfoResourceId+"_"+childAttributeId
	//				metricId := childMonitorInfoResourceId+"_"+childAttributeId
	//				unMetricid := resourceId+"_"+childMonitorInfoResourceId+"_"+childAttributeId
	//				createMetric(type1,name,key,metricId, unMetricid)
	//				createObjectMetricRelation(resourceId, resourceId+"_"+childMonitorInfoResourceId+"_"+childAttributeId)
	//			}
	//		}
	//	}
	//}

}
