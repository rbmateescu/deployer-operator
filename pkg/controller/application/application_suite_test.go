// Copyright 2019 The Kubernetes Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package application

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	apis "github.com/IBM/deployer-operator/pkg/apis"
	"github.com/onsi/gomega"
)

var (
	managedClusterConfig *rest.Config
	hubClusterConfig     *rest.Config
)

func TestMain(m *testing.M) {
	// setup the managed cluster environment
	managedCluster := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "deploy", "crds"),
			filepath.Join("..", "..", "..", "hack", "test"),
		},
	}

	// add deployer-operator scheme
	err := apis.AddToScheme(scheme.Scheme)
	if err != nil {
		log.Fatal(err)
	}

	if managedClusterConfig, err = managedCluster.Start(); err != nil {
		log.Fatal(err)
	}

	// setup the hub cluster environment
	hubCluster := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "deploy", "crds"),
			filepath.Join("..", "..", "..", "hack", "test"),
		},
	}

	if hubClusterConfig, err = hubCluster.Start(); err != nil {
		log.Fatal(err)
	}

	code := m.Run()

	managedCluster.Stop()
	hubCluster.Stop()
	os.Exit(code)
}

const waitgroupDelta = 1

type HubClient struct {
	client.Client
	createCh chan runtime.Object
	deleteCh chan runtime.Object
	updateCh chan runtime.Object
}

func (hubClient HubClient) Create(ctx context.Context, obj runtime.Object, opts ...client.CreateOption) error {
	err := hubClient.Client.Create(ctx, obj, opts...)
	// non-blocking operation
	select {
	case hubClient.createCh <- obj:
	default:
	}
	return err
}

func (hubClient HubClient) Delete(ctx context.Context, obj runtime.Object, opts ...client.DeleteOption) error {
	err := hubClient.Client.Delete(ctx, obj, opts...)
	// non-blocking operation
	select {
	case hubClient.deleteCh <- obj:
	default:
	}
	return err
}

func (hubClient HubClient) Update(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
	err := hubClient.Client.Update(ctx, obj, opts...)
	// non-blocking operation
	select {
	case hubClient.updateCh <- obj:
	default:
	}
	return err
}

func SetupHubClient(innerClient client.Client) client.Client {
	cCh := make(chan runtime.Object)
	dCh := make(chan runtime.Object)
	uCh := make(chan runtime.Object)

	hubClient := HubClient{
		Client:   innerClient,
		createCh: cCh,
		deleteCh: dCh,
		updateCh: uCh,
	}
	return hubClient
}

type ApplicationSync struct {
	*ReconcileApplication
	createCh chan interface{}
	updateCh chan interface{}
	deleteCh chan interface{}
}

func (as ApplicationSync) start() {
	as.stop()
	// generic explorer
	as.stopCh = make(chan struct{})
	handler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(new interface{}) {
			as.syncCreateApplication(new)
		},
		UpdateFunc: func(old, new interface{}) {
			as.syncUpdateApplication(old, new)
		},
		DeleteFunc: func(old interface{}) {
			as.syncRemoveApplication(old)
		},
	}

	as.dynamicMCFactory.ForResource(as.explorer.GVKGVRMap[applicationGVK]).Informer().AddEventHandler(handler)

	as.stopCh = make(chan struct{})
	as.dynamicMCFactory.Start(as.stopCh)
}

func (as ApplicationSync) stop() {
	if as.stopCh != nil {
		as.dynamicMCFactory.WaitForCacheSync(as.stopCh)
		close(as.stopCh)
	}
	as.stopCh = nil
}

func (as ApplicationSync) syncCreateApplication(obj interface{}) {
	as.ReconcileApplication.syncCreateApplication(obj)
	// non-blocking operation
	select {
	case as.createCh <- obj:
	default:
	}

}
func (as ApplicationSync) syncUpdateApplication(old interface{}, new interface{}) {
	as.ReconcileApplication.syncUpdateApplication(old, new)
	// non-blocking operation
	select {
	case as.updateCh <- new:
	default:
	}

}
func (as ApplicationSync) syncRemoveApplication(old interface{}) {
	as.ReconcileApplication.syncRemoveApplication(old)
	// non-blocking operation
	select {
	case as.deleteCh <- old:
	default:
	}

}

func SetupApplicationSync(inner *ReconcileApplication) ReconcileApplicationInterface {
	cCh := make(chan interface{})
	uCh := make(chan interface{})
	dCh := make(chan interface{})

	appSync := ApplicationSync{
		ReconcileApplication: inner,
		createCh:             cCh,
		updateCh:             uCh,
		deleteCh:             dCh,
	}
	return appSync
}

// StartTestManager adds recFn
func StartTestManager(mgr manager.Manager, g *gomega.GomegaWithT) (chan struct{}, *sync.WaitGroup) {
	stop := make(chan struct{})
	wg := &sync.WaitGroup{}
	wg.Add(waitgroupDelta)

	go func() {
		defer wg.Done()
		g.Expect(mgr.Start(stop)).NotTo(gomega.HaveOccurred())
	}()

	return stop, wg
}
