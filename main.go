package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/docker/docker/client"
	"io"
	"log"
	"strings"
	"sync"
	"time"
)

var containersRaw string
var containers []string
var parserCount int
var printerCount int
var iterations int
var delay int

func main() {
	flag.StringVar(&containersRaw, "Containers", "", "Container ids. Separated by comma.")
	flag.IntVar(&parserCount, "Parsers", 1, "Number of parsers.")
	flag.IntVar(&printerCount, "Printers", 1, "Number of printers.")
	flag.IntVar(&iterations, "Iterations", 5, "Number of iterations. -1 for infinite.")
	flag.IntVar(&delay, "Delay", -1, "Query delay in milliseconds. -1 no delay")

	flag.Parse()
	containers = strings.Split(containersRaw, ",")

	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatal(err)
	}
	defer func(cli *client.Client) {
		err := cli.Close()
		if err != nil {
			log.Fatal(err)
		}
	}(cli)

	var printerWaitGroup sync.WaitGroup
	var parserWaitGroup sync.WaitGroup
	var queryWaitGroup sync.WaitGroup

	printerWaitGroup.Add(printerCount)
	parserWaitGroup.Add(parserCount)
	queryWaitGroup.Add(len(containers))

	dataChan := make(chan string, len(containers))
	parsedDataChan := make(chan ContainerData, len(containers))

	for i := 0; i < len(containers); i++ {
		if len(containers) < 20 {
			go QueryContainerStream(containers[i], dataChan, ctx, cli, &queryWaitGroup)
		} else {
			go QueryContainer(containers[i], dataChan, ctx, cli, &queryWaitGroup)
		}
	}
	for i := 0; i < parserCount; i++ {
		go ParseData(dataChan, parsedDataChan, &parserWaitGroup)
	}
	for i := 0; i < printerCount; i++ {
		go PrintData(parsedDataChan, &printerWaitGroup)
	}

	queryWaitGroup.Wait()
	close(dataChan)
	parserWaitGroup.Wait()
	close(parsedDataChan)
	printerWaitGroup.Wait()
}

func PrintData(parsedDataChan <-chan ContainerData, waitgroup *sync.WaitGroup) {
	for value := range parsedDataChan {
		cpuDelta := value.CPUStats.CPUUsage.TotalUsage - value.PrecpuStats.CPUUsage.TotalUsage
		systemCpuDelta := value.CPUStats.SystemCPUUsage - value.PrecpuStats.SystemCPUUsage
		cpuUsage := (cpuDelta / systemCpuDelta) * int64(value.CPUStats.OnlineCpus) * 100.0

		usedMemory := value.MemoryStats.Usage - value.MemoryStats.Stats.Cache
		memoryUsage := (usedMemory / value.MemoryStats.Limit) * 100.0

		fmt.Printf("Name: %s CPU: %d%% Memory(%%): %d Memory(MB): %d\n", value.Name, cpuUsage, memoryUsage, usedMemory/1000000)
	}

	waitgroup.Done()
}

func ParseData(dataChan <-chan string, parsedDataChan chan<- ContainerData, waitgroup *sync.WaitGroup) {
	var parsedData ContainerData
	for value := range dataChan {
		err := json.Unmarshal([]byte(value), &parsedData)
		if err != nil {
			fmt.Println("Failed to parse")
			continue
		}
		parsedDataChan <- parsedData
	}

	waitgroup.Done()
}

func QueryContainerStream(
	container string,
	dataChan chan<- string,
	ctx context.Context,
	cli *client.Client,
	waitgroup *sync.WaitGroup,
) {
	containerStats, _ := cli.ContainerStats(ctx, container, true)

	buf := make([]byte, 1)
	buf2 := make([]byte, 0)
	for i := 0; i < iterations; i++ {

		for !json.Valid(buf2) || len(buf2) < 5 {
			_, err := io.ReadFull(containerStats.Body, buf)
			if err != nil {
				log.Fatal(err)
			}
			buf2 = append(buf2, buf...)
		}

		dataChan <- string(buf2)

		buf = make([]byte, 1)
		buf2 = make([]byte, 0)
		if iterations == -1 {
			i = -2
		}
	}

	waitgroup.Done()
}

func QueryContainer(
	container string,
	dataChan chan<- string,
	ctx context.Context,
	cli *client.Client,
	waitgroup *sync.WaitGroup,
) {
	buf := new(bytes.Buffer)

	for i := 0; i < iterations; i++ {
		containerStats, err := cli.ContainerStats(ctx, container, false)
		if err != nil {
			log.Fatal(err)
		}
		_, err = buf.ReadFrom(containerStats.Body)
		if err != nil {
			log.Fatal(err)
		}

		dataChan <- buf.String()
		buf.Reset()

		if iterations == -1 {
			i = -2
		}
	}

	waitgroup.Done()
}

type ContainerData struct {
	Read      time.Time `json:"read"`
	Preread   time.Time `json:"preread"`
	PidsStats struct {
		Current int `json:"current"`
	} `json:"pids_stats"`
	BlkioStats struct {
		IoServiceBytesRecursive []interface{} `json:"io_service_bytes_recursive"`
		IoServicedRecursive     []interface{} `json:"io_serviced_recursive"`
		IoQueueRecursive        []interface{} `json:"io_queue_recursive"`
		IoServiceTimeRecursive  []interface{} `json:"io_service_time_recursive"`
		IoWaitTimeRecursive     []interface{} `json:"io_wait_time_recursive"`
		IoMergedRecursive       []interface{} `json:"io_merged_recursive"`
		IoTimeRecursive         []interface{} `json:"io_time_recursive"`
		SectorsRecursive        []interface{} `json:"sectors_recursive"`
	} `json:"blkio_stats"`
	NumProcs     int `json:"num_procs"`
	StorageStats struct {
	} `json:"storage_stats"`
	CPUStats struct {
		CPUUsage struct {
			TotalUsage        int64 `json:"total_usage"`
			PercpuUsage       []int `json:"percpu_usage"`
			UsageInKernelmode int   `json:"usage_in_kernelmode"`
			UsageInUsermode   int   `json:"usage_in_usermode"`
		} `json:"cpu_usage"`
		SystemCPUUsage int64 `json:"system_cpu_usage"`
		OnlineCpus     int   `json:"online_cpus"`
		ThrottlingData struct {
			Periods          int `json:"periods"`
			ThrottledPeriods int `json:"throttled_periods"`
			ThrottledTime    int `json:"throttled_time"`
		} `json:"throttling_data"`
	} `json:"cpu_stats"`
	PrecpuStats struct {
		CPUUsage struct {
			TotalUsage        int64 `json:"total_usage"`
			PercpuUsage       []int `json:"percpu_usage"`
			UsageInKernelmode int   `json:"usage_in_kernelmode"`
			UsageInUsermode   int   `json:"usage_in_usermode"`
		} `json:"cpu_usage"`
		SystemCPUUsage int64 `json:"system_cpu_usage"`
		OnlineCpus     int   `json:"online_cpus"`
		ThrottlingData struct {
			Periods          int `json:"periods"`
			ThrottledPeriods int `json:"throttled_periods"`
			ThrottledTime    int `json:"throttled_time"`
		} `json:"throttling_data"`
	} `json:"precpu_stats"`
	MemoryStats struct {
		Usage    int64 `json:"usage"`
		MaxUsage int   `json:"max_usage"`
		Stats    struct {
			ActiveAnon              int   `json:"active_anon"`
			ActiveFile              int   `json:"active_file"`
			Cache                   int64 `json:"cache"`
			Dirty                   int   `json:"dirty"`
			HierarchicalMemoryLimit int64 `json:"hierarchical_memory_limit"`
			HierarchicalMemswLimit  int64 `json:"hierarchical_memsw_limit"`
			InactiveAnon            int   `json:"inactive_anon"`
			InactiveFile            int   `json:"inactive_file"`
			MappedFile              int   `json:"mapped_file"`
			Pgfault                 int   `json:"pgfault"`
			Pgmajfault              int   `json:"pgmajfault"`
			Pgpgin                  int   `json:"pgpgin"`
			Pgpgout                 int   `json:"pgpgout"`
			Rss                     int   `json:"rss"`
			RssHuge                 int   `json:"rss_huge"`
			TotalActiveAnon         int   `json:"total_active_anon"`
			TotalActiveFile         int   `json:"total_active_file"`
			TotalCache              int   `json:"total_cache"`
			TotalDirty              int   `json:"total_dirty"`
			TotalInactiveAnon       int   `json:"total_inactive_anon"`
			TotalInactiveFile       int   `json:"total_inactive_file"`
			TotalMappedFile         int   `json:"total_mapped_file"`
			TotalPgfault            int   `json:"total_pgfault"`
			TotalPgmajfault         int   `json:"total_pgmajfault"`
			TotalPgpgin             int   `json:"total_pgpgin"`
			TotalPgpgout            int   `json:"total_pgpgout"`
			TotalRss                int   `json:"total_rss"`
			TotalRssHuge            int   `json:"total_rss_huge"`
			TotalUnevictable        int   `json:"total_unevictable"`
			TotalWriteback          int   `json:"total_writeback"`
			Unevictable             int   `json:"unevictable"`
			Writeback               int   `json:"writeback"`
		} `json:"stats"`
		Limit int64 `json:"limit"`
	} `json:"memory_stats"`
	Name     string `json:"name"`
	ID       string `json:"id"`
	Networks struct {
		Eth0 struct {
			RxBytes   int `json:"rx_bytes"`
			RxPackets int `json:"rx_packets"`
			RxErrors  int `json:"rx_errors"`
			RxDropped int `json:"rx_dropped"`
			TxBytes   int `json:"tx_bytes"`
			TxPackets int `json:"tx_packets"`
			TxErrors  int `json:"tx_errors"`
			TxDropped int `json:"tx_dropped"`
		} `json:"eth0"`
	} `json:"networks"`
}
