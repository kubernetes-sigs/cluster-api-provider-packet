/*
Copyright 2024 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package emlb

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/oauth2"
)

// TokenExchanger is an client for authenticating to the Load Balancer API.
type TokenExchanger struct {
	metalAPIKey string
	client      *http.Client
}

// Token creates a Token object to authenticate with the Load Balancer API.
func (m *TokenExchanger) Token() (*oauth2.Token, error) {
	tokenExchangeURL := "https://iam.metalctrl.io/api-keys/exchange"                             //nolint:gosec
	tokenExchangeRequest, err := http.NewRequest(http.MethodPost, tokenExchangeURL, http.NoBody) //nolint:noctx // we can't find a way to get the ctx into here yet and just using context.Background adds no value that we can tell
	if err != nil {
		return nil, err
	}
	tokenExchangeRequest.Header.Add("Authorization", fmt.Sprintf("Bearer %v", m.metalAPIKey))

	resp, err := m.client.Do(tokenExchangeRequest)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange request failed with status %v, body %v", resp.StatusCode, string(body))
	}

	token := oauth2.Token{}
	err = json.Unmarshal(body, &token)
	if err != nil {
		fmt.Println(len(body))
		fmt.Println(token)
		fmt.Println(err)
		return nil, err
	}

	expiresIn := token.Extra("expires_in")
	if expiresIn != nil {
		expiresInSeconds := expiresIn.(int)
		token.Expiry = time.Now().Add(time.Second * time.Duration(expiresInSeconds))
	}

	return &token, nil
}
