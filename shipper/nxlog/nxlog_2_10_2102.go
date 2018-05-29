package main

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"sync"
	"syscall"
	"text/template"

	utils "github.com/hartfordfive/logshipper-benchmark/lib"
)

var Debug bool = false

const SupportedShipperVersionMajor = 2
const SupportedShipperVersionMinor = 10
const SupportedShipperVersionPatch = 2102

// Configuration documenation for nxlog can be found here:
//	http://nxlog-ce.sourceforge.net/nxlog-docs/en/nxlog-reference-manual.html

var configTpl = `
#######################################
# Global directives #
########################################
User nxlog
Group nxlog
PidFile ~/nxlog.pid
#LogFile /var/log/nxlog/nxlog.log
#LogLevel INFO

########################################
# Modules #
########################################
{{range $index, $file := .FilesToMonitor}}
<Input inFile{{$index}}>
  Module im_file
  File "{{$file}}"
  SavePos TRUE
  Recursive TRUE
</Input>

{{end}}

<Output outKafka>
  Module om_kafka
  BrokerList {{range $index, $broker := .KafkaBrokers}}{{if $index}},{{end}}{{$broker}}{{end}}
  Topic {{ .KafkaTopic}}
  #-- Partition <number> - defaults to RD_KAFKA_PARTITION_UA
  #-- Compression, one of none, gzip, snappy
  Compression none
</Output>

########################################
# Routes #
########################################
<Route 1>
{{range $index, $file := .FilesToMonitor}}
  Path inFile{{$index}} => outKafka
{{end}}
</Route>

`

type config struct {
	KafkaBrokers   []string
	KafkaTopic     string
	FilesToMonitor []string
}

type shipper struct{}

func (s shipper) Name() string { return "nxlog" }

func (s shipper) CleanupFiles() {
	if _, err := os.Stat("nxlog.conf"); err == nil {
		os.Remove("nxlog.conf")
	}

}

func (s shipper) BuildConfig(confDestPath string, filesToMonitor []string, kafkTopicName string, kafkaBrokersList []string) {

	s.CleanupFiles()

	confBaseName := path.Base(confDestPath)
	t := template.Must(template.New(fmt.Sprintf("%s.tpl", confBaseName)).Parse(configTpl))

	fh, err := os.Create(confDestPath)
	if err != nil {
		fmt.Println("[ERROR] Could not create config: ", err)
		os.Exit(1)
	}
	defer fh.Close()

	err = t.Execute(fh, &config{
		KafkaBrokers:   kafkaBrokersList,
		KafkaTopic:     kafkTopicName,
		FilesToMonitor: filesToMonitor,
	})

	if err != nil {
		panic(err)
	}
}

func (s shipper) Run(binPath string, cmdArgs []string, filesToMonitor []string, kafkBrokers []string, execChan chan *exec.Cmd, shutdownChan chan bool, wg *sync.WaitGroup) {

	// First generate, the filebeat config
	s.BuildConfig(
		"nxlog.conf",
		filesToMonitor,
		fmt.Sprintf("dev-logs-shipper-benchmarks-%s", s.Name()),
		kafkBrokers,
	)

	// Now run the binary
	cmd := exec.Command(binPath, cmdArgs...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// var stdout, stderr bytes.Buffer
	// cmd.Stdout = &stdout
	// cmd.Stderr = &stderr
	err := cmd.Start()

	go utils.CollectCpuStats(cmd.Process.Pid, "metrics/"+s.Name(), shutdownChan)

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
		err := cmd.Process.Signal(os.Interrupt)
		if err != nil {
			fmt.Printf("[ERROR] Could not shutdown shipper: %s\n", err.Error())
		} else {
			fmt.Printf("[INFO] %s has been shut down.\n", s.Name())
		}
	}(shutdownChan, cmd)

	fmt.Printf("[INFO] %s is now running.\n", s.Name())
	cmd.Wait()

}

func InitShipper() (s interface{}, err error) {
	s = shipper{}
	return
}
