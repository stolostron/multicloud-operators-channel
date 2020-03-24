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
	"reflect"
	"strings"

	chv1 "github.com/open-cluster-management/multicloud-operators-channel/pkg/apis/apps/v1"
	gitsync "github.com/open-cluster-management/multicloud-operators-channel/pkg/synchronizer/githubsynchronizer"
	helmsync "github.com/open-cluster-management/multicloud-operators-channel/pkg/synchronizer/helmreposynchronizer"
	"github.com/open-cluster-management/multicloud-operators-channel/pkg/utils"
	dplv1 "github.com/open-cluster-management/multicloud-operators-deployable/pkg/apis/apps/v1"
	dplutils "github.com/open-cluster-management/multicloud-operators-deployable/pkg/utils"
	placementutils "github.com/open-cluster-management/multicloud-operators-placementrule/pkg/utils"

	corev1 "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			//sync the channel to the serving-channel annotation in all involved secrets - remove channel
			r.syncSecrectAnnotation(nil, request.NamespacedName)

			//remove the channel from the serving-channel annotation in all involved ConfigMaps - remove channel
			r.syncConfigAnnotation(nil, request.NamespacedName)

			return reconcile.Result{}, utils.CleanupDeployables(r.Client, request.NamespacedName)
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
		klog.Info("Failed to validate RBAC for clusters for channel ", instance.Name, " with error: ", err)
		return reconcile.Result{}, err
	}

	// If the channel has relative secret and configMap, annotate the channel info in the secret and configMap
	if instance.Spec.SecretRef != nil && instance.Spec.SecretRef.Name > "" {
		secretName := instance.Spec.SecretRef.Name

		secretNamespace := instance.Spec.SecretRef.Namespace
		if secretNamespace == "" {
			secretNamespace = instance.Namespace
		}

		secrectInstance := &corev1.Secret{}
		secetKey := types.NamespacedName{Name: secretName, Namespace: secretNamespace}

		err = r.Get(context.TODO(), secetKey, secrectInstance)
		if err == nil {
			localLabels := secrectInstance.GetLabels()
			if localLabels == nil {
				localLabels = make(map[string]string)
			}

			localLabels[chv1.ServingChannel] = "true"
			secrectInstance.SetLabels(localLabels)

			err = r.Update(context.TODO(), secrectInstance)
			klog.Infof("Set label serving-channel to secret object: %#v, error: %#v", *secrectInstance, err)
		}
		//sync the channel to the serving-channel annotation in all involved secrets.
		r.syncSecrectAnnotation(instance, request.NamespacedName)
	}

	r.updateConfigMap(instance)

	//sync the channel to the serving-channel annotation in all involved ConfigMaps.
	r.syncConfigAnnotation(instance, request.NamespacedName)

	return reconcile.Result{}, nil
}

func (r *ReconcileChannel) updateConfigMap(instance *chv1.Channel) {
	if instance.Spec.ConfigMapRef != nil && instance.Spec.ConfigMapRef.Name > "" {
		configName := instance.Spec.ConfigMapRef.Name
		configNamespace := instance.Spec.ConfigMapRef.Namespace

		if configNamespace == "" {
			configNamespace = instance.Namespace
		}

		configInstance := &corev1.ConfigMap{}
		configKey := types.NamespacedName{Name: configName, Namespace: configNamespace}

		err := r.Get(context.TODO(), configKey, configInstance)
		if err == nil {
			localLabels := configInstance.GetLabels()

			if localLabels == nil {
				localLabels = make(map[string]string)
			}

			localLabels[chv1.ServingChannel] = "true"
			configInstance.SetLabels(localLabels)

			err = r.Update(context.TODO(), configInstance)

			klog.Infof("Set label serving-channel to configMap object: %#v, error: %#v", *configInstance, err)
		}
	}
}

func (r *ReconcileChannel) syncSecrectAnnotation(channel *chv1.Channel, channelKey types.NamespacedName) {
	if klog.V(debugLevel) {
		fnName := dplutils.GetFnName()
		klog.Infof("Entering: %v()", fnName)

		defer klog.Infof("Exiting: %v()", fnName)
	}

	secList := &corev1.SecretList{}

	secListOptions := &client.ListOptions{}

	secLabel := make(map[string]string)
	secLabel[chv1.ServingChannel] = "true"
	labelSelector := &metav1.LabelSelector{
		MatchLabels: secLabel,
	}

	clSelector, err := dplutils.ConvertLabels(labelSelector)
	if err != nil {
		klog.Error("Failed to set label selector for secret objects. err: ", err)
		return
	}

	secListOptions.LabelSelector = clSelector

	err = r.Client.List(context.TODO(), secList, secListOptions)
	if err != nil {
		klog.Error("Failed to list Secret objects. error: ", err)
		return
	}

	for _, secret := range secList.Items {
		annotations := secret.GetAnnotations()

		if annotations == nil {
			annotations = make(map[string]string)
		}

		newServingChannel := annotations[chv1.ServingChannel]

		if channel != nil && channel.Spec.SecretRef != nil {
			if secret.Name == channel.Spec.SecretRef.Name && channel.Namespace == secret.Namespace {
				newServingChannel = utils.UpdateServingChannel(annotations[chv1.ServingChannel], channelKey.String(), "add")
				annotations[DeployableAnnotation] = "true"
			}
		} else {
			newServingChannel = utils.UpdateServingChannel(annotations[chv1.ServingChannel], channelKey.String(), "remove")
		}

		if newServingChannel > "" {
			annotations[chv1.ServingChannel] = newServingChannel
		} else {
			delete(annotations, chv1.ServingChannel)
		}

		secret.SetAnnotations(annotations)

		err = r.Update(context.TODO(), &secret)
		klog.Infof("Annotate secret object: %#v, error: %#v", secret, err)
	}
}

func (r *ReconcileChannel) syncConfigAnnotation(channel *chv1.Channel, channelKey types.NamespacedName) {
	if klog.V(debugLevel) {
		fnName := dplutils.GetFnName()
		klog.Infof("Entering: %v()", fnName)

		defer klog.Infof("Exiting: %v()", fnName)
	}

	configList := &corev1.ConfigMapList{}

	configListOptions := &client.ListOptions{}

	configLabel := make(map[string]string)
	configLabel[chv1.ServingChannel] = "true"
	labelSelector := &metav1.LabelSelector{
		MatchLabels: configLabel,
	}

	clSelector, err := dplutils.ConvertLabels(labelSelector)
	if err != nil {
		klog.Error("Failed to set label selector for configMap objects. err: ", err)
		return
	}

	configListOptions.LabelSelector = clSelector

	err = r.Client.List(context.TODO(), configList, configListOptions)
	if err != nil {
		klog.Error("Failed to list ConfigMap objects. error: ", err)
		return
	}

	for _, config := range configList.Items {
		annotations := config.GetAnnotations()
		newServingChannel := annotations[chv1.ServingChannel]

		if channel != nil && channel.Spec.ConfigMapRef != nil && channel.Spec.ConfigMapRef.Name > "" && channel.Spec.ConfigMapRef.Namespace > "" {
			if config.Name == channel.Spec.ConfigMapRef.Name && config.Namespace == channel.Spec.ConfigMapRef.Namespace {
				newServingChannel = utils.UpdateServingChannel(annotations[chv1.ServingChannel], channelKey.String(), "add")
			}
		} else {
			newServingChannel = utils.UpdateServingChannel(annotations[chv1.ServingChannel], channelKey.String(), "remove")
		}

		if newServingChannel > "" {
			annotations[chv1.ServingChannel] = newServingChannel
		} else {
			delete(annotations, chv1.ServingChannel)
		}

		config.SetAnnotations(annotations)

		err = r.Update(context.TODO(), &config)
		klog.Infof("Annotate configMap object: %#v, error: %#v", config, err)
	}
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

	cllist := &clusterv1alpha1.ClusterList{}

	err = r.List(context.TODO(), cllist, &client.ListOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}

		return err
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
		if errors.IsNotFound(err) {
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
		if errors.IsNotFound(err) {
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
