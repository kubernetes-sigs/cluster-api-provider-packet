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
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

func TestTokenExchanger_Token(t *testing.T) {
	g := NewWithT(t)
	// Create a mock server to handle the token exchange request
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Set content type
		w.Header().Set("Content-Type", "application/json")
		// Write out oauth2 json response
		_, err := w.Write([]byte(`{
			"access_token": "sample_token",
			"token_type": "Bearer",
			"expires_in": 3600
		}`))
		if err != nil {
			t.Fatalf("failed to write json response, err = %v", err)
		}
	}))
	defer mockServer.Close()

	// Create a TokenExchanger instance with the mock server URL
	exchanger := &TokenExchanger{
		metalAPIKey:      "sample_api_key",
		tokenExchangeURL: mockServer.URL,
		client:           mockServer.Client(),
	}

	// Call the Token method
	token, err := exchanger.Token()

	// Assert that no error occurred
	g.Expect(err).ToNot(HaveOccurred())

	// Assert that the token is not nil
	g.Expect(token).ToNot(BeNil())

	// Assert the token values
	g.Expect(token.AccessToken).To(Equal("sample_token"))
	g.Expect(token.Expiry.Round(time.Second)).To(Equal(time.Now().Add(time.Hour).Round(time.Second)))
}
