package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/pkg/sftp"
	"github.com/pufferpanel/github-runner-scaler/env"
	"golang.org/x/crypto/ssh"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

const VmNamePrefix = "github-workflow-"

var httpClient = &http.Client{}
var NumWorkers = env.GetIntOr("workers", 3)
var TemplateVmId = env.GetInt("proxmox.templateId")
var ProxmoxUrl = env.Get("proxmox.baseUrl")
var ProxmoxNode = env.Get("proxmox.node")
var ProxmoxSftpHost = env.Get("proxmox.sftp.host")
var ProxmoxSftpUser = env.Get("proxmox.sftp.user")
var ProxmoxSftpPassword = env.Get("proxmox.sftp.password")
var CloudInitSshUser = env.Get("cloudinit.ssh.user")
var CloudInitSshKey = env.Get("cloudinit.ssh.key")

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

func cloneVM(githubRunId string) error {
	vms, err := getVMs()
	if err != nil {
		return err
	}

	var currentId int
	for _, vm := range vms {
		if currentId < vm.Id {
			currentId = vm.Id
		}
	}
	currentId++

	b := new(bytes.Buffer)
	err = json.NewEncoder(b).Encode(&CloneRequest{
		NewId: currentId,
		Name:  VmNamePrefix + githubRunId,
	})
	if err != nil {
		return err
	}

	taskId, err := doRequest[string](http.MethodPost, CloneVmUrl, b.Bytes())

	//wait for task to complete
	var done bool
	for !done {
		time.Sleep(10 * time.Second)
		done, err = isTaskComplete(taskId)
		if err != nil {
			proxmoxLogger.Printf("Error getting task %s status: %s", taskId, err.Error())
		}
	}

	var snippet = fmt.Sprintf("snippets/%d.json", currentId)
	//drop in the new snippet
	err = writeMetaCloudInit(snippet, map[string]string{})
	if err != nil {
		proxmoxLogger.Printf("Error creating snippet: %s", err)
		return err
	}

	//update the config to include our snippet
	err = updateCloudInit(currentId, snippet)
	if err != nil {
		proxmoxLogger.Printf("Error updating cicustom: %s", err)
		return err
	}

	//just in case, rebuild the cloud init image
	//do this in a function so things are closed here
	err = regenerateCloudInitImage(currentId)
	if err != nil {
		proxmoxLogger.Printf("Error rebuilding cloudinit: %s", err)
		return err
	}

	//start the VM
	err = startVM(currentId)
	return err
}

func getVMs() ([]VM, error) {
	return doRequest[[]VM](http.MethodGet, GetVmsUrl, nil)
}

func updateCloudInit(id int, path string) error {
	u, err := url.Parse(fmt.Sprintf("%s/api2/json/nodes/%s/qemu/%d/config", ProxmoxUrl, ProxmoxNode, id))
	if err != nil {
		return err
	}

	newConfig := &VM{
		CloudInitCustom: fmt.Sprintf("meta=local:%s,user=local:snippets/github-runner.yaml", path),
		CloudInitUser:   CloudInitSshUser,
		SshKeys:         url.PathEscape(CloudInitSshKey),
	}

	buf := new(bytes.Buffer)
	err = json.NewEncoder(buf).Encode(&newConfig)
	if err != nil {
		return err
	}

	_, err = doRequest[None](http.MethodPut, u, buf.Bytes())
	return err
}

func regenerateCloudInitImage(id int) error {
	u, err := url.Parse(fmt.Sprintf("%s/api2/json/nodes/%s/qemu/%d/cloudinit", ProxmoxUrl, ProxmoxNode, id))
	if err != nil {
		return err
	}
	_, err = doRequest[None](http.MethodPut, u, nil)
	return err
}

func isTaskComplete(id string) (bool, error) {
	u, err := url.Parse(fmt.Sprintf("%s/api2/json/nodes/%s/tasks/%s/status", ProxmoxUrl, ProxmoxNode, id))
	if err != nil {
		return false, err
	}

	response, err := doRequest[TaskStatus](http.MethodGet, u, nil)
	if err != nil {
		return false, err
	}
	if response.Status == "stopped" {
		if response.ExitStatus != "OK" {
			return true, fmt.Errorf("task %s failed (%s)", id, response.ExitStatus)
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

	_, err = doRequest[string](http.MethodPost, u, nil)
	return err
}

func writeMetaCloudInit(filename string, data map[string]string) error {
	msg := map[string]map[string]string{
		"v1": data,
	}

	sshConn, err := ssh.Dial("tcp", ProxmoxSftpHost, &ssh.ClientConfig{
		Config: ssh.Config{},
		User:   ProxmoxSftpUser,
		Auth:   []ssh.AuthMethod{ssh.Password(ProxmoxSftpPassword)},
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			return nil
		},
	})
	defer func() {
		if sshConn != nil {
			_ = sshConn.Close()
		}
	}()
	if err != nil {
		return err
	}

	sftpConn, err := sftp.NewClient(sshConn)
	defer func() {
		if sftpConn != nil {
			_ = sftpConn.Close()
		}
	}()
	if err != nil {
		return err
	}
	file, err := sftpConn.Create(filepath.Join("/var/lib/vz/", filename))
	defer func() {
		if file != nil {
			_ = file.Close()
		}
	}()
	if err != nil {
		return err
	}
	err = json.NewEncoder(file).Encode(&msg)
	return err
}

func closeResponse(response *http.Response) {
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
}

func doRequest[T ProxmoxResponse](method string, url *url.URL, body []byte) (T, error) {
	request := &http.Request{
		Method: method,
		URL:    url,
		Header: make(http.Header),
	}

	if (method == http.MethodPost || method == http.MethodPut) && body == nil {
		body = []byte("{}") //proxmox wants a junk json object for POSTs
	}

	if body != nil {
		request.ContentLength = int64(len(body))
		request.Body = io.NopCloser(bytes.NewReader(body))
	}

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", fmt.Sprintf("PVEAPIToken=%s=%s", env.Get("proxmox.user"), env.Get("proxmox.password")))

	response, err := httpClient.Do(request)
	defer closeResponse(response)
	if err != nil || response.StatusCode >= 400 {
		var data []byte
		if err != nil {
			proxmoxLogger.Printf("Error from call: %s", err.Error())
			data = []byte(err.Error())
		} else {
			data, _ = io.ReadAll(response.Body)
			_ = response.Body.Close()
			response.Body = io.NopCloser(bytes.NewReader(data)) //replace body in case a downstream reader wants it
			err = errors.New(string(data))
		}
		proxmoxLogger.Printf("%s: %s (%d) %s", request.Method, request.URL.String(), response.StatusCode, string(data))
		return *new(T), err
	} else {
		proxmoxLogger.Printf("%s: %s (%d)", request.Method, request.URL.String(), response.StatusCode)
	}

	type resType struct {
		Data T `json:"data"`
	}

	res := &resType{}
	err = json.NewDecoder(response.Body).Decode(res)
	return res.Data, err
}

type ProxmoxResponse interface {
	None | TaskStatus | string | VM | []VM
}

type None interface{}

type CloneRequest struct {
	NewId int    `json:"newid"`
	Name  string `json:"name"`
}

type TaskStatus struct {
	Status     string `json:"status"`
	ExitStatus string `json:"exitstatus"`
}

type VM struct {
	Id              int    `json:"vmid,omitempty"`
	Name            string `json:"name,omitempty"`
	CloudInitCustom string `json:"cicustom,omitempty"`
	CloudInitUser   string `json:"ciuser,omitempty"`
	SshKeys         string `json:"sshkeys,omitempty"`
}
