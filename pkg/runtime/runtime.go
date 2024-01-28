package runtime

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"runtime"
	"time"
)

const (
	authUrl                       = "https://auth.docker.io/token?service=registry.docker.io&scope=repository:library/%s:pull"
	manifestUrl                   = "https://registry.hub.docker.com/v2/library/%s/manifests/%s"
	imageUrl                      = "https://registry.hub.docker.com/v2/library/%s/blobs/%s"
	OCIImageLayerMediaTypeV1      = "application/vnd.oci.image.layer.v1.tar"
	OCIImageIndexMediaTypeV1      = "application/vnd.oci.image.index.v1+json"
	OCIImageManifestMediaTypeV1   = "application/vnd.oci.image.manifest.v1+json"
	DockerManifestMediaTypeV2     = "application/vnd.docker.distribution.manifest.v2+json"
	DockerManifestListMediaTypeV2 = "application/vnd.docker.distribution.manifest.list.v2+json"
)

var AcceptMediaTypes = []string{
	OCIImageIndexMediaTypeV1,
	OCIImageManifestMediaTypeV1,
	DockerManifestMediaTypeV2,
	DockerManifestListMediaTypeV2,
}

type OCIImageRetriever struct {
	image  string
	tag    string
	auth   Auth
	client *http.Client
}

type Auth struct {
	Token       string    `json:"token"`
	AccessToken string    `json:"access_token"`
	ExpiresIn   int       `json:"expires_in"`
	IssuedAt    time.Time `json:"issued_at"`
}

func NewOCIImageRetriever(image string, tag string) (*OCIImageRetriever, error) {
	resp, err := http.Get(fmt.Sprintf(authUrl, image))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var auth Auth
	if err := json.NewDecoder(resp.Body).Decode(&auth); err != nil {
		return nil, err
	}

	return &OCIImageRetriever{image: image, tag: tag, auth: auth, client: &http.Client{}}, nil
}

func (d *OCIImageRetriever) fetchImageManifest() (ImageManifest, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf(manifestUrl, d.image, d.tag), nil)
	if err != nil {
		return ImageManifest{}, err
	}
	req.Header.Add("Authorization", "Bearer "+d.auth.AccessToken)
	for _, mediaType := range AcceptMediaTypes {
		req.Header.Add("Accept", mediaType)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return ImageManifest{}, err
	}
	defer resp.Body.Close()

	switch resp.Header.Get("Content-Type") {
	case OCIImageIndexMediaTypeV1, DockerManifestListMediaTypeV2:
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
		req.Header.Add("Accept", OCIImageManifestMediaTypeV1)

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
	case OCIImageManifestMediaTypeV1, DockerManifestMediaTypeV2:
		var im ImageManifest
		if err := json.NewDecoder(resp.Body).Decode(&im); err != nil {
			return im, err
		}
		return im, nil
	}
	return ImageManifest{}, fmt.Errorf("unknown content type: %s", resp.Header.Get("Content-Type"))
}

func (d *OCIImageRetriever) pull() (string, error) {
	// imageのキャッシュを探す。なければpullする

	manifest, err := d.fetchImageManifest()
	if err != nil {
		return "", err
	}

	dirPath, err := os.Getwd()
	if err != nil {
		return "", err
	}

	imagesDir := path.Join(dirPath, fmt.Sprintf("images/%s", manifest.Config.Digest))

	if _, err := os.Stat(imagesDir); os.IsNotExist(err) {
		if err := os.MkdirAll(imagesDir, 0755); err != nil {
			return "", err
		}
	}

	errChan := make(chan error, len(manifest.Layers))
	for _, layer := range manifest.Layers {
		go func(layer ManifestLayer) {
			err := d.downloadLayer(layer, imagesDir)
			errChan <- err
		}(layer)
	}

	for i := 0; i < len(manifest.Layers); i++ {
		if err := <-errChan; err != nil {
			return "", err
		}
	}

	return imagesDir, nil
}

func (d *OCIImageRetriever) downloadLayer(layer ManifestLayer, dirPath string) error {
	layerURL := fmt.Sprintf(imageUrl, d.image, layer.Digest)

	req, err := http.NewRequest("GET", layerURL, nil)
	if err != nil {
		return err
	}
	req.Header.Add("Authorization", "Bearer "+d.auth.AccessToken)
	req.Header.Add("Accept", OCIImageLayerMediaTypeV1)

	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download layer: %s", resp.Status)
	}

	filePath := path.Join(dirPath, layer.Digest+".tar")
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	return err
}

func Tar(filePath string) error {
	cmd := exec.Command("tar", "-xvf", filePath)
	err := cmd.Run()
	if err != nil {
		return err
	}
	return nil
}
