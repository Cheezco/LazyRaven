package main

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
)

var storedData []ComputedData
var data [][]ComputedData

func StartApi(storedDataChan chan ComputedData) {
	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	go storeData(storedDataChan)
	storedData = []ComputedData{}
	data = [][]ComputedData{}

	router.GET("/metrics", getData)
	err := router.Run("localhost:3030")
	if err != nil {
		panic(err)
	}
}

func storeData(storedDataChan chan ComputedData) {
	for value := range storedDataChan {
		storedData = append([]ComputedData{value}, storedData...)
		if len(storedData) > 5 {
			storedData = storedData[:len(storedData)-1]
		}
	}
	//for value := range storedDataChan {
	//	foundId := -1
	//	for arr := range data {
	//		if data[arr][0].Name != value.Name {
	//			continue
	//		}
	//		foundId = arr
	//	}
	//	if foundId == -1 {
	//		data = append(data, []ComputedData{value})
	//	} else {
	//		arr := data[foundId]
	//
	//	}
	//}
}

// Don't judge me

func getPrometheusDataResponse(data ComputedData) string {
	return fmt.Sprintf(`
# HELP CpuUsagePercentage
# TYPE raven_%s_cpu_usage_percentage gauge
raven_%s_cpu_usage_percentage %f
# HELP MemoryUsagePercentage
# TYPE raven_%s_memory_usage_percentage gauge
raven_%s_memory_usage_percentage %f
# HELP MemoryUsageBytes
# TYPE raven_%s_memory_usage_bytes gauge
raven_%s_memory_usage_bytes %d
# HELP Energy usage in watts
# TYPE raven_%s_energy_usage gauge
raven_%s_energy_usage %f`,
		data.Name,
		data.Name,
		data.CpuUsagePerc,
		data.Name,
		data.Name,
		data.MemoryUsagePerc,
		data.Name,
		data.Name,
		data.UsedMemory,
		data.Name,
		data.Name,
		data.CpuEnergy)
}

func getPrometheusDataResponseArr(data []ComputedData) string {
	result := ""
	uniqueData := make([]ComputedData, 0)
	encountered := make(map[string]bool)

	for _, value := range data {
		if !encountered[value.Name] {
			encountered[value.Name] = true
			uniqueData = append(uniqueData, value)
		}
	}

	for _, value := range uniqueData {
		result += getPrometheusDataResponse(value)
	}

	return result
}

func getData(c *gin.Context) {
	//c.IndentedJSON(http.StatusOK, storedData)

	if len(storedData) == 0 {
		c.Status(http.StatusNotFound)
	}

	c.String(http.StatusOK, getPrometheusDataResponseArr(storedData))
}
