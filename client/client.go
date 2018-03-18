// Package mcapi has methods for requesting information from mcapi.us
package mcapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/syfaro/mcapi/types"
)

// APIEndpoint is the endpoint to use for requesting any information.
const APIEndpoint = "https://mcapi.us"

// GetServerStatus allows you to ping a server and get basic information.
func GetServerStatus(ip string, port int) (*types.ServerStatus, error) {
	resp, err := http.Get(fmt.Sprintf("%s/server/status?ip=%s&port=%d", APIEndpoint, ip, port))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := ioutil.ReadAll(resp.Body)

	var status types.ServerStatus
	err = json.Unmarshal(data, &status)
	if err != nil {
		return nil, err
	}

	if status.Error != "" {
		return &status, errors.New(status.Error)
	}

	return &status, nil
}

// GetServerQuery allows you to query a server to get more detailed information.
//
// Note that this function requires you to have query enabled on your server.
func GetServerQuery(ip string, port int) (*types.ServerQuery, error) {
	resp, err := http.Get(fmt.Sprintf("%s/server/query?ip=%s&port=%d", APIEndpoint, ip, port))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := ioutil.ReadAll(resp.Body)

	var status types.ServerQuery
	err = json.Unmarshal(data, &status)
	if err != nil {
		return nil, err
	}

	if status.Error != "" {
		return &status, errors.New(status.Error)
	}

	return &status, nil
}
