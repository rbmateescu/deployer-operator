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
	"context"

	appv1alpha1 "github.com/IBM/deployer-operator/pkg/apis/app/v1alpha1"
	"github.com/IBM/deployer-operator/pkg/utils"
	dplv1alpha1 "github.com/IBM/multicloud-operators-deployable/pkg/apis/app/v1alpha1"
	subv1alpha1 "github.com/IBM/multicloud-operators-subscription/pkg/apis/app/v1alpha1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	trueCondition       = "true"
	packageInfoLogLevel = 3
)

var (
	ignoreAnnotation = []string{
		dplv1alpha1.AnnotationHosting,
		subv1alpha1.AnnotationHosting,
	}
)

func SyncDeployable(obj interface{}, explorer *utils.Explorer) {
	metaobj, err := meta.Accessor(obj)
	if err != nil {
		klog.Error("Failed to access object metadata for sync with error: ", err)
		return
	}

	annotations := metaobj.GetAnnotations()
	if annotations != nil {
		matched := false
		for _, key := range ignoreAnnotation {
			if _, matched = annotations[key]; matched {
				break
			}
		}

		if matched {
			klog.Info("Ignore object:", metaobj.GetNamespace(), "/", metaobj.GetName())
			return
		}
	}

	dpl := locateDeployableForObject(metaobj, explorer)
	if dpl == nil {
		dpl = &dplv1alpha1.Deployable{}
		dpl.GenerateName = genDeployableGenerateName(metaobj)
		dpl.Namespace = explorer.Cluster.Namespace
	}

	prepareDeployable(dpl, metaobj, explorer)
	prepareTemplate(metaobj)

	dpl.Spec.Template = &runtime.RawExtension{
		Object: metaobj.(runtime.Object),
	}

	if dpl.UID == "" {
		err = explorer.HubClient.Create(context.TODO(), dpl)
	} else {
		err = explorer.HubClient.Update(context.TODO(), dpl)
	}

	if err != nil {
		klog.Error("Failed to sync object deployable with error: ", err)
	}

	err = patchObject(dpl, metaobj, explorer)
	if err != nil {
		klog.Error("Failed to patch object with error: ", err)
	}

	klog.V(packageInfoLogLevel).Info("Successfully synched deployable for object ", metaobj.GetNamespace()+"/"+metaobj.GetName())
}

func patchObject(dpl *dplv1alpha1.Deployable, metaobj metav1.Object, explorer *utils.Explorer) error {

	rtobj := metaobj.(runtime.Object)
	klog.V(5).Info("Patching meta object:", metaobj, " covert to runtime object:", rtobj)

	objgvr := explorer.GVKGVRMap[rtobj.GetObjectKind().GroupVersionKind()]

	ucobj, err := explorer.DynamicMCClient.Resource(objgvr).Namespace(metaobj.GetNamespace()).Get(metaobj.GetName(), metav1.GetOptions{})
	if err != nil {
		klog.Error("Failed to patch managed cluster object with error:", err)
		return err
	}

	annotations := ucobj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[subv1alpha1.AnnotationHosting] = "/"
	annotations[subv1alpha1.AnnotationSyncSource] = "subnsdpl-/"
	annotations[dplv1alpha1.AnnotationHosting] = types.NamespacedName{Namespace: dpl.GetNamespace(), Name: dpl.GetName()}.String()
	annotations[appv1alpha1.AnnotationDiscovered] = trueCondition
	ucobj.SetAnnotations(annotations)

	_, err = explorer.DynamicMCClient.Resource(objgvr).Namespace(metaobj.GetNamespace()).Update(ucobj, metav1.UpdateOptions{})

	return err
}

func genDeployableGenerateName(obj metav1.Object) string {
	return obj.GetName() + "-"
}

func locateDeployableForObject(metaobj metav1.Object, explorer *utils.Explorer) *dplv1alpha1.Deployable {
	dpllist := &dplv1alpha1.DeployableList{}

	err := explorer.HubClient.List(context.TODO(), dpllist, &client.ListOptions{
		Namespace: explorer.Cluster.Namespace,
	})
	if err != nil {
		klog.Error("Failed to list deployable objects from hub cluster namespace with error:", err)
		return nil
	}

	var objdpl *dplv1alpha1.Deployable

	for _, dpl := range dpllist.Items {
		annotations := dpl.GetAnnotations()
		if annotations == nil {
			continue
		}

		key := types.NamespacedName{
			Namespace: metaobj.GetNamespace(),
			Name:      metaobj.GetName(),
		}.String()

		srcobj, ok := annotations[appv1alpha1.SourceObject]
		if ok && srcobj == key {
			objdpl = &dpl
			break
		}
	}

	return objdpl
}

var (
	obsoleteAnnotations = []string{
		"kubectl.kubernetes.io/last-applied-configuration",
	}
)

func prepareTemplate(metaobj metav1.Object) {
	var emptyuid types.UID
	metaobj.SetUID(emptyuid)
	metaobj.SetSelfLink("")
	metaobj.SetResourceVersion("")
	metaobj.SetGeneration(0)
	metaobj.SetCreationTimestamp(metav1.Time{})

	annotations := metaobj.GetAnnotations()
	if annotations != nil {
		for _, k := range obsoleteAnnotations {
			delete(annotations, k)
		}
		metaobj.SetAnnotations(annotations)
	}
}

func prepareDeployable(deployable *dplv1alpha1.Deployable, metaobj metav1.Object, explorer *utils.Explorer) {
	labels := deployable.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}

	for key, value := range metaobj.GetLabels() {
		labels[key] = value
	}

	deployable.SetLabels(labels)

	annotations := deployable.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	annotations[appv1alpha1.SourceObject] = types.NamespacedName{Namespace: metaobj.GetNamespace(), Name: metaobj.GetName()}.String()
	annotations[dplv1alpha1.AnnotationManagedCluster] = explorer.Cluster.String()
	annotations[dplv1alpha1.AnnotationLocal] = trueCondition
	annotations[appv1alpha1.AnnotationDiscovered] = trueCondition
	deployable.SetAnnotations(annotations)
}
