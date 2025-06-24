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
	"strings"
	"time"
)

const VmNamePrefix = "github_workflow_"

var httpClient = &http.Client{}
var NumWorkers = env.GetIntOr("workers", 3)
var TemplateVmId = env.GetInt("proxmox.templateId")
var ProxmoxUrl = env.Get("proxmox.baseUrl")
var ProxmoxNode = env.Get("proxmox.node")

var CloneVmUrl *url.URL
var GetVmsUrl *url.URL

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

	request := createBaseRequest()
	request.Method = http.MethodPost
	request.URL = CloneVmUrl
	request.Body = io.NopCloser(b)

	response, err := httpClient.Do(request)
	defer closeResponse(response)
	if err != nil {
		return err
	}

	//get back id
	buf := new(strings.Builder)
	_, err = io.Copy(buf, response.Body)
	if err != nil {
		return err
	}

	taskId := buf.String()

	//wait for task to complete
	var done bool
	for {
		time.Sleep(10 * time.Second)
		if done, err = isTaskComplete(taskId); done {
			break
		}
		if err != nil {
			log.Printf("Error getting task %s status: %s", taskId, err.Error())
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
	request := createBaseRequest()
	request.URL = GetVmsUrl

	response, err := httpClient.Do(request)
	defer closeResponse(response)
	if err != nil {
		return nil, err
	}

	data := make([]VM, 0)
	err = json.NewDecoder(response.Body).Decode(&data)
	return data, err
}

func isTaskComplete(id string) (bool, error) {
	var err error
	request := createBaseRequest()
	request.URL, err = url.Parse(fmt.Sprintf("%s/api2/json/nodes/%s/tasks/%s/status", ProxmoxUrl, ProxmoxNode, id))
	if err != nil {
		return false, err
	}

	var response *http.Response
	response, err = httpClient.Do(request)
	defer closeResponse(response)
	if err != nil {
		return false, err
	}

	var taskStatus TaskStatusResponse
	err = json.NewDecoder(response.Body).Decode(&taskStatus)
	if err != nil {
		return false, err
	}

	if taskStatus.Status == "stopped" {
		return true, err
	}

	return false, err
}

func startVM(id int) error {
	var err error
	request := createBaseRequest()
	request.URL, err = url.Parse(fmt.Sprintf("%s/api2/json/nodes/%s/qemu/%d/status/start", ProxmoxUrl, ProxmoxNode, id))
	if err != nil {
		return err
	}
	request.Method = http.MethodPost

	var response *http.Response
	response, err = httpClient.Do(request)
	defer closeResponse(response)
	return err
}

func createBaseRequest() *http.Request {
	request := &http.Request{
		Header: make(http.Header),
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", fmt.Sprintf("PVEAPIToken=%s=%s", env.Get("proxmox.user"), env.Get("proxmox.password")))

	return request
}

func closeResponse(response *http.Response) {
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
}

type CloneRequest struct {
	NewId int    `json:"newid"`
	Name  string `json:"name"`
}

type TaskStatusResponse struct {
	Status string `json:"status"`
}

type VM struct {
	Id   int    `json:"vmid"`
	Name string `json:"name"`
}
