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

package deployer

import (
	"context"
	"testing"
	"time"

	"github.com/onsi/gomega"

	appv1alpha1 "github.com/IBM/deployer-operator/pkg/apis/app/v1alpha1"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	kubevirtType = "kubevirt"

	deployerName      = "test-deployer-operator"
	deployerNamespace = "default"
)

var (
	// A Deployer resource with metadata and spec.
	managedClusterDeployer = &appv1alpha1.Deployer{
		TypeMeta: metav1.TypeMeta{
			Kind: "Deployer",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        deployerName,
			Namespace:   deployerNamespace,
			Annotations: map[string]string{appv1alpha1.IsDefaultDeployer: "true"},
		},
		Spec: appv1alpha1.DeployerSpec{
			Type: kubevirtType,
		},
	}

	// deployer object
	key = types.NamespacedName{
		Name:      deployerName,
		Namespace: deployerNamespace,
	}

	expectedRequest = reconcile.Request{NamespacedName: key}

	timeout = time.Second * 2
)

func TestReconcile(t *testing.T) {
	g := gomega.NewWithT(t)

	// Create the Deployer object and expect the Reconcile and Deployment to be created
	dep := managedClusterDeployer.DeepCopy()
	g.Expect(managedClusterClient.Create(context.TODO(), dep)).NotTo(gomega.HaveOccurred())

	// reconcile.Request
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))

	// deployer in managed cluster
	deployerResource := &appv1alpha1.Deployer{}
	g.Expect(managedClusterClient.Get(context.TODO(), expectedRequest.NamespacedName, deployerResource)).NotTo(gomega.HaveOccurred())

	// don't use dep at this point for assertion, as the client.Create nulled out the TypeMeta.
	g.Expect(deployerResource.TypeMeta.Kind).To(gomega.Equal(managedClusterDeployer.TypeMeta.Kind))
	g.Expect(deployerResource.ObjectMeta.Name).To(gomega.Equal(managedClusterDeployer.ObjectMeta.Name))
	g.Expect(deployerResource.ObjectMeta.Namespace).To(gomega.Equal(managedClusterDeployer.ObjectMeta.Namespace))
	g.Expect(deployerResource.Spec).To(gomega.Equal(managedClusterDeployer.Spec))

	// don't use defer for cleanup, as we weant to go through reconcile logic to make sure hubclient
	// cache stays consistent
	managedClusterClient.Delete(context.TODO(), dep)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
}

func TestDeployersetCreatedOnHub(t *testing.T) {
	g := gomega.NewWithT(t)

	dep := managedClusterDeployer.DeepCopy()
	g.Expect(managedClusterClient.Create(context.TODO(), dep)).NotTo(gomega.HaveOccurred())

	// reconcile.Request
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))

	// deployerset in hub cluster
	deployersetResource := &appv1alpha1.DeployerSet{}
	g.Expect(hubClusterClient.Get(context.TODO(), clusterOnHub, deployersetResource)).NotTo(gomega.HaveOccurred())
	g.Expect(deployersetResource.ObjectMeta.Name).To(gomega.Equal(clusterNameOnHub))
	g.Expect(deployersetResource.ObjectMeta.Namespace).To(gomega.Equal(clusterNamespaceOnHub))
	g.Expect(deployersetResource.Spec.DefaultDeployer).To(gomega.Equal(deployerNamespace + "/" + deployerName))
	g.Expect(deployersetResource.Spec.Deployers).To(gomega.HaveLen(1))
	g.Expect(deployersetResource.Spec.Deployers[0].Spec).To(gomega.Equal(managedClusterDeployer.Spec))

	// don't use defer for cleanup, as we weant to go through reconcile logic to make sure hubclient
	// cache stays consistent
	managedClusterClient.Delete(context.TODO(), dep)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
}

func TestDeployersetRemovedFromHub(t *testing.T) {
	g := gomega.NewWithT(t)

	dep := managedClusterDeployer.DeepCopy()
	g.Expect(managedClusterClient.Create(context.TODO(), dep)).NotTo(gomega.HaveOccurred())

	// reconcile.Request
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))

	managedClusterClient.Delete(context.TODO(), dep)

	// reconcile.Request
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))

	// deployerset in hub cluster
	deployersetResource := &appv1alpha1.DeployerSet{}
	err := hubClusterClient.Get(context.TODO(), clusterOnHub, deployersetResource)
	g.Expect(errors.IsNotFound(err)).To(gomega.BeTrue())
}
