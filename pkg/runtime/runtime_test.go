// +go:build linux
package runtime

import (
	"fmt"
	"testing"
)

func Test_Pull(t *testing.T) {
	sut, _ := NewOCIImageRetriever("alpine", "latest")
	path, err := sut.pull()
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(path)
}
