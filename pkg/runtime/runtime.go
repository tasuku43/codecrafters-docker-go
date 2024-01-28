package runtime

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"time"
)

const (
	authUrl                   = "https://auth.docker.io/token?service=registry.docker.io&scope=repository:library/%s:pull"
	manifestUrl               = "https://registry.hub.docker.com/v2/library/%s/manifests/%s"
	OCIImageIndexMediaType    = "application/vnd.oci.image.index.v1+json"
	OCIImageManifestMediaType = "application/vnd.oci.image.manifest.v1+json"
)

type OCIRuntime struct {
	image  string
	auth   Auth
	client *http.Client
}

type Auth struct {
	Token       string    `json:"token"`
	AccessToken string    `json:"access_token"`
	ExpiresIn   int       `json:"expires_in"`
	IssuedAt    time.Time `json:"issued_at"`
}

func NewDocker(image string) (*OCIRuntime, error) {
	resp, err := http.Get(fmt.Sprintf(authUrl, image))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var t Auth
	if err := json.NewDecoder(resp.Body).Decode(&t); err != nil {
		return nil, err
	}

	return &OCIRuntime{image: image, auth: t, client: &http.Client{}}, nil
}

func (d *OCIRuntime) fetchImageManifest(tag string) (ImageManifest, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf(manifestUrl, d.image, tag), nil)
	if err != nil {
		return ImageManifest{}, err
	}
	req.Header.Add("Authorization", "Bearer "+d.auth.AccessToken)
	req.Header.Add("Accept", OCIImageIndexMediaType)
	req.Header.Add("Accept", OCIImageManifestMediaType)

	resp, err := d.client.Do(req)
	if err != nil {
		return ImageManifest{}, err
	}
	defer resp.Body.Close()

	switch resp.Header.Get("Content-Type") {
	case "application/vnd.oci.image.index.v1+json":
		var m Manifests
		if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
			return ImageManifest{}, err
		}
		digest, _ := m.findDigest(runtime.GOARCH, "linux") // TODO: runtime.GOOS
		req, err := http.NewRequest("GET", fmt.Sprintf(manifestUrl, d.image, digest), nil)
		if err != nil {
			return ImageManifest{}, err
		}
		req.Header.Add("Authorization", "Bearer "+d.auth.AccessToken)
		req.Header.Add("Accept", OCIImageManifestMediaType)

		resp, err := d.client.Do(req)
		if err != nil {
			return ImageManifest{}, err
		}
		defer resp.Body.Close()

		var im ImageManifest
		if err := json.NewDecoder(resp.Body).Decode(&im); err != nil {
			return im, err
		}
		return im, nil
	case "application/vnd.oci.image.manifest.v1+json":
		var im ImageManifest
		if err := json.NewDecoder(resp.Body).Decode(&im); err != nil {
			return im, err
		}
		return im, nil
	}
	return ImageManifest{}, fmt.Errorf("unknown content type: %s", resp.Header.Get("Content-Type"))
}
