/*
Copyright 2016 Citrix Systems, Inc, All rights reserved.

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

package netscaler

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
)

func listBoundResources(resourceName string, resourceType string, boundResourceType string, boundResourceFilterName string, boundResourceFilterValue string) ([]byte, error) {
	log.Println("listing resource of type ", resourceType)
	nsIp := os.Getenv("NS_IP")
	username := os.Getenv("NS_USERNAME")
	password := os.Getenv("NS_PASSWORD")
	var url string
	if boundResourceFilterName == "" {
		url = fmt.Sprintf("http://%s/nitro/v1/config/%s_%s_binding/%s", nsIp, resourceType, boundResourceType, resourceName)
	} else {
		url = fmt.Sprintf("http://%s/nitro/v1/config/%s_%s_binding/%s?filter=%s:%s", nsIp, resourceType, boundResourceType, resourceName, boundResourceFilterName, boundResourceFilterValue)
	}

	var contentType = fmt.Sprintf("application/vnd.com.citrix.netscaler.%s_%s_binding+json", resourceType, boundResourceType)

	log.Println("url:", url)
	req, err := http.NewRequest("GET", url, bytes.NewBuffer([]byte{}))
	//req.Header.Set("Accept", contentType)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-NITRO-USER", username)
	req.Header.Set("X-NITRO-PASS", password)

	client := &http.Client{}
	resp, err := client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		log.Fatal(err)
		return []byte{}, err
	} else {
		log.Println("response Status:", resp.Status)

		switch resp.Status {
		case "200 OK":
			body, _ := ioutil.ReadAll(resp.Body)
			return body, nil
		case "400 Bad Request", "401 Unauthorized", "403 Forbidden", "404 Not Found",
			"405 Method Not Allowed", "406 Not Acceptable",
			"409 Conflict", "503 Service Unavailable", "599 Netscaler specific error":
			//TODO
			body, _ := ioutil.ReadAll(resp.Body)
			log.Println("error = " + string(body))
			return body, errors.New("failed: " + resp.Status + " (" + string(body) + ")")
		default:
			body, err := ioutil.ReadAll(resp.Body)
			log.Println("error = " + string(body))
			return body, err

		}
	}
}

func listResource(resourceType string, resourceName string) ([]byte, error) {
	log.Println("listing resource of type ", resourceType)
	nsIp := os.Getenv("NS_IP")
	username := os.Getenv("NS_USERNAME")
	password := os.Getenv("NS_PASSWORD")
	url := fmt.Sprintf("http://%s/nitro/v1/config/%s", nsIp, resourceType)

	if resourceName != "" {
		url = fmt.Sprintf("http://%s/nitro/v1/config/%s/%s", nsIp, resourceType, resourceName)
	}

	var contentType = fmt.Sprintf("application/vnd.com.citrix.netscaler.%s+json", resourceType)

	log.Println("url:", url)
	req, err := http.NewRequest("GET", url, bytes.NewBuffer([]byte{}))
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-NITRO-USER", username)
	req.Header.Set("X-NITRO-PASS", password)

	client := &http.Client{}
	resp, err := client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		log.Fatal(err)
		return []byte{}, err
	} else {
		log.Println("response Status:", resp.Status)

		switch resp.Status {
		case "200 OK":
			body, _ := ioutil.ReadAll(resp.Body)
			return body, nil
		case "400 Bad Request", "401 Unauthorized", "403 Forbidden", "404 Not Found",
			"405 Method Not Allowed", "406 Not Acceptable",
			"409 Conflict", "503 Service Unavailable", "599 Netscaler specific error":
			//TODO
			body, _ := ioutil.ReadAll(resp.Body)
			log.Println("error = " + string(body))
			return body, errors.New("failed: " + resp.Status + " (" + string(body) + ")")
		default:
			body, err := ioutil.ReadAll(resp.Body)
			log.Println("error = " + string(body))
			return body, err

		}
	}
}
