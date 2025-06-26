package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/pufferpanel/github-runner-scaler/env"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const VmNamePrefix = "github-workflow-"

var httpClient = &http.Client{}
var NumWorkers = env.GetIntOr("workers", 3)
var TemplateVmId = env.GetInt("proxmox.templateId")
var ProxmoxUrl = env.Get("proxmox.baseUrl")
var ProxmoxNode = env.Get("proxmox.node")

var CloneVmUrl *url.URL
var GetVmsUrl *url.URL

var proxmoxLogger = log.New(os.Stdout, "[Proxmox] ", log.LstdFlags|log.Lmicroseconds)

func init() {
	var err error
	CloneVmUrl, err = url.Parse(fmt.Sprintf("%s/api2/json/nodes/%s/qemu/%d/clone", ProxmoxUrl, ProxmoxNode, TemplateVmId))
	if err != nil {
		panic(err)
	}

	GetVmsUrl, err = url.Parse(fmt.Sprintf("%s/api2/json/nodes/%s/qemu", ProxmoxUrl, ProxmoxNode))
	if err != nil {
		panic(err)
	}
}

func cloneVM(id string) error {
	currentId, err := getHighestId()
	if err != nil {
		return err
	}
	currentId++

	b := new(bytes.Buffer)
	err = json.NewEncoder(b).Encode(&CloneRequest{
		NewId: currentId,
		Name:  VmNamePrefix + id,
	})
	if err != nil {
		return err
	}

	response, err := doRequest(http.MethodPost, CloneVmUrl, b.Bytes())
	defer closeResponse(response)

	var body []byte
	if response != nil {
		body, _ = io.ReadAll(response.Body)
		proxmoxLogger.Printf("Response: %s", body)
	}

	if err != nil {
		return err
	}

	//get back id
	d := StringDataResponse{}
	err = json.NewDecoder(bytes.NewReader(body)).Decode(&d)
	if err != nil {
		proxmoxLogger.Printf("Json decode failed: %s", err)
		return err
	}

	taskId := d.Data

	//wait for task to complete
	var done bool
	for !done {
		time.Sleep(10 * time.Second)
		done, err = isTaskComplete(taskId)
		if err != nil {
			proxmoxLogger.Printf("Error getting task %s status: %s", taskId, err.Error())
		}
	}

	//start the VM
	err = startVM(currentId)
	return err
}

func getNumVMs() (int, error) {
	vms, err := getVMs()
	if err != nil {
		return 0, err
	}
	var count = 0
	for _, v := range vms {
		if strings.HasPrefix(v.Name, VmNamePrefix) {
			count++
		}
	}

	return count, nil
}

func getHighestId() (int, error) {
	vms, err := getVMs()
	if err != nil {
		return 0, err
	}
	id := 100
	for _, v := range vms {
		if id < v.Id {
			id = v.Id
		}
	}
	return id, nil
}

func getVMs() ([]VM, error) {
	response, err := doRequest(http.MethodGet, GetVmsUrl, nil)
	defer closeResponse(response)
	if err != nil {
		return nil, err
	}

	data := ListVmsResponse{}
	err = json.NewDecoder(response.Body).Decode(&data)
	return data.Data, err
}

func isTaskComplete(id string) (bool, error) {
	u, err := url.Parse(fmt.Sprintf("%s/api2/json/nodes/%s/tasks/%s/status", ProxmoxUrl, ProxmoxNode, id))
	if err != nil {
		return false, err
	}

	response, err := doRequest(http.MethodGet, u, nil)
	defer closeResponse(response)
	if err != nil {
		return false, err
	}

	var taskStatus TaskStatusResponse
	err = json.NewDecoder(response.Body).Decode(&taskStatus)
	if err != nil {
		return false, err
	}

	if taskStatus.Data.Status == "stopped" {
		if taskStatus.Data.ExitStatus != "OK" {
			return true, fmt.Errorf("task %s failed (%s)", id, taskStatus.Data.ExitStatus)
		}
		return true, err
	}

	return false, err
}

func startVM(id int) error {
	u, err := url.Parse(fmt.Sprintf("%s/api2/json/nodes/%s/qemu/%d/status/start", ProxmoxUrl, ProxmoxNode, id))
	if err != nil {
		return err
	}

	response, err := doRequest(http.MethodPost, u, nil)
	defer closeResponse(response)
	return err
}

func closeResponse(response *http.Response) {
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
}

func doRequest(method string, url *url.URL, body []byte) (*http.Response, error) {
	request := &http.Request{
		Method: method,
		URL:    url,
		Header: make(http.Header),
	}

	if method == http.MethodPost && body == nil {
		body = []byte("{}") //proxmox wants a junk json object for POSTs
	}

	if body != nil {
		request.ContentLength = int64(len(body))
		request.Body = io.NopCloser(bytes.NewReader(body))
	}

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", fmt.Sprintf("PVEAPIToken=%s=%s", env.Get("proxmox.user"), env.Get("proxmox.password")))

	response, err := httpClient.Do(request)
	if err != nil || response.StatusCode > 400 {
		var data []byte
		if err != nil {
			data = []byte(err.Error())
		} else {
			data, _ = io.ReadAll(response.Body)
			_ = response.Body.Close()
			response.Body = io.NopCloser(bytes.NewReader(data)) //replace body in case a downstream reader wants it
		}
		proxmoxLogger.Printf("%s: %s (%d)\n%s", request.Method, request.URL.String(), response.StatusCode, data)
	} else {
		proxmoxLogger.Printf("%s: %s (%d)", request.Method, request.URL.String(), response.StatusCode)
	}
	return response, err
}

type CloneRequest struct {
	NewId int    `json:"newid"`
	Name  string `json:"name"`
}

type TaskStatusResponse struct {
	Data TaskStatus `json:"data"`
}
type TaskStatus struct {
	Status     string `json:"status"`
	ExitStatus string `json:"exitstatus"`
}

type VM struct {
	Id   int    `json:"vmid"`
	Name string `json:"name"`
}

type ListVmsResponse struct {
	Data []VM `json:"data"`
}

type StringDataResponse struct {
	Data string `json:"data"`
}
