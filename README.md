# Log Shipper Benchmarks

## Description

The role of this application is to provide the ability to easily benchmark the file input capabilities of various log shipper clients to a Kafka output destination. 
Metrics from the log shippers are collected via metricbeat.


## Requirments/Dependencies

Go:
----
- Must be running Go >= 1.8.x, although was built with 1.10.0
- Ensure `GOPATH` is set to `$HOME/go` and that the directory exists
- Ensure `$HOME/go/bin` is part of you `PATH` environment variable
- Require go vendoring library: `github.com/kardianos/govendor` (`go get github.com/kardianos/govendor`)

For currently included modules:
----
- [Fluent Bit](https://fluentbit.io/documentation/current/installation/redhat_centos.html): v0.13.0
- [Filebeat](https://www.elastic.co/guide/en/beats/filebeat/6.1/filebeat-installation.html) : v6.1.1
- [Logstash](https://www.elastic.co/guide/en/logstash/6.1/installing-logstash.html) : v6.1.1
- [Rsyslogd](https://www.rsyslog.com/rhelcentos-rpms/) : v8.34.0

Go Packages:
----
- github.com/kardianos/govendor
- github.com/c9s/goprocinfo/linux 
- github.com/Pallinder/go-randomdata
- github.com/mitchellh/go-ps

Metrics Collection:
----
- [Metricbeat](https://www.elastic.co/guide/en/beats/metricbeat/6.1/metricbeat-installation.html): v6.1.1

## Building from source

```
make deps
make clean
make all
```


## Configuration

The config, which is in JSON format, should contain the following fields:
- `additional_metricbeat_fields` : An object consisting of additional key/value properties to add the the metricbeat data. (Type: map[string]string, Default: <empty>)
- `custom_log_entry` : If set, the this specific log entry will be written to the files instead of a randomly generated one. (Type: string, Default: <empty>)
- `enable_random` : If set to true, the application will randomly choose a line size and wait time between writes. (Type: boolean, Default: false)
- `kafka_broker_list` : List of Kafka broker hostnames (HOST:PORT) to use in the log shippers. Currently only this output destination is supported. (Type: []string, Default)
- `log_files_base_dir` : Location where the sample log files will be created (Type: string, Default: <empty>)
- `log_line_size` : The size (character length) of the log entry to be randomly generated. (Type: int, Default: 50)
- `log_shipper_bin_path` : The path to the log shipper binary. (Type: string, Default: <empty>)
- `log_shipper_flags` :  The flags to use when executing the log shipper binary. (Type: string, Default: <empty>)
- `log_shipper_name` : The name of the log shipper. (Type: string, Default: <empty>)
- `log_shipper_process_name` : The running process name of the log shipper. (Type: string, Default: <empty>)
- `max_procs` : The max number of processors this benchmarking app should use. (Type: int, Default: <empty>)
- `metrics_dir` : The directory in which the collected process metrics will be stored. (Type: string, Default: <empty>)
- `module_dir` : The directory in which the `.so` shipper module is found. (Type: string, Default: <empty>)
- `module_name` : The filename of the module excluding the `.so` extension. (Type: string, Default: <empty>)
- `num_active_log_files` : The number of active log files that will be written to concurrently/in-parallel. (Type: int, Default: 10)
- `random_line_size` : The MIN,MAX range for the length of the line in characters. (Type []int, Default: <empty>)
- `random_write_wait` : The MIN,MAX range for period (in milliseconds) bewteen writes to each individual log files. (Type []int, Default: <empty>)
- `total_run_time_seconds` : The total time (in seconds) to run the benchmark. (Type int, Default: <empty>)
- `working_dir` :  The working directory in which the module will be running. (Type: string, Default: <empty>)
- `write_wait_period_ms` : The period (in milliseconds) bewteen writes to the each individual log files.  (Type int, Default: <empty>)

Samples can be found in the [_sample_configs](_sample_configs/) directory.

## Implementing additional shippers

The individual shippers work with a plugin based system.  In order to benchmark a new shipper, you must create a plugin that respects the following interface
```
type Shipper interface {
        Name()
        Run(binPath string, cmdArgs []string, workingDir string, filesToMonitor []string, kafkaBrokers []string, filebeatExec chan *exec.Cmd, terminate chan bool, wg *sync.WaitGroup)
        CleanupFiles()
        BuildConfig(confDestPath string, filesToMonitor []string, kafkTopicName string, kafkaBrokersList []string)
        GetVersion() string
}
```

To compile the plugin manually, run the following command:
```
go build -a -v -buildmode=plugin -o output/path/to/plugin.so source/path/to/plugin.go
```


## Running the benchmarks:

The benchmarks for a given log shipper are executed as follows:
```
./logshipper-benchmark [PATH_TO_CONFIG]
```

To terminate the benchmark, simply hit `Ctrl+C`.  If the process is backgrounded, to initial a clean shutdown, you must kill the `logshipper-benchmark` with a `SIGINT` or `SIGTERM` signal.
For example:
```
kill -INT [PID]
```

## Considerations

This application was developed quickly to have the ability to efficiently create benchmarks for a variety of log shippers.  Having said that, it's certain this code isn't optimal and may likely contain bugs.  If you find any bugs/performance improvements to this application, feel free to create a bug report or issue a PR.  If you have any additional log shippers you'd like to test, or even different version of them, a PR for thew new log shipper would be much appreciated.


## Author

Alain Lefebvre <hartfordfive@gmail.com>
