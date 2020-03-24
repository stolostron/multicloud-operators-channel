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

package channel

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	chv1 "github.com/open-cluster-management/multicloud-operators-channel/pkg/apis/apps/v1"
	gitsync "github.com/open-cluster-management/multicloud-operators-channel/pkg/synchronizer/githubsynchronizer"
	helmsync "github.com/open-cluster-management/multicloud-operators-channel/pkg/synchronizer/helmreposynchronizer"
	"github.com/open-cluster-management/multicloud-operators-channel/pkg/utils"
	dplv1 "github.com/open-cluster-management/multicloud-operators-deployable/pkg/apis/apps/v1"
	dplutils "github.com/open-cluster-management/multicloud-operators-deployable/pkg/utils"
	placementutils "github.com/open-cluster-management/multicloud-operators-placementrule/pkg/utils"

	gerr "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	clusterv1alpha1 "k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var (
	clusterRules = []rbac.PolicyRule{
		{
			Verbs:     []string{"get", "list", "watch"},
			APIGroups: []string{dplv1.SchemeGroupVersion.Group},
			Resources: []string{"deployables", "deployables/status", "channels", "channels/status"},
		},
		{
			Verbs:     []string{"get", "list", "watch"},
			APIGroups: []string{""},
			Resources: []string{"secrets", "configmaps"},
		},
	}

	//DeployableAnnotation is used to indicate a resource as a logic deployable
	DeployableAnnotation = dplv1.SchemeGroupVersion.Group + "/deployables"
	srtGvk               = schema.GroupVersionKind{Group: "", Kind: "Secret", Version: "v1"}
	cmGvk                = schema.GroupVersionKind{Group: "", Kind: "ConfigMap", Version: "v1"}
)

const (
	debugLevel = klog.Level(10)
)

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new Channel Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, recorder record.EventRecorder,
	channelDescriptor *utils.ChannelDescriptor, sync *helmsync.ChannelSynchronizer,
	gsync *gitsync.ChannelSynchronizer) error {
	return add(mgr, newReconciler(mgr, recorder))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, recorder record.EventRecorder) reconcile.Reconciler {
	return &ReconcileChannel{Client: mgr.GetClient(), scheme: mgr.GetScheme(), Recorder: recorder}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	if klog.V(debugLevel) {
		fnName := dplutils.GetFnName()
		klog.Infof("Entering: %v()", fnName)

		defer klog.Infof("Exiting: %v()", fnName)
	}
	// Create a new controller
	c, err := controller.New("channel-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Channel
	err = c.Watch(&source.Kind{Type: &chv1.Channel{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	if placementutils.IsReadyACMClusterRegistry(mgr.GetAPIReader()) {
		err = c.Watch(
			&source.Kind{Type: &clusterv1alpha1.Cluster{}},
			&handler.EnqueueRequestsFromMapFunc{ToRequests: &clusterMapper{mgr.GetClient()}},
			placementutils.ClusterPredicateFunc,
		)
	}

	return err
}

type clusterMapper struct {
	client.Client
}

// Map triggers all placements
func (mapper *clusterMapper) Map(obj handler.MapObject) []reconcile.Request {
	if klog.V(debugLevel) {
		fnName := dplutils.GetFnName()
		klog.Infof("Entering: %v()", fnName)

		defer klog.Infof("Exiting: %v()", fnName)
	}

	cname := obj.Meta.GetName()

	klog.Info("In cluster Mapper for ", cname)

	plList := &chv1.ChannelList{}

	listopts := &client.ListOptions{}
	err := mapper.List(context.TODO(), plList, listopts)

	if err != nil {
		klog.Error(err)
	}

	var requests []reconcile.Request

	for _, pl := range plList.Items {
		objkey := types.NamespacedName{
			Name:      pl.GetName(),
			Namespace: pl.GetNamespace(),
		}

		requests = append(requests, reconcile.Request{NamespacedName: objkey})
	}

	return requests
}

var _ reconcile.Reconciler = &ReconcileChannel{}

// ReconcileChannel reconciles a Channel object
type ReconcileChannel struct {
	client.Client
	scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// Reconcile reads that state of the cluster for a Channel object and makes changes based on the state read
// and what is in the Channel.Spec
// a Deployment as an example
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.open-cluster-management.io,resources=channels,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.open-cluster-management.io,resources=channels/status,verbs=get;update;patch
func (r *ReconcileChannel) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	if klog.V(debugLevel) {
		fnName := dplutils.GetFnName()
		klog.Infof("Entering: %v()", fnName)

		defer klog.Infof("Exiting: %v()", fnName)
	}
	// Fetch the Channel instance

	klog.Info("Reconciling:", request.NamespacedName)

	instance := &chv1.Channel{}

	err := r.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if kerr.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			//sync the channel to the serving-channel annotation in all involved secrets - remove channel
			if err := r.syncReferredObjAnnotation(request, nil, srtGvk); err != nil {
				return reconcile.Result{}, err
			}

			//remove the channel from the serving-channel annotation in all involved ConfigMaps - remove channel
			if err := r.syncReferredObjAnnotation(request, nil, cmGvk); err != nil {
				return reconcile.Result{}, err
			}

			if err := utils.CleanupDeployables(r.Client, request.NamespacedName); err != nil {
				klog.Errorf("failed to reconcile on deletion, err %v", err)
				return reconcile.Result{}, err
			}

			return reconcile.Result{}, nil
		}

		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	if (strings.EqualFold(string(instance.Spec.Type), chv1.ChannelTypeNamespace)) && (instance.Spec.Pathname != instance.GetNamespace()) {
		instance.Spec.Pathname = instance.GetNamespace()

		err := r.Update(context.TODO(), instance)
		if err != nil {
			klog.Infof("Can't update the pathname field due to %v", err)
			return reconcile.Result{}, err
		}

		return reconcile.Result{}, nil
	}

	err = r.validateClusterRBAC(instance)
	if err != nil {
		klog.Info("failed to validate RBAC for clusters for channel ", instance.Name, " with error: ", err)
		return reconcile.Result{}, err
	}

	// If the channel has relative secret and configMap, annotate the channel info in the secret and configMap

	//sync the channel to the serving-channel annotation in all involved secrets.
	srtRef := instance.Spec.SecretRef

	if err := r.updatedReferredObjLabel(srtRef, srtGvk); err != nil {
		klog.Errorf("failed to update referred secret label %v", err)
	}

	if err := r.syncReferredObjAnnotation(request, srtRef, srtGvk); err != nil {
		klog.Errorf("faild to annotation %v", err)
	}

	//sync the channel to the serving-channel annotation in all involved ConfigMaps.
	//r.syncConfigAnnotation(instance, request.NamespacedName)
	cmRef := instance.Spec.ConfigMapRef
	if err := r.updatedReferredObjLabel(cmRef, cmGvk); err != nil {
		klog.Errorf("failed to update referred configMap label %v", err)
	}

	if err := r.syncReferredObjAnnotation(request, cmRef, cmGvk); err != nil {
		klog.Errorf("faild to annotation %v", err)
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileChannel) updatedReferredObjLabel(ref *corev1.ObjectReference, objGvk schema.GroupVersionKind) error {
	if ref == nil {
		return gerr.New(fmt.Sprintf("empty referred object %v", objGvk.Kind))
	}

	objName := ref.Name
	objNs := ref.Namespace

	obj := &unstructured.Unstructured{}
	objKey := types.NamespacedName{Name: objName, Namespace: objNs}

	obj.SetGroupVersionKind(objGvk)

	if err := r.Get(context.TODO(), objKey, obj); err != nil {
		return err
	}

	localLabels := obj.GetLabels()
	if localLabels == nil {
		localLabels = make(map[string]string)
	}

	localLabels[chv1.ServingChannel] = "true"
	obj.SetLabels(localLabels)

	if err := r.Update(context.TODO(), obj); err != nil {
		return err
	}

	klog.Infof("Set label serving-channel to object: %v", objKey.String())

	return nil
}

func (r *ReconcileChannel) syncReferredObjAnnotation(rq reconcile.Request, ref *corev1.ObjectReference, objGvk schema.GroupVersionKind) error {
	if klog.V(debugLevel) {
		fnName := dplutils.GetFnName()
		klog.Infof("Entering: %v()", fnName)

		defer klog.Infof("Exiting: %v()", fnName)
	}

	chnKey := types.NamespacedName{Name: rq.Name, Namespace: rq.Namespace}

	uObjList := &unstructured.UnstructuredList{}

	uObjList.SetGroupVersionKind(objGvk)

	opts := &client.ListOptions{}

	objLabel := make(map[string]string)
	objLabel[chv1.ServingChannel] = "true"
	labelSelector := &metav1.LabelSelector{
		MatchLabels: objLabel,
	}

	clSelector, err := dplutils.ConvertLabels(labelSelector)
	if err != nil {
		return gerr.Wrap(err, "failed to set lable selector for referred object")
	}

	opts.LabelSelector = clSelector

	if err := r.Client.List(context.TODO(), uObjList, opts); err != nil {
		return gerr.Wrapf(err, "failed to list objects %v. error: ", objGvk.String())
	}

	for _, obj := range uObjList.Items {
		annotations := obj.GetAnnotations()

		if annotations == nil {
			annotations = make(map[string]string)
		}

		newServingChannel := annotations[chv1.ServingChannel]

		if ref != nil && (ref.Name > "" && ref.Namespace > "") {
			if obj.GetName() == ref.Name && obj.GetNamespace() == ref.Namespace {
				newServingChannel = utils.UpdateServingChannel(annotations[chv1.ServingChannel], chnKey.String(), "add")
			}
		} else {
			newServingChannel = utils.UpdateServingChannel(annotations[chv1.ServingChannel], chnKey.String(), "remove")
		}

		if newServingChannel > "" {
			annotations[chv1.ServingChannel] = newServingChannel
		} else {
			delete(annotations, chv1.ServingChannel)
		}

		obj.SetAnnotations(annotations)

		if err := r.Update(context.TODO(), &obj); err != nil {
			klog.Errorf("failed to annotate object: %v/%v, err: %#v", obj.GetNamespace(), obj.GetName(), err)
		}
	}

	return nil
}

func (r *ReconcileChannel) validateClusterRBAC(instance *chv1.Channel) error {
	if klog.V(debugLevel) {
		fnName := dplutils.GetFnName()
		klog.Infof("Entering: %v()", fnName)

		defer klog.Infof("Exiting: %v()", fnName)
	}

	role := &rbac.Role{}

	err := r.setupRole(instance, role)
	if err != nil {
		return err
	}

	rolebinding := &rbac.RoleBinding{}

	var subjects []rbac.Subject

	clusterCRD := &apiextensionsv1beta1.CustomResourceDefinition{}
	clusterCRDKey := client.ObjectKey{
		Name: "clusters.clusterregistry.k8s.io",
	}

	if err := r.Get(context.TODO(), clusterCRDKey, clusterCRD); err != nil {
		klog.Infof("skipping role binding for %v/%v since cluste CRD is not ready, err %v", instance.Name, instance.Namespace, err)
		return nil
	}

	cllist := &clusterv1alpha1.ClusterList{}

	err = r.List(context.TODO(), cllist, &client.ListOptions{})
	if err != nil {
		if kerr.IsNotFound(err) {
			return nil
		}

		return gerr.Wrap(err, "failed to list cluster resource while rolebinding")
	}

	for _, cl := range cllist.Items {
		subjects = append(subjects, rbac.Subject{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "User",
			Name:     "hcm:clusters:" + cl.Namespace + ":" + cl.Name,
		})
	}

	roleref := rbac.RoleRef{
		APIGroup: "rbac.authorization.k8s.io",
		Kind:     "Role",
		Name:     instance.Name,
	}

	err = r.Get(context.TODO(), types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, rolebinding)
	if err != nil {
		if kerr.IsNotFound(err) {
			rolebinding.Name = instance.Name
			rolebinding.Namespace = instance.Namespace

			err = controllerutil.SetControllerReference(instance, rolebinding, r.scheme)
			if err != nil {
				klog.Error("Failed to set controller reference, err:", err)
				return err
			}

			rolebinding.RoleRef = roleref
			rolebinding.Subjects = subjects

			err = r.Create(context.TODO(), rolebinding)
			if err != nil {
				klog.Error("Failed to create rolebinding, err:", err)
				return err
			}
		} else {
			return err
		}
	} else {
		if !reflect.DeepEqual(subjects, rolebinding.Subjects) || !reflect.DeepEqual(rolebinding.RoleRef, roleref) {
			err = controllerutil.SetControllerReference(instance, rolebinding, r.scheme)
			if err != nil {
				klog.Error("Failed to set controller reference, err:", err)
				return err
			}

			rolebinding.RoleRef = roleref
			rolebinding.Subjects = subjects
			err = r.Update(context.TODO(), rolebinding)
		}
	}

	return err
}

func (r *ReconcileChannel) setupRole(instance *chv1.Channel, role *rbac.Role) error {
	err := r.Get(context.TODO(), types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, role)
	if err != nil {
		if kerr.IsNotFound(err) {
			role.Name = instance.Name
			role.Namespace = instance.Namespace
			role.Rules = clusterRules

			err = controllerutil.SetControllerReference(instance, role, r.scheme)
			if err != nil {
				klog.Error("Failed to set controller reference, err:", err)
				return err
			}

			err = r.Create(context.TODO(), role)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	} else {
		if !reflect.DeepEqual(role.Rules, clusterRules) {
			role.Rules = clusterRules

			err = controllerutil.SetControllerReference(instance, role, r.scheme)
			if err != nil {
				klog.Error("Failed to set controller reference, err:", err)
				return err
			}

			err = r.Update(context.TODO(), role)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
