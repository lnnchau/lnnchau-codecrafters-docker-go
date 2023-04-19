package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
)

type Registry struct {
	AUTHENTICATION_URL string
	REGISTRY_URL       string

	accessToken string
	chroot      string
}

type LayerInfo struct {
	Digest    string                 `json:"digest"`
	Size      int                    `json:"size"`
	MediaType string                 `json:"mediaType"`
	Platform  map[string]interface{} `json:"platform"`
}

type FSLayerInfo struct {
	Digest string `json:"blobSum"`
}

type Manifest struct {
	SchemaVersion int    `json:"schemaVersion"`
	MediaType     string `json:"mediaType"`

	// for v1
	ManifestList []LayerInfo `json:"manifests"`

	// for v2
	FsLayers []FSLayerInfo `json:"fsLayers"`
}

type ImageManifest struct {
	SchemaVersion int         `json:"schemaVersion"`
	MediaType     string      `json:"mediaType"`
	Config        LayerInfo   `json:"config"`
	Layers        []LayerInfo `json:"layers"`
}

func (registry *Registry) Authenticate(imageName string) error {
	accessToken, err := registry.getAccessToken(imageName)
	if err != nil {
		return err
	}

	registry.accessToken = accessToken

	return nil
}

func (registry *Registry) PullImage(name string, tag string) error {

	manifest, err := registry.getManifest(name, tag)
	if err != nil {
		return err
	}

	if err := registry.pullLayers(manifest, name); err != nil {
		return err
	}

	return nil
}

func (registry *Registry) getAccessToken(imageName string) (string, error) {
	req, err := http.NewRequest("GET", registry.AUTHENTICATION_URL, nil)
	if err != nil {
		return "", err
	}

	q := req.URL.Query()
	q.Add("service", "registry.docker.io")
	q.Add("scope", fmt.Sprintf("repository:%s:pull", imageName))
	req.URL.RawQuery = q.Encode()

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}

	accessToken := body["access_token"].(string)
	return accessToken, nil
}

func (registry *Registry) getManifest(imageName string, tag string) (*Manifest, error) {
	manifestReq, err := http.NewRequest("GET", fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry.REGISTRY_URL, imageName, tag), nil)
	if err != nil {
		return nil, err
	}

	manifestReq.Header.Add("Authorization", fmt.Sprintf("Bearer %s", registry.accessToken))
	manifestReq.Header.Add("Accept", "application/vnd.oci.image.index.v1+json")

	resp, err := http.DefaultClient.Do(manifestReq)
	if err != nil {
		return nil, err
	}

	// parse resp
	var manifestReqBody Manifest
	if err := json.NewDecoder(resp.Body).Decode(&manifestReqBody); err != nil {
		return nil, err
	}

	return &manifestReqBody, nil
}

func (registry *Registry) getImageManifest(imageName string, manifest LayerInfo) (*ImageManifest, error) {
	manifestReq, err := http.NewRequest("GET", fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry.REGISTRY_URL, imageName, manifest.Digest), nil)
	if err != nil {
		return nil, err
	}

	manifestReq.Header.Add("Authorization", fmt.Sprintf("Bearer %s", registry.accessToken))
	manifestReq.Header.Add("Accept", manifest.MediaType)

	resp, err := http.DefaultClient.Do(manifestReq)
	if err != nil {
		return nil, err
	}

	// parse resp
	var manifestReqBody ImageManifest
	if err := json.NewDecoder(resp.Body).Decode(&manifestReqBody); err != nil {
		return nil, err
	}

	return &manifestReqBody, nil
}

func (registry *Registry) pullLayers(manifest *Manifest, name string) error {
	switch manifest.SchemaVersion {
	case 1:
		for _, layer := range manifest.FsLayers {
			registry.pullLayer(name, layer.Digest)
		}

	case 2:
		imageManifest, err := registry.getImageManifest(name, manifest.ManifestList[0])
		if err != nil {
			return err
		}

		for _, layer := range imageManifest.Layers {
			registry.pullLayer(name, layer.Digest)
		}

	default:
		return fmt.Errorf("unknown schema version: %d", manifest.SchemaVersion)
	}

	return nil
}

func (registry *Registry) pullLayer(imageName string, digest string) error {
	req, err := http.NewRequest(
		"GET",
		fmt.Sprintf("https://%s/v2/%s/blobs/%s", registry.REGISTRY_URL, imageName, digest), nil)
	if err != nil {
		return err
	}

	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", registry.accessToken))

	outFn := fmt.Sprintf("%s.tar.gz", digest)
	redirectUrl, err := downloadFile(req, outFn)
	for redirectUrl != "" {
		req, err := http.NewRequest("GET", redirectUrl, nil)
		if err != nil {
			return err
		}

		redirectUrl, err = downloadFile(req, outFn)
		if err != nil {
			return err
		}
	}

	cmd := exec.Command("tar", "-xzf", outFn, "-C", registry.chroot)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
}

func downloadFile(req *http.Request, outFn string) (string, error) {
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}

	if resp.StatusCode == 307 {
		return resp.Header.Get("Location"), nil
	} else if resp.StatusCode != 200 {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	out, err := os.Create(outFn)
	if err != nil {
		return "", err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return "", err
	}

	return "", nil
}
