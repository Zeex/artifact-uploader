package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type linkVisibility struct {
	Tag string `json:".tag"`
}

type linkPermissions struct {
	ResolvedVisibility linkVisibility `json:"resolved_visibility"`
}

type sharedLink struct {
	ID string `json:"id"`
	Name string `json:"name"`
	URL string `json:"url"`
	PawnLower string `json:"path_lower"`
	LinkPermissions linkPermissions `json:"link_permissions"`
}

type listSharedLinksResponse struct {
	Links []sharedLink `json:"links"`
}

func exit(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}

func exitOnError(err error) {
	if err != nil {
		exit(err.Error())
	}
}

func newDropboxContentRequest(
		method string,
		url string,
		token string,
		contentType string,
		data []byte,
		params map[string]interface{}) (*http.Request, error) {
	request, err := http.NewRequest(method, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer " + token)
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("Dropbox-API-Arg", string(paramsJSON))
	return request, nil
}

func newDropboxAPIRequest(
		method string,
		url string,
		token string,
		params map[string]interface{}) (*http.Request, error) {
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequest(
		method,
		url,
		bytes.NewReader(paramsJSON))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer " + token)
	request.Header.Set("Content-Type", "application/json")
	return request, nil
}

func processResponse(response http.Response, v interface{}) (string, error) {
	responseBody, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return "", err
	}
	if v != nil {
		err = json.Unmarshal(responseBody, v)
		if err != nil {
			return "", err
		}
	}
	responseString := string(responseBody)
	if response.StatusCode != http.StatusOK {
		message := fmt.Sprintf("%d\n%s", response.StatusCode, responseString)
		return "", errors.New(message)
	}
	return responseString, err
}

func main() {
	args := os.Args
	if len(args) < 3 {
		exit("Usage: artifact-uploader <dropbox_token> <file_path> [<upload_path>]")
	}

	dropboxToken := args[1]
	filePath := args[2]
	
	uploadPath := "/" + filepath.Base(filePath)
	if len(args) >= 4 {
		uploadPath = args[3]
	}

	data, err := ioutil.ReadFile(filePath)
	exitOnError(err)

	httpClient := &http.Client{}
	request, err := newDropboxContentRequest(
		"POST",
		"https://content.dropboxapi.com/2/files/upload",
		dropboxToken,
		"application/octet-stream",
		data,
		map[string]interface{} {
			"path": uploadPath,
			"mode": "overwrite",
			"autorename": true,
			"mute": false,
			"strict_conflict": false,
		});
	exitOnError(err)

	response, err := httpClient.Do(request)
	exitOnError(err)
	defer response.Body.Close()

	_, err = processResponse(*response, nil)
	exitOnError(err)

	request, err = newDropboxAPIRequest(
		"POST",
		"https://api.dropboxapi.com/2/sharing/list_shared_links",
		dropboxToken,
		map[string]interface{} {
			"path": uploadPath,
		})
	exitOnError(err)

	response, err = httpClient.Do(request)
	exitOnError(err)
	defer response.Body.Close()

	var linksResponse listSharedLinksResponse
	_, err = processResponse(*response, &linksResponse)
	exitOnError(err)

	var url string
	for _, link := range linksResponse.Links {
		if link.LinkPermissions.ResolvedVisibility.Tag == "public" {
			url = link.URL
			break
		}
	}

	if url == "" {
		request, err = newDropboxAPIRequest(
			"POST",
			"https://api.dropboxapi.com/2/sharing/create_shared_link_with_settings",
			dropboxToken,
			map[string]interface{} {
				"path": uploadPath,
				"settings": map[string]interface{} {
					"requested_visibility": "public",
					"access": "viewer",
				},
			})
		exitOnError(err)

		response, err = httpClient.Do(request)
		exitOnError(err)
		defer response.Body.Close()

		var shareResponse map[string]interface{}
		_, err = processResponse(*response, &shareResponse)
		exitOnError(err)

		url = shareResponse["url"].(string)
	}

	if url == "" {
		exit("Dropbox API did not return file URL!")
	}

	url = strings.Replace(url, "?dl=0", "?dl=1", 1)
	fmt.Println(url);
}
