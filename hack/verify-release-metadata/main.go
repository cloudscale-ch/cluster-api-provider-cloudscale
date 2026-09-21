/*
Copyright 2026 cloudscale.ch.

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

package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strconv"

	"k8s.io/apimachinery/pkg/util/yaml"
)

var releaseTagPattern = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)

type providerMetadata struct {
	ReleaseSeries []releaseSeries `json:"releaseSeries"`
}

type releaseSeries struct {
	Major int `json:"major"`
	Minor int `json:"minor"`
}

func main() {
	tag := flag.String("tag", "", "release tag to verify")
	metadataPath := flag.String("metadata", "metadata.yaml", "path to the clusterctl provider metadata")
	flag.Parse()

	metadata, err := os.ReadFile(*metadataPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading metadata: %v\n", err)
		os.Exit(1)
	}
	if err := verifyReleaseSeries(*tag, metadata); err != nil {
		fmt.Fprintf(os.Stderr, "Release metadata verification failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Verified that %s contains the release series for %s\n", *metadataPath, *tag)
}

func verifyReleaseSeries(tag string, metadataYAML []byte) error {
	matches := releaseTagPattern.FindStringSubmatch(tag)
	if matches == nil {
		return fmt.Errorf("tag %q is not a valid semantic version prefixed with v", tag)
	}

	major, err := strconv.Atoi(matches[1])
	if err != nil {
		return fmt.Errorf("parse tag major version: %w", err)
	}
	minor, err := strconv.Atoi(matches[2])
	if err != nil {
		return fmt.Errorf("parse tag minor version: %w", err)
	}

	metadataJSON, err := yaml.ToJSON(metadataYAML)
	if err != nil {
		return fmt.Errorf("parse metadata: %w", err)
	}
	metadata := providerMetadata{}
	if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
		return fmt.Errorf("parse metadata: %w", err)
	}
	if len(metadata.ReleaseSeries) == 0 {
		return errors.New("metadata contains no release series")
	}

	for _, series := range metadata.ReleaseSeries {
		if series.Major == major && series.Minor == minor {
			return nil
		}
	}

	return fmt.Errorf("metadata does not contain release series %d.%d required by tag %s", major, minor, tag)
}
