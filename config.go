package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
)

type BenchmarkConfig struct {
	LogLineSize           int      `json:"log_line_size"`
	NumActiveLogFiles     int      `json:"num_active_log_files"`
	EnableRandom          bool     `json:"enable_random"`
	RandomLineSize        []int    `json:"random_line_size"`
	RandomWriteWait       []int    `json:"random_write_wait"`
	LogFilesBaseDir       string   `json:"log_files_base_dir"`
	WriteWaitPeriodMs     int      `json:"write_wait_period_ms"`
	LogShipperName        string   `json:"log_shipper_name"`
	LogShipperProcessName string   `json:"log_shipper_process_name"`
	ModuleDir             string   `json:"module_dir"`
	ModuleName            string   `json:"module_name"`
	LogShipperBinPath     string   `json:"log_shipper_bin_path"`
	LogShipperFlags       string   `json:"log_shipper_flags"`
	MetricsDir            string   `json:"metrics_dir"`
	WorkingDir            string   `json:"working_dir"`
	MaxProcs              int      `json:"max_procs"`
	CustomLogEntry        string   `json:"custom_log_entry"`
	KafkaBrokerList       []string `json:"kafka_broker_list"`
	TotalRunTimeSeconds   int64    `json:"total_run_time_seconds"`
	EnableCgroupLimit        bool     `json:"enable_cgroup_limit"`
	CgroupCpuRuntime      int64    `json:"cgroup_cpu_runtime"` // >= cgroup_cpu_period
	CgroupCpuPeriod       uint64   `json:"cgroup_cpu_period"`  // out of a max of 1 000 000 microseconds
}

func LoadConfig(confPath string) *BenchmarkConfig {
	confFile, err := os.Open(confPath)
	if err != nil {
		fmt.Printf("[ERROR] Could not open %s: %s\n", confPath, err)
	}
	defer confFile.Close()
	byteValue, err := ioutil.ReadAll(confFile)
	if err != nil {
		fmt.Printf("[ERROR] Could not read %s: %s\n", confPath, err)
	}
	var conf BenchmarkConfig
	if err := json.Unmarshal(byteValue, &conf); err != nil {
		fmt.Println("[ERROR] Could not parse JSON: ", err)
		os.Exit(1)
	}
	return &conf
}
