/*
Copyright 2021 The Kubernetes Authors.

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

package utils

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/util/sets"
)

type VolumeLocks struct {
	locks sets.String //nolint:staticcheck
	mux   sync.Mutex
}

func NewVolumeLocks() *VolumeLocks {
	return &VolumeLocks{
		locks: sets.NewString(),
	}
}

// IsFileExisting check file exist in volume driver
func IsFileExisting(filename string) bool {
	_, err := os.Stat(filename)
	if err == nil {
		return true
	}
	// Notice: this err may be is not dictionary error, it will returns true
	if os.IsNotExist(err) {
		return false
	}
	return true
}

func (vl *VolumeLocks) TryAcquire(volumeID string) bool {
	vl.mux.Lock()
	defer vl.mux.Unlock()
	if vl.locks.Has(volumeID) {
		return false
	}
	vl.locks.Insert(volumeID)
	return true
}

func (vl *VolumeLocks) Release(volumeID string) {
	vl.mux.Lock()
	defer vl.mux.Unlock()
	vl.locks.Delete(volumeID)
}

func ParseEndpoint(ep string) (string, string, error) {
	if strings.HasPrefix(strings.ToLower(ep), "unix://") || strings.HasPrefix(strings.ToLower(ep), "tcp://") {
		s := strings.SplitN(ep, "://", 2)
		if s[1] != "" {
			return s[0], s[1], nil
		}
	}
	return "", "", fmt.Errorf("Invalid endpoint: %v", ep)
}

func WriteStringToFile(file *os.File, data string) (err error) {
	write := bufio.NewWriter(file)
	size, err := write.WriteString(data)
	if err != nil {
		return err
	} else if size == 0 {
		return fmt.Errorf("write nothing")
	}
	err = write.Flush()
	if err != nil {
		return err
	}
	return nil
}

func ReadLinesFromFile(file *os.File) (lines []string, err error) {
	reader := bufio.NewReader(file)
	if reader.Size() < 0 {
		return nil, fmt.Errorf("Failed to read from %s ", file.Name())
	}

	for {
		line, err := reader.ReadString('\n')
		lines = append(lines, strings.TrimSpace(line))
		if err != nil {
			if err == io.EOF {
				return lines, nil
			} else {
				return nil, err
			}
		}
	}
}
