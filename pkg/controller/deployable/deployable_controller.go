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

package deployable

import (
	"time"

	"github.com/IBM/deployer-operator/pkg/utils"
	dplv1alpha1 "github.com/IBM/multicloud-operators-deployable/pkg/apis/app/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var (
	resync        = 20 * time.Minute
	deployableGVK = schema.GroupVersionKind{
		Group:   dplv1alpha1.SchemeGroupVersion.Group,
		Version: dplv1alpha1.SchemeGroupVersion.Version,
		Kind:    "Deployable",
	}
)

// Add creates a new Deployable Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, hubconfig *rest.Config, cluster types.NamespacedName) error {
	explorer, err := utils.InitExplorer(hubconfig, mgr.GetConfig(), cluster)
	if err != nil {
		klog.Error("Failed to create client explorer: ", err)
		return err
	}
	var dynamicHubFactory = dynamicinformer.NewDynamicSharedInformerFactory(explorer.DynamicHubClient, resync)

	return add(mgr, &ReconcileDeployable{
		dynamicHubFactory: dynamicHubFactory,
	})
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {

	return nil
}

// blank assignment to verify that ReconcileDeployer implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileDeployable{}

// ReconcileDeployable reconciles a Deployable object
type ReconcileDeployable struct {
	explorer          *utils.Explorer
	dynamicHubFactory dynamicinformer.DynamicSharedInformerFactory
	stopCh            chan struct{}
}

func (r *ReconcileDeployable) start() {
	r.stop()

	if r.dynamicHubFactory == nil {
		return
	}
	// generic explorer
	r.stopCh = make(chan struct{})

	if _, ok := r.explorer.GVKGVRMap[deployableGVK]; !ok {
		klog.Error("Failed to obtain gvr for application gvk:", deployableGVK.String())
		return
	}

	handler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(new interface{}) {
			r.syncDeployable(new)
		},
		UpdateFunc: func(old, new interface{}) {
			r.syncDeployable(new)
		},
		DeleteFunc: func(old interface{}) {
			r.syncRemoveDeployable(old)
		},
	}

	r.dynamicHubFactory.ForResource(r.explorer.GVKGVRMap[deployableGVK]).Informer().AddEventHandler(handler)

	r.stopCh = make(chan struct{})
	r.dynamicHubFactory.Start(r.stopCh)
}

func (r *ReconcileDeployable) stop() {
	if r.stopCh != nil {
		r.dynamicHubFactory.WaitForCacheSync(r.stopCh)
		close(r.stopCh)
	}
	r.stopCh = nil
}

func (r *ReconcileDeployable) syncDeployable(obj interface{}) {}

func (r *ReconcileDeployable) syncRemoveDeployable(obj interface{}) {}

func (r *ReconcileDeployable) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}
