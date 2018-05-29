package main

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
	"sync"
	"syscall"
	"text/template"

	utils "github.com/hartfordfive/logshipper-benchmark/lib"
)

var Debug bool = false

const supportedShipperVersionMajor = 6
const supportedShipperVersionMinor = 1
const supportedShipperVersionPatch = 1

const configTpl = `
---
node.name: ${HOSTNAME:logstash01}
pipeline.workers: 2
config.reload.automatic: true
config.reload.interval: 30s
config.debug: false
dead_letter_queue.enable: false
http.host: 127.0.0.1
http.port: 9600
log.level: error

`

const pipelineTpl = `

input {
{{range $index, $file := .FilesToMonitor}}
  file { 
    path => "{{$file}}"
    start_position => "beginning"
  }
{{end}}
}

output {
    kafka {
        bootstrap_servers => "{{range $index, $broker := .KafkaBrokers}}{{if $index}},{{end}}{{$broker}}{{end}}"
        topic_id => "{{.KafkaTopic}}"
        codec => "json"
    }
}

`

type config struct {
	KafkaBrokers   []string
	KafkaTopic     string
	FilesToMonitor []string
}

type shipper struct{}

func (s shipper) Name() string { return "logstash" }

func (s shipper) CleanupFiles() {

	files := []string{"logstash.yml"}
	for _, f := range files {
		if _, err := os.Stat(f); err == nil {
			os.Remove(f)
		}
	}

}

func (s shipper) BuildConfig(confDestPath string, filesToMonitor []string, kafkTopicName string, kafkaBrokersList []string) {

	s.CleanupFiles()

	confBaseName := path.Base(confDestPath)
	confBaseDir := path.Dir(confDestPath)

	// ---------------- Write the pipeline config --------------
	t := template.Must(template.New(fmt.Sprintf("%s.tpl", confBaseName)).Parse(pipelineTpl))
	fmt.Printf("[INFO] Writing pipeline to: %s/main.conf\n", confBaseDir)
	fh, err := os.Create(confBaseDir + "/main.conf")
	if err != nil {
		fmt.Println("[ERROR] Could not create config: ", err)
		os.Exit(1)
	}

	err = t.Execute(fh, &config{
		KafkaBrokers:   kafkaBrokersList,
		KafkaTopic:     kafkTopicName,
		FilesToMonitor: filesToMonitor,
	})
	fh.Close()

	if err != nil {
		panic(err)
	}

	// ---------------- Write the logstash.yml config
	t = template.Must(template.New(fmt.Sprintf("%s.tpl", confBaseName)).Parse(configTpl))
	fmt.Printf("[INFO] Writing config to: %s/logstash.yml\n", confBaseDir)
	fh, err = os.Create(confBaseDir + "/logstash.yml")
	if err != nil {
		fmt.Println("[ERROR] Could not create config: ", err)
		os.Exit(1)
	}
	err = t.Execute(fh, &config{})
	fh.Close()

	if err != nil {
		panic(err)
	}
}

func (s shipper) Run(binPath string, cmdArgs []string, workingDir string, filesToMonitor []string, kafkBrokers []string, execChan chan *exec.Cmd, shutdownChan chan bool, wg *sync.WaitGroup) {

	// First generate, the config
	s.BuildConfig(
		fmt.Sprintf("%s/logstash.conf", strings.TrimRight(workingDir, "/")),
		filesToMonitor,
		fmt.Sprintf("dev-logs-shipper-benchmarks-%s", s.Name()),
		kafkBrokers,
	)

	// Ensure the working directory exists, if not create it
	//workingDir :=  fmt.Sprintf("%s/benchmarks/%s/", strings.TrimRight(utils.GetCwd(), "/"), s.Name())
	if Debug {
		fmt.Println("[DEBUG] Changing to working dir: ", workingDir)
	}
	//utils.CreateDir(workingDir)

	// Now run filebeat
	cmd := exec.Command(binPath, cmdArgs...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Dir = workingDir
	cmd.Env = append(cmd.Env, fmt.Sprintf("LOGSTASH_HOME=%", workingDir))
	cmd.Env = append(cmd.Env, fmt.Sprintf("LS_HOME=%", workingDir))
	// var stdout, stderr bytes.Buffer
	// cmd.Stdout = &stdout
	// cmd.Stderr = &stderr
	err := cmd.Start()

	go utils.CollectCpuStats(cmd.Process.Pid, fmt.Sprintf("%s/metrics/%s", strings.TrimRight(workingDir, "/"), s.Name()), shutdownChan)

	if err != nil {
		fmt.Printf("[ERROR] Could not run %s: %s\n", s.Name(), err)
		os.Exit(1)
	}

	execChan <- cmd

	// outStr, errStr := string(stdout.Bytes()), string(stderr.Bytes())
	// fmt.Printf("out:\n%s\nerr:\n%s\n", outStr, errStr)

	go func(shudownChan <-chan bool, cmd *exec.Cmd) {
		fmt.Printf("[INFO] Waiting for signal to shutdown %s...\n", s.Name())
		<-shutdownChan
		fmt.Printf("[INFO] Terminating shipper...\n")
		// err := cmd.Process.Signal(os.Interrupt)
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		if err != nil {
			fmt.Printf("[ERROR] Could not shutdown shipper: %s\n", err.Error())
		} else {
			fmt.Printf("[INFO] %s has been shut down.\n", s.Name())
		}
		s.CleanupFiles()
	}(shutdownChan, cmd)

	fmt.Printf("[INFO] %s is now running.\n", s.Name())
	cmd.Wait()
}

func (s shipper) GetVersion() string {
	return fmt.Sprintf("%d.%d.%d", supportedShipperVersionMajor, supportedShipperVersionMinor, supportedShipperVersionPatch)
}

func InitShipper() (s interface{}, err error) {
	s = shipper{}
	return
}
