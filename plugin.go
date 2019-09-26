package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"text/template"
	"time"

	"github.com/pelletier/go-toml"
	"github.com/sirupsen/logrus"
)

var netClient *http.Client

type (
	// Plugin defines the sonar-scaner plugin parameters.
	Plugin struct {
		Host       string
		Token      string
		Key        string
		Name       string
		Version    string
		Sources    string
		Inclusions string
		Exclusions string
		Language   string
		Profile    string
		Encoding   string
		Remote     string
		Branch     string
		Quality    string
	}
	// SonarReport it is the representation of .scannerwork/report-task.txt
	SonarReport struct {
		ProjectKey   string `toml:"projectKey"`
		ServerURL    string `toml:"serverUrl"`
		DashboardURL string `toml:"dashboardUrl"`
		CeTaskID     string `toml:"ceTaskId"`
		CeTaskURL    string `toml:"ceTaskUrl"`
	}
	// TaskResponse Give Compute Engine task details such as type, status, duration and associated component.
	TaskResponse struct {
		Task struct {
			ID            string `json:"id"`
			Type          string `json:"type"`
			ComponentID   string `json:"componentId"`
			ComponentKey  string `json:"componentKey"`
			ComponentName string `json:"componentName"`
			AnalysisID    string `json:"analysisId"`
			Status        string `json:"status"`
		} `json:"task"`
	}
	// ProjectStatusResponse Get the quality gate status of a project or a Compute Engine task
	ProjectStatusResponse struct {
		ProjectStatus struct {
			Status string `json:"status"`
		} `json:"projectStatus"`
	}
)

func init() {
	netClient = &http.Client{
		Timeout: time.Second * 10,
		Transport: &http.Transport{
			Dial: (&net.Dialer{
				Timeout: 5 * time.Second,
			}).Dial,
			TLSHandshakeTimeout: 5 * time.Second,
		},
	}
}

func (p Plugin) buildScannerProperties() error {

	p.Key = strings.Replace(p.Key, "/", ":", -1)

	tmpl, err := template.ParseFiles("/opt/sonar-scanner/conf/sonar-scanner.properties.tmpl")
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err,
		}).Fatal("Template parsing failed")
	}

	f, err := os.Create("/opt/sonar-scanner/conf/sonar-scanner.properties")
	defer f.Close()
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err,
		}).Fatal("sonar-properties file creation failed")
	}

	err = tmpl.ExecuteTemplate(f, "sonar-scanner.properties.tmpl", p)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err,
		}).Fatal("Template execution failed")
	}

	return nil
}

// Exec executes the plugin step
func (p Plugin) Exec() error {

	err := p.buildScannerProperties()
	if err != nil {
		return err
	}

	report, err := staticScan()
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err,
		}).Fatal("Unable to scan")
	}

	logrus.WithFields(logrus.Fields{
		"job url": report.CeTaskURL,
	}).Info("Job url")

	task, err := waitForSonarJob(report)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err,
		}).Fatal("Unable to get Job state")
	}

	status := getStatus(task, report)

	if status != p.Quality {
		logrus.WithFields(logrus.Fields{
			"status": status,
		}).Fatal("QualityGate status failed")
	}

	return nil
}

func staticScan() (*SonarReport, error) {
	cmd := exec.Command("/opt/sonar-scanner/bin/sonar-scanner")
	output, err := cmd.CombinedOutput()
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err,
		}).Fatal("Run command failed")
		return nil, err
	}
	fmt.Printf("out:\n%s", output)
	cmd = exec.Command("/bin/sed", "-e", "s/=/=\"/", "-e", "s/$/\"/", ".scannerwork/report-task.txt")
	output, err = cmd.CombinedOutput()
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err,
		}).Fatal("Run command failed")
		return nil, err
	}
	// log.Printf("%s\n",output)

	report := SonarReport{}
	err = toml.Unmarshal(output, &report)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err,
		}).Fatal("Toml Unmarshal failed")
		return nil, err
	}

	return &report, nil
}

func getStatus(task *TaskResponse, report *SonarReport) string {
	reportRequest := url.Values{
		"analysisId": {task.Task.AnalysisID},
	}
	projectRequest, err := http.NewRequest("GET", report.ServerURL+"/api/qualitygates/project_status?"+reportRequest.Encode(), nil)
	projectRequest.Header.Add("Authorization", "Basic "+os.Getenv("TOKEN"))
	projectResponse, err := netClient.Do(projectRequest)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err,
		}).Fatal("Failed get status")
	}
	buf, _ := ioutil.ReadAll(projectResponse.Body)
	project := ProjectStatusResponse{}
	if err := json.Unmarshal(buf, &project); err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err,
		}).Fatal("Failed")
	}
	return project.ProjectStatus.Status
}

func getSonarJobStatus(report *SonarReport) *TaskResponse {

	taskRequest, err := http.NewRequest("GET", report.CeTaskURL, nil)
	taskRequest.Header.Add("Authorization", "Basic "+os.Getenv("TOKEN"))
	taskResponse, err := netClient.Do(taskRequest)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err,
		}).Fatal("Failed get sonar job status")
	}
	buf, _ := ioutil.ReadAll(taskResponse.Body)
	task := TaskResponse{}
	json.Unmarshal(buf, &task)
	return &task
}

func waitForSonarJob(report *SonarReport) (*TaskResponse, error) {
	timeout := time.After(300 * time.Second)
	tick := time.Tick(500 * time.Millisecond)
	for {
		select {
		case <-timeout:
			return nil, errors.New("timed out")
		case <-tick:
			job := getSonarJobStatus(report)
			if job.Task.Status == "SUCCESS" {
				return job, nil
			}
		}
	}
}
