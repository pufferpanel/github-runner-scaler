package main

import (
	"context"
	"github.com/bartventer/httpcache"
	_ "github.com/bartventer/httpcache/store/memcache" //  Register the in-memory backend
	"github.com/google/go-github/v73/github"
	"github.com/pufferpanel/github-runner-scaler/env"
)

var githubClient = github.NewClient(httpcache.NewClient("memcache://")).WithAuthToken(env.Get("github.token"))
var githubOrganization = env.Get("github.organization")

func GetJITConfig() (string, error) {
	config, response, err := githubClient.Actions.GenerateOrgJITConfig(context.Background(), githubOrganization, &github.GenerateJITConfigRequest{
		Name:          "",
		RunnerGroupID: 0,
		WorkFolder:    nil,
		Labels:        nil,
	})
	defer func() {
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
	}()
	if err != nil {
		return "", err
	}

	return config.GetEncodedJITConfig(), err
}
