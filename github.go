package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/bartventer/httpcache"
	_ "github.com/bartventer/httpcache/store/memcache" //  Register the in-memory backend
	"github.com/google/go-github/v73/github"
	"github.com/pufferpanel/github-runner-scaler/env"
)

var githubClient = github.NewClient(httpcache.NewClient("memcache://")).WithAuthToken(env.Get("github.token"))
var githubOrganization = env.Get("github.organization")
var githubLabel = env.Get("github.label")
var githubGroup = env.Get("github.group")
var githubRunnerPrefix = env.Get("github.runnerprefix")

func GetJITConfig(id int) (string, error) {
	groupId, err := GetRunnerGroupId()
	if err != nil {
		return "", err
	}

	config, response, err := githubClient.Actions.GenerateOrgJITConfig(context.Background(), githubOrganization, &github.GenerateJITConfigRequest{
		Name:          fmt.Sprintf("%s-%d", githubRunnerPrefix, id),
		Labels:        []string{githubLabel},
		RunnerGroupID: groupId,
	})
	defer CloseGithubResponse(response)
	if err != nil {
		return "", err
	}

	return config.GetEncodedJITConfig(), err
}

func GetRunnerGroupId() (int64, error) {
	groups, response, err := githubClient.Actions.ListOrganizationRunnerGroups(context.Background(), githubOrganization, &github.ListOrgRunnerGroupOptions{})
	defer CloseGithubResponse(response)
	if err != nil {
		return 0, err
	}
	for _, g := range groups.RunnerGroups {
		if *g.Name == githubGroup {
			return *g.ID, nil
		}
	}

	return 0, errors.New("runner group not found")
}

// CloseGithubResponse GitHub's wrapper means we can't use our own one...
func CloseGithubResponse(response *github.Response) {
	if response != nil {
		CloseResponse(response.Response)
	}
}
