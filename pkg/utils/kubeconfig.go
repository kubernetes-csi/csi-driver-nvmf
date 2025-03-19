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
	"os"
	"strings"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

// GetK8sClient returns a Kubernetes clientset using the appropriate configuration
func GetK8sClient() (kubernetes.Interface, error) {
	var restConfig *rest.Config
	var err error

	// Try KUBECONFIG environment variable first
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig != "" {
		klog.Infof("Attempting to create k8s client from KUBECONFIG: %s", kubeconfig)
		for _, kConf := range strings.Split(kubeconfig, ":") {
			restConfig, err = clientcmd.BuildConfigFromFlags("", kConf)
			if err == nil {
				clientset, err := kubernetes.NewForConfig(restConfig)
				if err == nil {
					klog.Infof("Created k8s client from KUBECONFIG: %s", kConf)
					return clientset, nil
				}
			}
		}
		klog.Warningf("Failed to create k8s client from KUBECONFIG: %v", err)
	}

	// Try default kubeconfig location
	klog.Info("Attempting to create k8s client from default kubeconfig location")
	home, err := os.UserHomeDir()
	if err == nil {
		defaultKubeConfig := strings.Join([]string{home, ".kube", "config"}, "/")
		if _, err := os.Stat(defaultKubeConfig); err == nil {
			restConfig, err = clientcmd.BuildConfigFromFlags("", defaultKubeConfig)
			if err == nil {
				clientset, err := kubernetes.NewForConfig(restConfig)
				if err == nil {
					klog.Infof("Created k8s client from default kubeconfig: %s", defaultKubeConfig)
					return clientset, nil
				}
			}
			klog.Warningf("Failed to create k8s client from default kubeconfig: %v", err)
		}
	}

	// Finally, try in-cluster config
	klog.Info("Attempting to create k8s client using in-cluster config")
	restConfig, err = rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, err
	}

	klog.Info("Created k8s client using in-cluster config")
	return clientset, nil
}
